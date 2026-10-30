package gotemplate_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util/cache"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

// concurrentWorkers is the number of goroutines rendering the same renderer
// instance at once. It must be large enough to make overlapping lazy parses
// and renders likely under the race detector.
const concurrentWorkers = 8

// concurrencyTemplate returns a template for the named source. The source name
// is baked into the template, the Env value is request-specific, and the shout
// function proves pure renderer-level template functions run concurrently.
func concurrencyTemplate(sourceName string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: %s-{{ .Env }}-config
data:
  rendered: {{ shout .Env }}
`, sourceName)
}

// concurrencySourceFS returns an immutable filesystem holding one template for
// the named source, shared by every worker through the renderer.
func concurrencySourceFS(sourceName string) fstest.MapFS {
	return fstest.MapFS{
		"templates/config.yaml.tpl": &fstest.MapFile{
			Data: []byte(concurrencyTemplate(sourceName)),
		},
	}
}

// concurrencyRendererOptions are shared by every concurrency renderer: they
// register a pure template function exercised by every concurrent render.
func concurrencyRendererOptions() []gotemplate.RendererOption {
	return []gotemplate.RendererOption{
		gotemplate.WithFunc("shout", strings.ToUpper),
	}
}

// concurrencyWorkerEnvs returns the request-specific Env value for each
// worker, where worker i renders with env-i.
func concurrencyWorkerEnvs() []string {
	envs := make([]string, concurrentWorkers)
	for i := range concurrentWorkers {
		envs[i] = fmt.Sprintf("env-%d", i)
	}

	return envs
}

// concurrencyWorkerValues returns request-specific values for each worker env.
func concurrencyWorkerValues(envs []string) []pkgtypes.Values {
	values := make([]pkgtypes.Values, len(envs))
	for i, env := range envs {
		values[i] = pkgtypes.Values{"Env": env}
	}

	return values
}

// TestRendererConcurrency verifies that concurrent calls to the same renderer
// succeed through lazy template parsing and per-source caches: every result
// carries its own request-specific values, sources with colliding cache keys
// never mix, and returned nested objects are independent of other results,
// later cache hits, the original values, and the source filesystems.
func TestRendererConcurrency(t *testing.T) {

	t.Run("cold renders parse once and keep same-path sources isolated", func(t *testing.T) {
		g := NewWithT(t)

		// Both sources share the path and static values, so their default cache
		// keys collide; only the per-source cache isolation keeps them apart.
		alphaValues := map[string]any{"Env": "static"}
		betaValues := map[string]any{"Env": "static"}
		alphaFS := concurrencySourceFS("alpha")
		betaFS := concurrencySourceFS("beta")

		opts := append(concurrencyRendererOptions(),
			gotemplate.WithCache(),
			gotemplate.WithSourceAnnotations(true),
		)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:     alphaFS,
					Path:   "templates/*.tpl",
					Values: gotemplate.Values(alphaValues),
				},
				{
					FS:     betaFS,
					Path:   "templates/*.tpl",
					Values: gotemplate.Values(betaValues),
				},
			},
			opts...,
		)
		g.Expect(err).ToNot(HaveOccurred())

		envs := concurrencyWorkerEnvs()
		workerValues := concurrencyWorkerValues(envs)
		results, errs := renderConcurrently(t.Context(), renderer, workerValues)

		for i := range concurrentWorkers {
			g.Expect(errs[i]).ToNot(HaveOccurred())

			// Every worker gets both sources rendered with its own values, and
			// distinct values mean every render is live, not a cache hit.
			expectSourceResults(g, results[i], envs[i], "alpha", "beta")
			g.Expect(renderOrigins(results[i])).To(Equal(map[string]string{
				"alpha-" + envs[i] + "-config": pkgtypes.RenderOriginLive,
				"beta-" + envs[i] + "-config":  pkgtypes.RenderOriginLive,
			}))
		}

		// Mutating one result must not affect any other concurrent result...
		mutateNestedContent(g, results[0])

		for i := 1; i < concurrentWorkers; i++ {
			expectSourceResults(g, results[i], envs[i], "alpha", "beta")
		}

		// ...nor a later cache hit for another worker's values...
		cached, err := renderer.Process(t.Context(), workerValues[1])
		g.Expect(err).ToNot(HaveOccurred())
		expectSourceResults(g, cached, envs[1], "alpha", "beta")
		g.Expect(renderOrigins(cached)).To(Equal(map[string]string{
			"alpha-" + envs[1] + "-config": pkgtypes.RenderOriginCache,
			"beta-" + envs[1] + "-config":  pkgtypes.RenderOriginCache,
		}))

		// ...nor the original source values and filesystems.
		g.Expect(alphaValues).To(Equal(map[string]any{"Env": "static"}))
		g.Expect(betaValues).To(Equal(map[string]any{"Env": "static"}))
		g.Expect(alphaFS["templates/config.yaml.tpl"].Data).To(Equal([]byte(concurrencyTemplate("alpha"))))
		g.Expect(betaFS["templates/config.yaml.tpl"].Data).To(Equal([]byte(concurrencyTemplate("beta"))))
	})

	t.Run("warm renders reflect request values with caching disabled", func(t *testing.T) {
		g := NewWithT(t)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   concurrencySourceFS("alpha"),
					Path: "templates/*.tpl",
				},
			},
			concurrencyRendererOptions()...,
		)
		g.Expect(err).ToNot(HaveOccurred())

		// Warm up with a successful cold parse and render.
		warmup, err := renderer.Process(t.Context(), pkgtypes.Values{"Env": "warmup"})
		g.Expect(err).ToNot(HaveOccurred())
		expectSourceResults(g, warmup, "warmup", "alpha")

		envs := concurrencyWorkerEnvs()
		workerValues := concurrencyWorkerValues(envs)
		results, errs := renderConcurrently(t.Context(), renderer, workerValues)

		for i := range concurrentWorkers {
			g.Expect(errs[i]).ToNot(HaveOccurred())
			expectSourceResults(g, results[i], envs[i], "alpha")
		}
	})

	t.Run("warm renders reflect request values with default cache keys", func(t *testing.T) {
		g := NewWithT(t)

		opts := append(concurrencyRendererOptions(),
			gotemplate.WithCache(),
			gotemplate.WithSourceAnnotations(true),
		)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   concurrencySourceFS("alpha"),
					Path: "templates/*.tpl",
				},
			},
			opts...,
		)
		g.Expect(err).ToNot(HaveOccurred())

		// Warm up with a successful cold parse and render.
		warmup, err := renderer.Process(t.Context(), pkgtypes.Values{"Env": "warmup"})
		g.Expect(err).ToNot(HaveOccurred())
		expectSourceResults(g, warmup, "warmup", "alpha")

		envs := concurrencyWorkerEnvs()
		workerValues := concurrencyWorkerValues(envs)

		// First concurrent round: distinct values mean distinct default keys,
		// so every worker renders live with its own values.
		live, errs := renderConcurrently(t.Context(), renderer, workerValues)
		for i := range concurrentWorkers {
			g.Expect(errs[i]).ToNot(HaveOccurred())
			expectSourceResults(g, live[i], envs[i], "alpha")
			g.Expect(renderOrigins(live[i])).To(Equal(map[string]string{
				"alpha-" + envs[i] + "-config": pkgtypes.RenderOriginLive,
			}))
		}

		// Second concurrent round with the same values: every worker now owns
		// a cache hit that still reflects its own values.
		cached, errs := renderConcurrently(t.Context(), renderer, workerValues)
		for i := range concurrentWorkers {
			g.Expect(errs[i]).ToNot(HaveOccurred())
			expectSourceResults(g, cached[i], envs[i], "alpha")
			g.Expect(renderOrigins(cached[i])).To(Equal(map[string]string{
				"alpha-" + envs[i] + "-config": pkgtypes.RenderOriginCache,
			}))
		}

		// Mutating a cache-hit result must not affect a later cache hit.
		mutateNestedContent(g, cached[0])

		again, err := renderer.Process(t.Context(), workerValues[1])
		g.Expect(err).ToNot(HaveOccurred())
		expectSourceResults(g, again, envs[1], "alpha")
		g.Expect(renderOrigins(again)).To(Equal(map[string]string{
			"alpha-" + envs[1] + "-config": pkgtypes.RenderOriginCache,
		}))
	})

	t.Run("constant custom keys keep same-path sources isolated", func(t *testing.T) {
		g := NewWithT(t)

		opts := append(concurrencyRendererOptions(),
			gotemplate.WithCache(
				cache.WithKeyFunc(func(_ any) string {
					return "constant"
				}),
			),
			gotemplate.WithSourceAnnotations(true),
		)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   concurrencySourceFS("alpha"),
					Path: "templates/*.tpl",
				},
				{
					FS:   concurrencySourceFS("beta"),
					Path: "templates/*.tpl",
				},
			},
			opts...,
		)
		g.Expect(err).ToNot(HaveOccurred())

		// Seed both sources' caches with a known render.
		seed, err := renderer.Process(t.Context(), pkgtypes.Values{"Env": "seed"})
		g.Expect(err).ToNot(HaveOccurred())
		expectSourceResults(g, seed, "seed", "alpha", "beta")

		// A constant key intentionally ignores values: concurrent workers with
		// distinct values must all observe the seeded, per-source entries
		// without any cross-source mixing.
		envs := concurrencyWorkerEnvs()
		workerValues := concurrencyWorkerValues(envs)
		results, errs := renderConcurrently(t.Context(), renderer, workerValues)

		for i := range concurrentWorkers {
			g.Expect(errs[i]).ToNot(HaveOccurred())
			expectSourceResults(g, results[i], "seed", "alpha", "beta")
			g.Expect(renderOrigins(results[i])).To(Equal(map[string]string{
				"alpha-seed-config": pkgtypes.RenderOriginCache,
				"beta-seed-config":  pkgtypes.RenderOriginCache,
			}))
		}
	})

	t.Run("missing keys fail every concurrent render", func(t *testing.T) {
		g := NewWithT(t)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   concurrencySourceFS("alpha"),
					Path: "templates/*.tpl",
				},
			},
			concurrencyRendererOptions()...,
		)
		g.Expect(err).ToNot(HaveOccurred())

		// None of the workers provides Env, so missingkey=error must reject
		// every render with the same error.
		workerValues := make([]pkgtypes.Values, concurrentWorkers)
		for i := range concurrentWorkers {
			workerValues[i] = pkgtypes.Values{"Other": fmt.Sprintf("value-%d", i)}
		}

		_, errs := renderConcurrently(t.Context(), renderer, workerValues)

		for i := range concurrentWorkers {
			g.Expect(errs[i]).To(HaveOccurred())
			g.Expect(errs[i].Error()).To(ContainSubstring(`map has no entry for key "Env"`))
		}
	})
}

// renderConcurrently runs one Process call per worker values entry on the same
// renderer, releasing all workers together through a start barrier. Workers
// only collect their results; all assertions happen in the caller's goroutine.
func renderConcurrently(
	ctx context.Context,
	renderer *gotemplate.Renderer,
	values []pkgtypes.Values,
) ([][]unstructured.Unstructured, []error) {
	results := make([][]unstructured.Unstructured, len(values))
	errs := make([]error, len(values))

	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := range values {
		wg.Go(func() {
			<-start

			objects, err := renderer.Process(ctx, values[i])

			results[i] = objects
			errs[i] = err
		})
	}

	close(start)
	wg.Wait()

	return results, errs
}

// expectSourceResults asserts that a concurrent render contains exactly one
// ConfigMap per named source, each rendered with env through the pure shout
// function.
func expectSourceResults(
	g *WithT,
	objects []unstructured.Unstructured,
	env string,
	sourceNames ...string,
) {
	g.Expect(objects).To(HaveLen(len(sourceNames)))

	expected := make(map[string]string, len(sourceNames))
	for _, sourceName := range sourceNames {
		expected[sourceName+"-"+env+"-config"] = strings.ToUpper(env)
	}

	g.Expect(concurrencyRenderedValues(objects)).To(Equal(expected))
}

// concurrencyRenderedValues maps each rendered ConfigMap name to its
// data.rendered value so assertions can verify request-specific output.
func concurrencyRenderedValues(objects []unstructured.Unstructured) map[string]string {
	rendered := make(map[string]string, len(objects))

	for _, obj := range objects {
		value, _, _ := unstructured.NestedString(obj.Object, "data", "rendered")
		rendered[obj.GetName()] = value
	}

	return rendered
}

// mutateNestedContent mutates nested fields of a rendered result to prove that
// results own their data. It must only be called on results that no worker
// goroutine can still reach.
func mutateNestedContent(g *WithT, objects []unstructured.Unstructured) {
	g.Expect(objects).ToNot(BeEmpty())

	objects[0].SetName("mutated-name")

	err := unstructured.SetNestedField(objects[0].Object, "mutated", "data", "rendered")
	g.Expect(err).ToNot(HaveOccurred())
}
