package gotemplate_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	engine "github.com/k8s-manifest-kit/engine/pkg"
	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	utilcache "github.com/k8s-manifest-kit/pkg/util/cache"
	utilmaps "github.com/k8s-manifest-kit/pkg/util/maps"
	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

const mergeValuesTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  replicas: "{{ .replicaCount }}"
  tag: "{{ .image.tag }}"
  repository: "{{ .image.repository }}"`

const nilSourceValuesTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  env: "{{ .environment }}"`

const simpleKeyTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-config
data:
  value: "{{ .key }}"`

func TestRenderTimeValues(t *testing.T) {

	t.Run("should merge render-time values with source values", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml": &fstest.MapFile{
				Data: []byte(mergeValuesTemplate),
			},
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "*.yaml",
				Values: gotemplate.Values(map[string]any{
					"replicaCount": 1,
					"image": map[string]any{
						"tag": "v1.0",
					},
				}),
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())

		// Render-time values should override source values
		renderTimeValues := map[string]any{
			"replicaCount": 3,
			"image": map[string]any{
				"repository": "nginx",
			},
		}

		objects, err := renderer.Process(t.Context(), renderTimeValues)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).Should(HaveLen(1))

		// Check merged values
		g.Expect(objects[0].Object).Should(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["replicas"]`), Equal("3")),       // From render-time
			WithTransform(jqmatcher.Extract(`.data["tag"]`), Equal("v1.0")),         // From source (deep merge)
			WithTransform(jqmatcher.Extract(`.data["repository"]`), Equal("nginx")), // From render-time
		))
	})

	t.Run("should work with nil source values", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml": &fstest.MapFile{
				Data: []byte(nilSourceValuesTemplate),
			},
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "*.yaml",
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())

		renderTimeValues := map[string]any{
			"environment": "production",
		}

		objects, err := renderer.Process(t.Context(), renderTimeValues)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).Should(HaveLen(1))

		g.Expect(objects[0].Object).Should(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["env"]`), Equal("production")),
		))
	})

	t.Run("should work with empty render-time values", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml": &fstest.MapFile{
				Data: []byte(simpleKeyTemplate),
			},
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "*.yaml",
				Values: gotemplate.Values(map[string]any{
					"key": "source-value",
				}),
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).Should(HaveLen(1))

		g.Expect(objects[0].Object).Should(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["value"]`), Equal("source-value")),
		))
	})

	t.Run("should update cache key with render-time values", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml": &fstest.MapFile{
				Data: []byte(simpleKeyTemplate),
			},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "*.yaml",
					Values: gotemplate.Values(map[string]any{
						"key": "base",
					}),
				},
			},
			gotemplate.WithCache(),
		)
		g.Expect(err).ShouldNot(HaveOccurred())

		// First render with render-time values
		renderValues1 := map[string]any{
			"key": "value1",
		}
		objects1, err := renderer.Process(t.Context(), renderValues1)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects1).Should(HaveLen(1))

		g.Expect(objects1[0].Object).Should(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["value"]`), Equal("value1")),
		))

		// Second render with different render-time values should not use cache
		renderValues2 := map[string]any{
			"key": "value2",
		}
		objects2, err := renderer.Process(t.Context(), renderValues2)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects2).Should(HaveLen(1))

		g.Expect(objects2[0].Object).Should(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["value"]`), Equal("value2")),
		))
	})
}

// TestProcessNilRenderer verifies that a typed-nil Renderer answers the
// sentinel instead of panicking, both when Process is called directly and
// when the renderer runs behind the engine, whose validation accepts the
// typed nil because Name is nil-safe.
func TestProcessNilRenderer(t *testing.T) {

	t.Run("should return the sentinel from a direct call", func(t *testing.T) {
		g := NewWithT(t)

		var renderer *gotemplate.Renderer

		objects, err := renderer.Process(t.Context(), nil)

		g.Expect(err).To(MatchError(pkgtypes.ErrRendererNil))
		g.Expect(objects).To(BeNil())
	})

	t.Run("should return the sentinel through the engine", func(t *testing.T) {
		g := NewWithT(t)

		var renderer *gotemplate.Renderer

		e, err := engine.New(engine.WithRenderer(renderer))
		g.Expect(err).ToNot(HaveOccurred(), "engine construction accepts the typed-nil built-in")

		objects, err := e.Render(t.Context())

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, pkgtypes.ErrRendererNil)).To(BeTrue())
		g.Expect(objects).To(BeNil())
	})
}

