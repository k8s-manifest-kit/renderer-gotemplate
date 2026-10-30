package gotemplate_test

import (
	"testing"

	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/labels"
	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util/cache"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

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
				cache.WithKeyFunc(func(key any) string {
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
			spec, ok := key.(gotemplate.TemplateSpec)
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
