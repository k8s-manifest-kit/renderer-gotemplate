package gotemplate_test

import (
	"context"
	"testing"
	"testing/fstest"

	pkgtypes "github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/rs/xid"

	gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"
)

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
				Values: func(_ context.Context) (pkgtypes.Values, error) {
					return pkgtypes.Values{
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
