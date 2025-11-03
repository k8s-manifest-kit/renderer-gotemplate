package gotemplate

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"sync"

	"github.com/k8s-manifest-kit/engine/pkg/pipeline"
	"github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util"
	"github.com/k8s-manifest-kit/pkg/util/cache"
	"github.com/k8s-manifest-kit/pkg/util/k8s"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const rendererType = "gotemplate"

// Source represents the input for a GoTemplate rendering operation.
type Source struct {
	// FS is the filesystem containing template files.
	// Supports embedded filesystems via embed.FS or testing via fstest.MapFS.
	FS fs.FS

	// Path specifies the glob pattern to match template files.
	// Examples: "templates/*.tpl", "**/*.yaml.gotmpl"
	Path string

	// Values provides data to be substituted into templates during rendering.
	// Function is called during rendering to obtain dynamic values.
	// Accessible within templates via dot notation (e.g., {{ .FieldName }}).
	Values func(context.Context) (map[string]any, error)
}

// Renderer handles Go template rendering operations.
// It implements types.Renderer.
//
// Thread-safety: Renderer is safe for concurrent use. Multiple goroutines
// may call Process() concurrently on the same Renderer instance. Template parsing
// is protected by per-Source mutexes to ensure thread-safe lazy initialization.
type Renderer struct {
	inputs []*sourceHolder
	opts   RendererOptions
	cache  cache.Interface[[]unstructured.Unstructured]
}

// New creates a new GoTemplate Renderer with the given inputs and options.
func New(inputs []Source, opts ...RendererOption) (*Renderer, error) {
	rendererOpts := RendererOptions{
		Filters:      make([]types.Filter, 0),
		Transformers: make([]types.Transformer, 0),
	}

	for _, opt := range opts {
		opt.ApplyTo(&rendererOpts)
	}

	// Wrap sources in holders and validate
	holders := make([]*sourceHolder, len(inputs))
	for i := range inputs {
		holders[i] = &sourceHolder{
			Source: inputs[i],
			mu:     &sync.RWMutex{},
		}
		if err := holders[i].Validate(); err != nil {
			return nil, fmt.Errorf("validation failed for source with path %q: %w", inputs[i].Path, err)
		}
	}

	r := &Renderer{
		inputs: holders,
		opts:   rendererOpts,
		cache:  newCache(rendererOpts.CacheOptions),
	}

	return r, nil
}

// Process executes the rendering logic for all configured inputs.
// This method is safe for concurrent use.
func (r *Renderer) Process(ctx context.Context, renderTimeValues map[string]any) ([]unstructured.Unstructured, error) {
	allObjects := make([]unstructured.Unstructured, 0)

	for i := range r.inputs {
		objects, err := r.renderSingle(ctx, r.inputs[i], renderTimeValues)
		if err != nil {
			return nil, fmt.Errorf("error rendering gotemplate pattern %s: %w", r.inputs[i].Path, err)
		}

		// Apply renderer-level filters and transformers per-source for better error context
		transformed, err := pipeline.Apply(ctx, objects, r.opts.Filters, r.opts.Transformers)
		if err != nil {
			return nil, fmt.Errorf(
				"error applying filters/transformers to gotemplate pattern %s: %w",
				r.inputs[i].Path,
				err,
			)
		}

		allObjects = append(allObjects, transformed...)
	}

	return allObjects, nil
}

// Name returns the renderer type identifier.
func (r *Renderer) Name() string {
	return rendererType
}

func (r *Renderer) values(
	ctx context.Context,
	holder *sourceHolder,
	renderTimeValues map[string]any,
) (map[string]any, error) {
	sourceValues := map[string]any{}

	if holder.Values != nil {
		v, err := holder.Values(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get values for template pattern %q: %w",
				holder.Path,
				err,
			)
		}

		sourceValues = v
	}

	// Deep merge with render-time values taking precedence
	return util.DeepMerge(sourceValues, renderTimeValues), nil
}

// renderSingle performs the rendering for a single template input.
func (r *Renderer) renderSingle(
	ctx context.Context,
	holder *sourceHolder,
	renderTimeValues map[string]any,
) ([]unstructured.Unstructured, error) {
	// Parse templates if not already parsed (thread-safe lazy loading)
	templates, err := holder.LoadTemplates()
	if err != nil {
		return nil, err
	}

	// Get values dynamically (includes render-time values)
	values, err := r.values(ctx, holder, renderTimeValues)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get values for pattern %q: %w",
			holder.Path,
			err,
		)
	}

	spec := TemplateSpec{
		Path:   holder.Path,
		Values: values,
	}

	// Check cache (if enabled)
	if r.cache != nil {
		// ensure objects are evicted
		r.cache.Sync()

		if cached, found := r.cache.Get(spec); found {
			return cached, nil
		}
	}

	result := make([]unstructured.Unstructured, 0)

	var buf bytes.Buffer

	// Execute each template
	for _, t := range templates.Templates() {
		// Skip the root template
		if t.Name() == "" {
			continue
		}

		// Check for context cancellation
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("template rendering cancelled: %w", err)
		}

		buf.Reset()

		// Execute the template
		if err := t.Execute(&buf, values); err != nil {
			return nil, fmt.Errorf("failed to execute template %s: %w", t.Name(), err)
		}

		// Decode the rendered output into unstructured objects
		objs, err := k8s.DecodeYAML(buf.Bytes())
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML from template %s: %w", t.Name(), err)
		}

		// Add source annotations if enabled
		if r.opts.SourceAnnotations {
			for i := range objs {
				annotations := objs[i].GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}

				annotations[types.AnnotationSourceType] = rendererType
				annotations[types.AnnotationSourcePath] = holder.Path
				annotations[types.AnnotationSourceFile] = t.Name()

				objs[i].SetAnnotations(annotations)
			}
		}

		result = append(result, objs...)
	}

	// Cache result (if enabled)
	if r.cache != nil {
		r.cache.Set(spec, result)
	}

	return result, nil
}
