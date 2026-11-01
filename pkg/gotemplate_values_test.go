package gotemplate_test

import (
	"testing"
	"testing/fstest"

	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

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
