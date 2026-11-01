package gotemplate_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing/fstest"

	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

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

	extractRendered := jqmatcher.Extract(`.data.rendered`)

	for _, obj := range objects {
		value, _ := extractRendered(obj.Object)
		entry, _ := value.(string)
		rendered[obj.GetName()] = entry
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
