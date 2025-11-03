package gotemplate_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/k8s-manifest-kit/engine/pkg/filter/meta/gvk"
	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/labels"
	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	"github.com/onsi/gomega/types"
	"github.com/rs/xid"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

const podTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Repo }}-pod
  labels:
    app: {{ .Repo }}
    component: {{ .Component }}
spec:
  containers:
  - name: nginx
    image: nginx:latest
`

const configMapTemplate = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Repo }}-config
  labels:
    app: {{ .Repo }}
    component: {{ .Component }}
data:
  config.yaml: |
    port: {{ .Port }}
`

const invalidTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .InvalidField }}-pod
`

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

func TestRenderer(t *testing.T) {

	tests := []struct {
		name          string
		data          gotemplate.Source
		opts          []gotemplate.RendererOption
		expectedCount int
		validation    types.GomegaMatcher
	}{
		{
			name: "should render single template",
			data: gotemplate.Source{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "test-app",
					"Component": "frontend",
				}),
			},
			expectedCount: 1,
			validation: And(
				jqmatcher.Match(`.kind == "Pod"`),
				jqmatcher.Match(`.metadata.name == "test-app-pod"`),
				jqmatcher.Match(`.metadata.labels["app"] == "test-app"`),
				jqmatcher.Match(`.metadata.labels["component"] == "frontend"`),
			),
		},
		{
			name: "should render multiple templates",
			data: gotemplate.Source{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
					"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "test-app",
					"Component": "frontend",
					"Port":      8080,
				}),
			},
			expectedCount: 2,
			validation: Or(
				And(
					jqmatcher.Match(`.kind == "Pod"`),
					jqmatcher.Match(`.metadata.name == "test-app-pod"`),
				),
				And(
					jqmatcher.Match(`.kind == "ConfigMap"`),
					jqmatcher.Match(`.metadata.name == "test-app-config"`),
					jqmatcher.Match(`.data["config.yaml"] == "port: 8080\n"`),
				),
			),
		},
		{
			name: "should handle invalid template",
			data: gotemplate.Source{
				FS: fstest.MapFS{
					"templates/invalid.yaml.tpl": &fstest.MapFile{Data: []byte(invalidTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo": "test-app",
				}),
			},
			expectedCount: 0,
			validation:    nil,
		},
		{
			name: "should apply filters",
			data: gotemplate.Source{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
					"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "test-app",
					"Component": "frontend",
					"Port":      8080,
				}),
			},
			opts: []gotemplate.RendererOption{
				gotemplate.WithFilter(gvk.Filter(corev1.SchemeGroupVersion.WithKind("Pod"))),
			},
			expectedCount: 1,
			validation: And(
				jqmatcher.Match(`.kind == "Pod"`),
				jqmatcher.Match(`.metadata.name == "test-app-pod"`),
			),
		},
		{
			name: "should apply transformers",
			data: gotemplate.Source{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "test-app",
					"Component": "frontend",
				}),
			},
			opts: []gotemplate.RendererOption{
				gotemplate.WithTransformer(labels.Set(map[string]string{
					"managed-by": "gotemplate-renderer",
					"env":        "test",
				})),
			},
			expectedCount: 1,
			validation: And(
				jqmatcher.Match(`.kind == "Pod"`),
				jqmatcher.Match(`.metadata.labels["managed-by"] == "gotemplate-renderer"`),
				jqmatcher.Match(`.metadata.labels["env"] == "test"`),
				jqmatcher.Match(`.metadata.labels["app"] == "test-app"`),
			),
		},
		{
			name: "should handle empty template",
			data: gotemplate.Source{
				FS:   fstest.MapFS{},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo": "test-app",
				}),
			},
			expectedCount: 0,
			validation:    nil,
		},
		{
			name: "should handle non-existent template",
			data: gotemplate.Source{
				FS: fstest.MapFS{
					"templates/other.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo": "test-app",
				}),
			},
			expectedCount: 0,
			validation:    nil,
		},
	}

	for i := range tests {
		tt := &tests[i]
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			sources := []gotemplate.Source{tt.data}
			renderer, err := gotemplate.New(sources, tt.opts...)
			g.Expect(err).ToNot(HaveOccurred())

			objects, err := renderer.Process(t.Context(), nil)

			if tt.validation == nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(objects).To(BeEmpty())

				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(objects).To(HaveLen(tt.expectedCount))

			for _, obj := range objects {
				g.Expect(obj.Object).To(tt.validation)
			}
		})
	}
}

func TestNew(t *testing.T) {

	tests := []struct {
		name        string
		inputs      []gotemplate.Source
		expectError bool
	}{
		{
			name: "should reject input without FS",
			inputs: []gotemplate.Source{{
				Path: "templates/*.yaml",
			}},
			expectError: true,
		},
		{
			name: "should reject input without path",
			inputs: []gotemplate.Source{{
				FS: fstest.MapFS{},
			}},
			expectError: true,
		},
		{
			name: "should accept valid input",
			inputs: []gotemplate.Source{{
				FS:   fstest.MapFS{},
				Path: "templates/*.yaml",
			}},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			renderer, err := gotemplate.New(tt.inputs)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(renderer).To(BeNil())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(renderer).ToNot(BeNil())
			}
		})
	}
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
		)
		g.Expect(err).ToNot(HaveOccurred())

		// First render - cache miss
		result1, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result1).ToNot(BeEmpty())

		// Second render - cache hit (should be identical)
		result2, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result2).To(HaveLen(len(result1)))

		// Results should be equal
		for i := range result1 {
			g.Expect(result2[i]).To(Equal(result1[i]))
		}
	})

	t.Run("should miss cache on different values", func(t *testing.T) {
		g := NewWithT(t)
		callCount := 0
		dynamicValues := func(_ context.Context) (map[string]any, error) {
			callCount++

			return map[string]any{
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
			gotemplate.WithCache(),
			gotemplate.WithCacheKeyFunc(gotemplate.FastCacheKey()),
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

func BenchmarkGoTemplateRenderWithoutCache(b *testing.B) {
	renderer, err := gotemplate.New([]gotemplate.Source{
		{
			FS: fstest.MapFS{
				"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
				"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
			},
			Path: "templates/*.tpl",
			Values: gotemplate.Values(map[string]any{
				"Repo":      "bench-app",
				"Component": "backend",
				"Port":      8080,
			}),
		},
	})
	if err != nil {
		b.Fatalf("failed to create renderer: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, err := renderer.Process(b.Context(), nil)
		if err != nil {
			b.Fatalf("failed to render: %v", err)
		}
	}
}

func BenchmarkGoTemplateRenderWithCache(b *testing.B) {
	renderer, err := gotemplate.New(
		[]gotemplate.Source{
			{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
					"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
				},
				Path: "templates/*.tpl",
				Values: gotemplate.Values(map[string]any{
					"Repo":      "bench-app",
					"Component": "backend",
					"Port":      8080,
				}),
			},
		},
		gotemplate.WithCache(),
	)
	if err != nil {
		b.Fatalf("failed to create renderer: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, err := renderer.Process(b.Context(), nil)
		if err != nil {
			b.Fatalf("failed to render: %v", err)
		}
	}
}

func BenchmarkGoTemplateRenderCacheMiss(b *testing.B) {
	renderer, err := gotemplate.New(
		[]gotemplate.Source{
			{
				FS: fstest.MapFS{
					"templates/pod.yaml.tpl":       &fstest.MapFile{Data: []byte(podTemplate)},
					"templates/configmap.yaml.tpl": &fstest.MapFile{Data: []byte(configMapTemplate)},
				},
				Path: "templates/*.tpl",
				Values: func(_ context.Context) (map[string]any, error) {
					return map[string]any{
						"Repo":      xid.New().String(),
						"Component": "backend",
						"Port":      8080,
					}, nil
				},
			},
		},
		gotemplate.WithCache(),
	)
	if err != nil {
		b.Fatalf("failed to create renderer: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, err := renderer.Process(b.Context(), nil)
		if err != nil {
			b.Fatalf("failed to render: %v", err)
		}
	}
}

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
		data, found, err := unstructured.NestedStringMap(objects[0].Object, "data")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(found).Should(BeTrue())
		g.Expect(data["replicas"]).Should(Equal("3"))       // From render-time
		g.Expect(data["tag"]).Should(Equal("v1.0"))         // From source (deep merge)
		g.Expect(data["repository"]).Should(Equal("nginx")) // From render-time
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

		data, found, err := unstructured.NestedStringMap(objects[0].Object, "data")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(found).Should(BeTrue())
		g.Expect(data["env"]).Should(Equal("production"))
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

		data, found, err := unstructured.NestedStringMap(objects[0].Object, "data")
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(found).Should(BeTrue())
		g.Expect(data["value"]).Should(Equal("source-value"))
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

		data1, _, _ := unstructured.NestedStringMap(objects1[0].Object, "data")
		g.Expect(data1["value"]).Should(Equal("value1"))

		// Second render with different render-time values should not use cache
		renderValues2 := map[string]any{
			"key": "value2",
		}
		objects2, err := renderer.Process(t.Context(), renderValues2)
		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(objects2).Should(HaveLen(1))

		data2, _, _ := unstructured.NestedStringMap(objects2[0].Object, "data")
		g.Expect(data2["value"]).Should(Equal("value2"))
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
