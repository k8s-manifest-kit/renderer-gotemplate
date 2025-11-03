package gotemplate

import (
	"fmt"

	engine "github.com/k8s-manifest-kit/engine/pkg"
)

// NewEngine creates an Engine configured with a single Go template renderer.
// This is a convenience function for simple Go template-only rendering scenarios.
//
// Example:
//
//	e, _ := gotemplate.NewEngine(gotemplate.Source{
//	    FS:   os.DirFS("/path/to/templates"),
//	    Path: "*.yaml.tmpl",
//	})
//	objects, _ := e.Render(ctx)
func NewEngine(source Source, opts ...RendererOption) (*engine.Engine, error) {
	sources := []Source{source}
	renderer, err := New(sources, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gotemplate renderer: %w", err)
	}

	e, err := engine.New(engine.WithRenderer(renderer))
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %w", err)
	}

	return e, nil
}
