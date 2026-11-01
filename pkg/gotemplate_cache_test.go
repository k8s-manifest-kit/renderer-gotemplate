// Renderer-level cache behavior: per-source output isolation, cache-hit
// render origins, and post-renderer reapplication through
// gotemplate.New and Process. Unit tests of the cache constructor and its
// key wiring live in pkg/cache.

package gotemplate_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/labels"
	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	utilcache "github.com/k8s-manifest-kit/pkg/util/cache"
	"github.com/rs/xid"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"
	"github.com/k8s-manifest-kit/renderer-gotemplate/pkg/cache"

	. "github.com/onsi/gomega"
)

// TestSourceCacheIsolation verifies that registered sources never share cached
// render output, even when their cache keys collide.
func TestSourceCacheIsolation(t *testing.T) {

	t.Run("should isolate same-path sources with equal values", func(t *testing.T) {
		g := NewWithT(t)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   isolatedSourceFS("templates", "alpha"),
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Env": "prod",
					}),
				},
				{
					FS:   isolatedSourceFS("templates", "beta"),
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Env": "prod",
					}),
				},
			},
			gotemplate.WithCache(),
			gotemplate.WithSourceAnnotations(true),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// Cold render: both sources execute and contribute their own object.
		cold, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configMapSources(cold)).To(Equal(map[string]string{
			"alpha-prod-config": "alpha",
			"beta-prod-config":  "beta",
		}))
		g.Expect(renderOrigins(cold)).To(Equal(map[string]string{
			"alpha-prod-config": pkgtypes.RenderOriginLive,
			"beta-prod-config":  pkgtypes.RenderOriginLive,
		}))

		// Warm render: both sources hit their own cache entry.
		warm, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configMapSources(warm)).To(Equal(map[string]string{
			"alpha-prod-config": "alpha",
			"beta-prod-config":  "beta",
		}))
		g.Expect(renderOrigins(warm)).To(Equal(map[string]string{
			"alpha-prod-config": pkgtypes.RenderOriginCache,
			"beta-prod-config":  pkgtypes.RenderOriginCache,
		}))
	})

	t.Run("should not merge sources through a constant custom key", func(t *testing.T) {
		g := NewWithT(t)

		var keys []any

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   isolatedSourceFS("alpha", "alpha"),
					Path: "alpha/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Env": "prod",
					}),
				},
				{
					FS:   isolatedSourceFS("beta", "beta"),
					Path: "beta/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Env": "dev",
					}),
				},
			},
			gotemplate.WithCache(
				utilcache.WithKeyFunc(func(key any) string {
					keys = append(keys, key)

					return "constant"
				}),
			),
			gotemplate.WithSourceAnnotations(true),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// Cold render: the constant key must not make the sources share output.
		cold, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configMapSources(cold)).To(Equal(map[string]string{
			"alpha-prod-config": "alpha",
			"beta-dev-config":   "beta",
		}))
		g.Expect(renderOrigins(cold)).To(Equal(map[string]string{
			"alpha-prod-config": pkgtypes.RenderOriginLive,
			"beta-dev-config":   pkgtypes.RenderOriginLive,
		}))

		// Warm render: the constant key still hits within each source's own cache.
		warm, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configMapSources(warm)).To(Equal(map[string]string{
			"alpha-prod-config": "alpha",
			"beta-dev-config":   "beta",
		}))
		g.Expect(renderOrigins(warm)).To(Equal(map[string]string{
			"alpha-prod-config": pkgtypes.RenderOriginCache,
			"beta-dev-config":   pkgtypes.RenderOriginCache,
		}))

		// The key function still receives a TemplateSpec for each source.
		paths := make([]string, 0, len(keys))
		for _, key := range keys {
			spec, ok := key.(cache.TemplateSpec)
			g.Expect(ok).To(BeTrue(), "key function should receive a TemplateSpec, got %T", key)
			paths = append(paths, spec.Path)
		}

		g.Expect(paths).To(ContainElement("alpha/*.tpl"))
		g.Expect(paths).To(ContainElement("beta/*.tpl"))
	})

	t.Run("should reapply source post-renderers and clone cached nested objects", func(t *testing.T) {
		g := NewWithT(t)

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   isolatedSourceFS("templates", "alpha"),
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Env": "prod",
					}),
					PostRenderers: []pkgtypes.PostRenderer{
						pkgtypes.TransformerAsPostRenderer(labels.Set(map[string]string{
							"team": "isolation",
						})),
					},
				},
			},
			gotemplate.WithCache(),
			gotemplate.WithSourceAnnotations(true),
		)
		g.Expect(err).ToNot(HaveOccurred())

		live, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(live).To(HaveLen(1))
		g.Expect(live[0].GetLabels()).To(Equal(map[string]string{"team": "isolation"}))

		// Mutate a nested field of the returned object.
		live[0].SetLabels(map[string]string{"mutated": "true"})

		warm, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(warm).To(HaveLen(1))

		// Cache hit: independent nested objects, correct source annotations,
		// and the source post-renderer reapplied.
		g.Expect(warm[0].GetLabels()).To(Equal(map[string]string{"team": "isolation"}))
		g.Expect(warm[0].GetAnnotations()).To(HaveKeyWithValue(
			pkgtypes.AnnotationRenderOrigin,
			pkgtypes.RenderOriginCache,
		))
		g.Expect(warm[0].GetAnnotations()).To(HaveKeyWithValue(
			pkgtypes.AnnotationSourcePath,
			"templates/*.tpl",
		))
	})

	t.Run("should keep parsed templates after the first successful load", func(t *testing.T) {
		g := NewWithT(t)

		fs := isolatedSourceFS("templates", "alpha")

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Env": "prod",
					}),
				},
			},
			gotemplate.WithCache(),
		)
		g.Expect(err).ToNot(HaveOccurred())

		live, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configMapSources(live)).To(Equal(map[string]string{
			"alpha-prod-config": "alpha",
		}))

		// Templates are a first-success snapshot: later filesystem changes are not
		// reparsed, even when the output cache misses after a render-time values change.
		fs["templates/config.yaml.tpl"].Data = []byte(isolatedSourceTemplate("changed"))

		miss, err := renderer.Process(t.Context(), map[string]any{"Env": "dev"})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(configMapSources(miss)).To(Equal(map[string]string{
			"alpha-dev-config": "alpha",
		}))
	})
}

