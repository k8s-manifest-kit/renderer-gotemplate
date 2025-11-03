package gotemplate

import (
	"github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util"
	"github.com/k8s-manifest-kit/pkg/util/cache"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// RendererOption is a generic option for RendererOptions.
type RendererOption = util.Option[RendererOptions]

// RendererOptions is a struct-based option that can set multiple renderer options at once.
type RendererOptions struct {
	// Filters are renderer-specific filters applied during Process().
	Filters []types.Filter

	// Transformers are renderer-specific transformers applied during Process().
	Transformers []types.Transformer

	// Cache is a custom cache implementation for render results.
	Cache cache.Interface[[]unstructured.Unstructured]

	// SourceAnnotations enables automatic addition of source tracking annotations.
	SourceAnnotations bool
}

// ApplyTo applies the renderer options to the target configuration.
func (opts RendererOptions) ApplyTo(target *RendererOptions) {
	target.Filters = opts.Filters
	target.Transformers = opts.Transformers

	if opts.Cache != nil {
		target.Cache = opts.Cache
	}

	target.SourceAnnotations = opts.SourceAnnotations
}

// WithFilter adds a renderer-specific filter to this GoTemplate renderer's processing chain.
// Renderer-specific filters are applied during Process(), before results are returned to the engine.
// For engine-level filtering applied to all renderers, use engine.WithFilter.
func WithFilter(f types.Filter) RendererOption {
	return util.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.Filters = append(opts.Filters, f)
	})
}

// WithTransformer adds a renderer-specific transformer to this GoTemplate renderer's processing chain.
// Renderer-specific transformers are applied during Process(), before results are returned to the engine.
// For engine-level transformation applied to all renderers, use engine.WithTransformer.
func WithTransformer(t types.Transformer) RendererOption {
	return util.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.Transformers = append(opts.Transformers, t)
	})
}

// WithCache enables render result caching with the specified options.
// If no options are provided, uses default TTL of 5 minutes.
// By default, caching is NOT enabled.
func WithCache(opts ...cache.Option) RendererOption {
	return util.FunctionalOption[RendererOptions](func(rendererOpts *RendererOptions) {
		rendererOpts.Cache = cache.NewRenderCache(opts...)
	})
}

// WithSourceAnnotations enables or disables automatic addition of source tracking annotations.
// When enabled, the renderer adds metadata annotations to track the source type, path, and template file.
// Annotations added: manifests.k8s-manifests-lib/source.type, source.path, source.file.
// Default: false (disabled).
func WithSourceAnnotations(enabled bool) RendererOption {
	return util.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.SourceAnnotations = enabled
	})
}
