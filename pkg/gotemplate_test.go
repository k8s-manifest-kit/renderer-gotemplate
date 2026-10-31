package gotemplate_test

import (
	"testing"
	"testing/fstest"

	"github.com/k8s-manifest-kit/engine/pkg/filter/meta/gvk"
	"github.com/k8s-manifest-kit/engine/pkg/transformer/meta/labels"
	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"
	"github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

const invalidTemplate = `
apiVersion: v1
kind: Pod
metadata:
  name: {{ .InvalidField }}-pod
`

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