func TestCacheIntegration(t *testing.T) {

	t.Run("should cache identical renders", func(t *testing.T) {
		g := NewWithT(t)
		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "cache-app",
					"Component": "frontend",
				}),
			},
		},
			gotemplate.WithCache(),
			gotemplate.WithSourceAnnotations(true),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render - cache miss
		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result1).ToNot(BeEmpty())
		for _, obj := range result1 {
			g.Expect(obj.GetAnnotations()).Should(HaveKeyWithValue(
				pkgtypes.AnnotationRenderOrigin,
				pkgtypes.RenderOriginLive,
			))
		}

		// Second render - cache hit
		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result2).To(HaveLen(len(result1)))

		for _, obj := range result2 {
			g.Expect(obj.GetAnnotations()).Should(HaveKeyWithValue(
				pkgtypes.AnnotationRenderOrigin,
				pkgtypes.RenderOriginCache,
			))
		}

		// A warm hit is origin-only: source fields and the exact hash bytes
		// stored on the live render are served unchanged.
		for i := range result1 {
			g.Expect(result2[i].GetAnnotations()).To(HaveKeyWithValue(
				pkgtypes.AnnotationSourceType,
				"gotemplate",
			))
			g.Expect(result2[i].GetAnnotations()).To(HaveKeyWithValue(
				pkgtypes.AnnotationSourcePath,
				"templates/*.tpl",
			))
			g.Expect(result2[i].GetAnnotations()).To(HaveKeyWithValue(
				pkgtypes.AnnotationContentHash,
				result1[i].GetAnnotations()[pkgtypes.AnnotationContentHash],
			))
		}
	})

	t.Run("should miss cache on different values", func(t *testing.T) {
		g := NewWithT(t)
		callCount := 0
		dynamicValues := func(_ context.Context) (pkgtypes.Values, error) {
			callCount++

			return pkgtypes.Values{
				"Repo":      xid.New().String(),
				"Component": "frontend",
			}, nil
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path:   "templates/*.tpl",
				Values: dynamicValues,
			},
		},
			gotemplate.WithCache(),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render
		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result1).ToNot(BeEmpty())

		// Second render with different values - cache miss
		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result2).ToNot(BeEmpty())

		// Values function should be called twice (no cache hits)
		g.Expect(callCount).To(Equal(2))
	})

	t.Run("should work with cache disabled", func(t *testing.T) {
		g := NewWithT(t)
		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS: fstest.MapFS{
						"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
					},
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Repo":      "no-cache-app",
						"Component": "frontend",
					}),
				},
			},
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render
		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result1).ToNot(BeEmpty())

		// Second render - should work even without cache
		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result2).To(HaveLen(len(result1)))
	})

	t.Run("should return clones from cache", func(t *testing.T) {
		g := NewWithT(t)
		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "clone-app",
					"Component": "frontend",
				}),
			},
		},
			gotemplate.WithCache(),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render
		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result1).ToNot(BeEmpty())

		// Modify first result
		if len(result1) > 0 {
			result1[0].SetName("modified-name")
		}

		// Second render - should not be affected by modification
		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result2).ToNot(BeEmpty())

		if len(result2) > 0 {
			g.Expect(result2[0].GetName()).ToNot(Equal("modified-name"))
		}
	})

	t.Run("should use custom cache key function", func(t *testing.T) {
		g := NewWithT(t)

		// Use FastCacheKey which ignores values
		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "custom-key-test",
					"Component": "frontend",
					"Port":      8080,
				}),
			},
		},
			gotemplate.WithCache(
				utilcache.WithKeyFunc(func(key any) string {
					if spec, ok := key.(cache.TemplateSpec); ok {
						return spec.Path
					}

					return utilcache.DefaultKeyFunc(key)
				}),
			),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render - cache miss
		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result1).To(HaveLen(1))

		// Second render with DIFFERENT values but using FastCacheKey
		// ignores values, so this should be a cache hit
		result2, err := renderer.Process(t.Context(), map[string]any{
			"Repo":      "different-app",
			"Component": "backend",
			"Port":      9090,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result2).To(HaveLen(len(result1)))

		// Results should be identical despite different values (cache hit)
		for i := range result1 {
			g.Expect(result2[i]).To(Equal(result1[i]))
		}
	})
}
