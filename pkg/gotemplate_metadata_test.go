package gotemplate_test

import (
	"context"
	"testing"
	"testing/fstest"

	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util/k8s"
	"github.com/rs/xid"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

// annotatedConfigMapTemplate emits an object carrying its own annotations,
// including stale kit-managed keys, to characterize how renderer annotation
// phases interact with pre-existing metadata.
const annotatedConfigMapTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Repo }}-annotated
  annotations:
    existing: annotation
    manifests.k8s-manifests-kit/content.hash: sha256:stale
    manifests.k8s-manifests-kit/render.origin: stale
data:
  key: value
`

func renderTemplateSource(
	t *testing.T,
	template string,
	values func(context.Context) (pkgtypes.Values, error),
	opts ...gotemplate.RendererOption,
) []unstructured.Unstructured {
	t.Helper()

	g := NewWithT(t)

	renderer, err := gotemplate.New([]gotemplate.Source{{
		FS:     fstest.MapFS{"templates/template.yaml.tpl": &fstest.MapFile{Data: []byte(template)}},
		Path:   "templates/*.tpl",
		Values: values,
	}}, opts...)
	g.Expect(err).NotTo(HaveOccurred())

	objects, err := renderer.Process(t.Context(), nil)
	g.Expect(err).NotTo(HaveOccurred())

	return objects
}

func TestAnnotationApplication(t *testing.T) {
	values := gotemplate.Values(map[string]any{"Repo": "annotation-app", "Component": "frontend"})

	sourceKeys := map[string]string{
		pkgtypes.AnnotationSourceType: "gotemplate",
		pkgtypes.AnnotationSourcePath: "templates/*.tpl",
		pkgtypes.AnnotationSourceFile: "template.yaml.tpl",
	}

	t.Run("hash covers source annotations before render origin", func(t *testing.T) {
		g := NewWithT(t)

		bare := renderTemplateSource(t, podTemplate, values,
			gotemplate.WithContentHash(false))

		expected := bare[0].DeepCopy()
		expected.SetAnnotations(sourceKeys)

		annotated := renderTemplateSource(t, podTemplate, values,
			gotemplate.WithSourceAnnotations(true))

		annotations := annotated[0].GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue(
			pkgtypes.AnnotationContentHash,
			k8s.ContentHash(expected),
		))
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationRenderOrigin, pkgtypes.RenderOriginLive))
	})

	t.Run("full annotation map matches the expected map", func(t *testing.T) {
		g := NewWithT(t)

		bare := renderTemplateSource(t, podTemplate, values,
			gotemplate.WithContentHash(false))

		expected := bare[0].DeepCopy()
		expected.SetAnnotations(sourceKeys)

		annotated := renderTemplateSource(t, podTemplate, values,
			gotemplate.WithSourceAnnotations(true))

		g.Expect(annotated[0].GetAnnotations()).To(Equal(map[string]string{
			pkgtypes.AnnotationSourceType:   "gotemplate",
			pkgtypes.AnnotationSourcePath:   "templates/*.tpl",
			pkgtypes.AnnotationSourceFile:   "template.yaml.tpl",
			pkgtypes.AnnotationContentHash:  k8s.ContentHash(expected),
			pkgtypes.AnnotationRenderOrigin: pkgtypes.RenderOriginLive,
		}))
	})

	t.Run("pre-existing annotations are preserved and included in the hash input", func(t *testing.T) {
		g := NewWithT(t)

		bare := renderTemplateSource(t, annotatedConfigMapTemplate, values,
			gotemplate.WithContentHash(false))

		expected := bare[0].DeepCopy()
		expected.SetAnnotations(map[string]string{
			"existing":                      "annotation",
			pkgtypes.AnnotationContentHash:  "sha256:stale",
			pkgtypes.AnnotationRenderOrigin: "stale",
			pkgtypes.AnnotationSourceType:   "gotemplate",
			pkgtypes.AnnotationSourcePath:   "templates/*.tpl",
			pkgtypes.AnnotationSourceFile:   "template.yaml.tpl",
		})

		annotated := renderTemplateSource(t, annotatedConfigMapTemplate, values,
			gotemplate.WithSourceAnnotations(true))

		annotations := annotated[0].GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("existing", "annotation"))
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationContentHash, k8s.ContentHash(expected)))
		g.Expect(annotations).NotTo(HaveKeyWithValue(pkgtypes.AnnotationContentHash, "sha256:stale"))
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationRenderOrigin, pkgtypes.RenderOriginLive))
	})

	t.Run("source annotations without content hash leave no hash key", func(t *testing.T) {
		g := NewWithT(t)

		annotated := renderTemplateSource(t, podTemplate, values,
			gotemplate.WithSourceAnnotations(true),
			gotemplate.WithContentHash(false))

		annotations := annotated[0].GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationSourceType, "gotemplate"))
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationSourcePath, "templates/*.tpl"))
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationSourceFile, "template.yaml.tpl"))
		g.Expect(annotations).To(HaveKeyWithValue(pkgtypes.AnnotationRenderOrigin, pkgtypes.RenderOriginLive))
		g.Expect(annotations).NotTo(HaveKey(pkgtypes.AnnotationContentHash))
	})
}

func TestSourceAnnotations(t *testing.T) {

	t.Run("should add source annotations when enabled", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
			"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Repo":      "test-app",
						"Component": "frontend",
						"Port":      8080,
					}),
				},
			},
			gotemplate.WithSourceAnnotations(true),
		)
		g.Expect(err).ShouldNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).Should(HaveLen(2))

		// Verify all objects have source annotations
		for _, obj := range objects {
			annotations := obj.GetAnnotations()
			g.Expect(annotations).Should(HaveKeyWithValue(pkgtypes.AnnotationSourceType, "gotemplate"))
			g.Expect(annotations).Should(HaveKeyWithValue(
				pkgtypes.AnnotationRenderOrigin,
				pkgtypes.RenderOriginLive,
			))
			g.Expect(annotations).Should(HaveKeyWithValue(pkgtypes.AnnotationSourcePath, "templates/*.tpl"))
			g.Expect(annotations).Should(HaveKey(pkgtypes.AnnotationSourceFile))
			// File should be one of the template names (without directory path)
			g.Expect(annotations[pkgtypes.AnnotationSourceFile]).Should(
				Or(
					Equal("pod.yaml.tpl"),
					Equal("configmap.yaml.tpl"),
				),
			)
		}
	})

	t.Run("should not add source annotations when disabled", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Repo":      "test-app",
						"Component": "frontend",
					}),
				},
			},
		)
		g.Expect(err).ShouldNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).ShouldNot(BeEmpty())

		// Verify no source annotations are present
		for _, obj := range objects {
			annotations := obj.GetAnnotations()
			g.Expect(annotations).ShouldNot(HaveKey(pkgtypes.AnnotationSourceType))
			g.Expect(annotations).ShouldNot(HaveKey(pkgtypes.AnnotationSourcePath))
			g.Expect(annotations).ShouldNot(HaveKey(pkgtypes.AnnotationSourceFile))
		}
	})
}

func TestContentHash(t *testing.T) {

	t.Run("should add content hash annotation by default", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
			"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "hash-app",
					"Component": "frontend",
					"Port":      8080,
				}),
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).Should(HaveLen(2))

		for _, obj := range objects {
			annotations := obj.GetAnnotations()
			g.Expect(annotations).Should(HaveKey(pkgtypes.AnnotationContentHash))
			g.Expect(annotations[pkgtypes.AnnotationContentHash]).Should(MatchRegexp("^sha256:[0-9a-f]{64}$"))
		}
	})

	t.Run("should not add content hash when disabled", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Repo":      "hash-app",
						"Component": "frontend",
					}),
				},
			},
			gotemplate.WithContentHash(false),
		)
		g.Expect(err).ShouldNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).ShouldNot(BeEmpty())

		for _, obj := range objects {
			annotations := obj.GetAnnotations()
			g.Expect(annotations).ShouldNot(HaveKey(pkgtypes.AnnotationContentHash))
		}
	})

	t.Run("different objects should have different hashes", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
			"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
		}

		renderer, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "hash-app",
					"Component": "frontend",
					"Port":      8080,
				}),
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects).Should(HaveLen(2))

		hash0 := objects[0].GetAnnotations()[pkgtypes.AnnotationContentHash]
		hash1 := objects[1].GetAnnotations()[pkgtypes.AnnotationContentHash]
		g.Expect(hash0).ShouldNot(Equal(hash1))
	})

	t.Run("hash should change when template values change", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
		}

		r1, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "app-v1",
					"Component": "frontend",
				}),
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())
		objects1, err := r1.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())

		r2, err := gotemplate.New([]gotemplate.Source{
			{
				FS:   fs,
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "app-v2",
					"Component": "frontend",
				}),
			},
		})
		g.Expect(err).ShouldNot(HaveOccurred())
		objects2, err := r2.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())

		hash1 := objects1[0].GetAnnotations()[pkgtypes.AnnotationContentHash]
		hash2 := objects2[0].GetAnnotations()[pkgtypes.AnnotationContentHash]
		g.Expect(hash1).ShouldNot(BeEmpty())
		g.Expect(hash2).ShouldNot(BeEmpty())
		g.Expect(hash1).ShouldNot(Equal(hash2))
	})

	t.Run("cached results should preserve content hash", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "templates/*.tpl",
					Values: gotemplate.Values(map[string]any{
						"Repo":      "cache-hash-app",
						"Component": "frontend",
					}),
				},
			},
			gotemplate.WithCache(),
		)
		g.Expect(err).ShouldNot(HaveOccurred())

		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())

		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())

		for i := range result1 {
			hash1 := result1[i].GetAnnotations()[pkgtypes.AnnotationContentHash]
			hash2 := result2[i].GetAnnotations()[pkgtypes.AnnotationContentHash]
			g.Expect(hash1).ShouldNot(BeEmpty())
			g.Expect(hash1).Should(Equal(hash2))
		}
	})

	t.Run("cache miss on different values should produce different hashes", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
		}

		callCount := 0
		dynamicValues := func(_ context.Context) (pkgtypes.Values, error) {
			callCount++

			return pkgtypes.Values{
				"Repo":      xid.New().String(),
				"Component": "frontend",
			}, nil
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:     fs,
					Path:   "templates/*.tpl",
					Values: dynamicValues,
				},
			},
			gotemplate.WithCache(),
		)
		g.Expect(err).ShouldNot(HaveOccurred())

		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())

		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ShouldNot(HaveOccurred())

		hash1 := result1[0].GetAnnotations()[pkgtypes.AnnotationContentHash]
		hash2 := result2[0].GetAnnotations()[pkgtypes.AnnotationContentHash]
		g.Expect(hash1).ShouldNot(BeEmpty())
		g.Expect(hash2).ShouldNot(BeEmpty())
		g.Expect(hash1).ShouldNot(Equal(hash2))
	})
}