// TestProcessRejectsCyclicRenderTimeValues verifies that direct Process
// calls reject supported map and slice cycles in render-time values before
// the per-source deep clone, naming the source's pattern, and that the
// renderer stays usable afterwards.
func TestProcessRejectsCyclicRenderTimeValues(t *testing.T) {

	newRenderer := func(t *testing.T) *gotemplate.Renderer {
		t.Helper()

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(simpleKeyTemplate)}},
				Path: "*.yaml",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		return renderer
	}

	t.Run("should reject a self-referential map value", func(t *testing.T) {
		g := NewWithT(t)

		cyclic := map[string]any{}
		cyclic["self"] = cyclic

		renderer := newRenderer(t)

		objects, err := renderer.Process(t.Context(), map[string]any{"cyclic": cyclic})

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, utilmaps.ErrCycle)).To(BeTrue())
		g.Expect(err.Error()).To(ContainSubstring("cyclic render-time values"))
		g.Expect(err.Error()).To(ContainSubstring("*.yaml"))
		g.Expect(objects).To(BeNil())
	})

	t.Run("should reject a self-referential slice value", func(t *testing.T) {
		g := NewWithT(t)

		cyclic := make([]any, 1)
		cyclic[0] = cyclic

		renderer := newRenderer(t)

		objects, err := renderer.Process(t.Context(), map[string]any{"cyclic": cyclic})

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, utilmaps.ErrCycle)).To(BeTrue())
		g.Expect(objects).To(BeNil())
	})

	t.Run("should keep the renderer usable after a rejection", func(t *testing.T) {
		g := NewWithT(t)

		cyclic := map[string]any{}
		cyclic["self"] = cyclic

		renderer := newRenderer(t)

		_, err := renderer.Process(t.Context(), map[string]any{"cyclic": cyclic})
		g.Expect(err).To(HaveOccurred())

		objects, err := renderer.Process(t.Context(), map[string]any{"key": "value"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objects).To(HaveLen(1))
	})
}

// TestValuesCallbackRejectsCyclicSourceValues verifies that resolved source
// values are validated before the merge on cold and warm output-cache paths,
// that an overlay replacing the cyclic branch does not rescue the input, and
// that the error keeps the source context and the shared sentinel.
func TestValuesCallbackRejectsCyclicSourceValues(t *testing.T) {

	newCyclicValues := func() map[string]any {
		cyclic := map[string]any{}
		cyclic["self"] = cyclic

		return map[string]any{"cyclic": cyclic}
	}

	t.Run("should reject a cyclic callback result on a cold cache", func(t *testing.T) {
		g := NewWithT(t)

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:     fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(simpleKeyTemplate)}},
				Path:   "*.yaml",
				Values: gotemplate.Values(newCyclicValues()),
			},
		})
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, utilmaps.ErrCycle)).To(BeTrue())
		g.Expect(err.Error()).To(ContainSubstring("cyclic source values"))
		g.Expect(err.Error()).To(ContainSubstring("*.yaml"))
		g.Expect(objects).To(BeNil())
	})

	t.Run("should reject a cyclic callback result on a warm cache", func(t *testing.T) {
		g := NewWithT(t)

		current := map[string]any{"key": "cached-value"}
		callbackInvocations := 0

		// The constant cache key keeps the earlier successful entry usable
		// for any later render of this source, so the rejection below
		// cannot be explained by a key change: values resolve and validate
		// before the cache is consulted.
		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(simpleKeyTemplate)}},
					Path: "*.yaml",
					Values: func(_ context.Context) (pkgtypes.Values, error) {
						callbackInvocations++

						return current, nil
					},
				},
			},
			gotemplate.WithCache(utilcache.WithKeyFunc(func(_ any) string { return "constant" })),
			gotemplate.WithSourceAnnotations(true),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render populates the output cache, second render proves it
		// is warm, and both resolve the callback.
		live, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(renderOrigins(live)).To(Equal(map[string]string{"test-config": pkgtypes.RenderOriginLive}))

		cached, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(renderOrigins(cached)).To(Equal(map[string]string{"test-config": pkgtypes.RenderOriginCache}))
		g.Expect(callbackInvocations).To(Equal(2), "values resolve on warm-cache hits")

		// The callback now returns a cyclic tree: the usable cached entry
		// must not bypass validation, and the rejection happens before the
		// merge and before the cache could serve the entry.
		current = newCyclicValues()

		objects, err := renderer.Process(t.Context(), nil)

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, utilmaps.ErrCycle)).To(BeTrue())
		g.Expect(callbackInvocations).To(Equal(3))
		g.Expect(objects).To(BeNil())
	})

	t.Run("should reject a cyclic branch that the overlay replaces", func(t *testing.T) {
		g := NewWithT(t)

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:     fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(simpleKeyTemplate)}},
				Path:   "*.yaml",
				Values: gotemplate.Values(newCyclicValues()),
			},
		})
		g.Expect(err).ToNot(HaveOccurred())

		// The render-time overlay replaces the cyclic branch with a scalar,
		// so a merge-time check would never see the cycle; the source tree
		// is still rejected whole before the merge runs.
		objects, err := renderer.Process(t.Context(), map[string]any{"cyclic": "safe-string"})

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, utilmaps.ErrCycle)).To(BeTrue())
		g.Expect(objects).To(BeNil())
	})
}

