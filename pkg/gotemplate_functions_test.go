package gotemplate_test

import (
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	jqmatcher "github.com/lburgazzoli/gomega-matchers/pkg/matchers/jq"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

	. "github.com/onsi/gomega"
)

const funcTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-funcs
data:
  value: "{{ shout .key }}"`

const yamlFuncTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-yaml
data:
  value: |
{{ toYAML .config | nindent 4 }}`

func TestTemplateFunctions(t *testing.T) {
	t.Run("should render with registered custom function", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml.tpl": &fstest.MapFile{
				Data: []byte(funcTemplate),
			},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "*.tpl",
					Values: gotemplate.Values(map[string]any{
						"key": "hello",
					}),
				},
			},
			gotemplate.WithFunc("shout", strings.ToUpper),
		)
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objects).To(HaveLen(1))

		g.Expect(objects[0].Object).To(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["value"]`), Equal("HELLO")),
		))
	})

	t.Run("should render with multiple registered functions", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml.tpl": &fstest.MapFile{
				Data: []byte(funcTemplate),
			},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "*.tpl",
					Values: gotemplate.Values(map[string]any{
						"key": "hello",
					}),
				},
			},
			gotemplate.WithFuncs(template.FuncMap{
				"noop": func(v string) string { return v },
			}),
			gotemplate.WithFunc("shout", strings.ToUpper),
		)
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objects).To(HaveLen(1))

		g.Expect(objects[0].Object).To(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["value"]`), Equal("HELLO")),
		))
	})

	t.Run("should fail when helper is not registered", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml.tpl": &fstest.MapFile{
				Data: []byte(yamlFuncTemplate),
			},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "*.tpl",
					Values: gotemplate.Values(map[string]any{
						"config": map[string]any{
							"enabled": true,
						},
					}),
				},
			},
		)
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring(`function "toYAML" not defined`))
		g.Expect(objects).To(BeEmpty())
	})

	t.Run("should render with registered convenience helpers", func(t *testing.T) {
		g := NewWithT(t)
		fs := fstest.MapFS{
			"template.yaml.tpl": &fstest.MapFile{
				Data: []byte(yamlFuncTemplate),
			},
		}

		renderer, err := gotemplate.New(
			[]gotemplate.Source{
				{
					FS:   fs,
					Path: "*.tpl",
					Values: gotemplate.Values(map[string]any{
						"config": map[string]any{
							"enabled": true,
							"ports":   []int{80, 443},
						},
					}),
				},
			},
			gotemplate.WithFuncs(template.FuncMap{
				"toYAML":  gotemplate.ToYAML,
				"indent":  gotemplate.Indent,
				"nindent": gotemplate.Nindent,
			}),
		)
		g.Expect(err).ToNot(HaveOccurred())

		objects, err := renderer.Process(t.Context(), nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(objects).To(HaveLen(1))

		g.Expect(objects[0].Object).To(And(
			jqmatcher.Match(`.data | type == "object" and all(.[]; type == "string")`),
			WithTransform(jqmatcher.Extract(`.data["value"]`), Equal("\nenabled: true\nports:\n- 80\n- 443")),
		))
	})
}
