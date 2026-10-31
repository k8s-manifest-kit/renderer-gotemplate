package gotemplate_test

import (
	"fmt"
	"testing"

	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util/cache"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

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