// TestDeselectedSourceSkipsValueValidation verifies that a source rejected
// by its selector neither invokes its values callback nor validates the
// render-time values: per-source value boundaries apply to selected sources
// only, and the engine's global rejection is a separate boundary.
func TestDeselectedSourceSkipsValueValidation(t *testing.T) {

	t.Run("should not validate values or invoke callbacks for deselected sources", func(t *testing.T) {
		g := NewWithT(t)

		cyclic := map[string]any{}
		cyclic["self"] = cyclic

		callbackInvocations := 0

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(simpleKeyTemplate)}},
					Path: "*.yaml",
					Values: func(_ context.Context) (pkgtypes.Values, error) {
						callbackInvocations++

						return map[string]any{"cyclic": cyclic}, nil
					},
				},
			},
			gotemplate.WithSourceSelector(func(_ context.Context, _ gotemplate.Source) (bool, error) {
				return false, nil
			}),
		)
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), map[string]any{"cyclic": cyclic})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objects).To(BeEmpty())
		g.Expect(callbackInvocations).To(BeZero(), "deselected sources must not invoke their callbacks")
	})
}

// TestValuesSharedReferencesRender verifies that shared references and a
// slice holding a prefix of itself — aliasing without cycles — render
// correctly through a template that consumes only safe scalar leaves.
func TestValuesSharedReferencesRender(t *testing.T) {

	t.Run("should render shared references and overlapping prefixes", func(t *testing.T) {
		g := NewWithT(t)

		shared := map[string]any{"tag": "v1.0", "repository": "nginx"}

		items := make([]any, 2)
		items[0] = shared
		items[1] = items[:1]

		values := map[string]any{
			"replicaCount": 3,
			"image":        shared,
			"reused":       shared,
			"items":        items,
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(mergeValuesTemplate)}},
				Path: "*.yaml",
			},
		})
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), values)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objects).To(HaveLen(1))
		g.Expect(objects[0].Object).Should(WithTransform(jqmatcher.Extract(`.data["replicas"]`), Equal("3")))
	})
}

// TestSourcePostRendererChainIndex verifies that a failing source-specific
// post-renderer reports the adopted engine chain index while preserving the
// cause and the source's pattern context.
func TestSourcePostRendererChainIndex(t *testing.T) {

	t.Run("should report the chain index with pattern context", func(t *testing.T) {
		g := NewWithT(t)

		errHook := errors.New("hook failed")

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fstest.MapFS{"template.yaml": &fstest.MapFile{Data: []byte(simpleKeyTemplate)}},
				Path: "*.yaml",
				PostRenderers: []pkgtypes.PostRenderer{
					func(_ context.Context, _ []unstructured.Unstructured) ([]unstructured.Unstructured, error) {
						return nil, errHook
					},
				},
			},
		})
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), map[string]any{"key": "value"})

		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, errHook)).To(BeTrue(), "the hook cause stays discoverable")
		g.Expect(err.Error()).To(ContainSubstring("post-renderer error at chain index 0"))
		g.Expect(err.Error()).To(ContainSubstring("source post-renderer error"))
		g.Expect(err.Error()).To(ContainSubstring("*.yaml"))
		g.Expect(objects).To(BeNil())
	})
}
