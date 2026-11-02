package gotemplate

import (
	"maps"
	"text/template"

	"github.com/k8s-manifest-kit/engine/pkg/types"
	"github.com/k8s-manifest-kit/pkg/util/cache"
	"github.com/k8s-manifest-kit/pkg/util/option"
)

// RendererOption is a generic option for RendererOptions.
type RendererOption = option.Option[RendererOptions]

// RendererOptions is a struct-based option that can set multiple renderer options at once.
//
// Options apply left to right: each option sees the state left by the ones
// before it. When RendererOptions is applied through ApplyTo, fields divide
// into replacing, preserving, and merging groups:
//
//   - Filters, Transformers, PostRenderers, and SourceSelectors replace the
//     target's slices, including when nil — a struct clears hooks and
//     selectors that earlier options added.
//   - SourceAnnotations and ContentHash replace the target's booleans,
//     including when false.
//   - A nil CacheOptions preserves the target's cache configuration; a
//     non-nil CacheOptions applies its own field rules to the target's cache
//     options, creating them if needed.
//   - Funcs merge rather than replace: entries are copied into the target's
//     function map, functions with matching names overwrite, and an empty or
//     nil Funcs leaves previously registered functions untouched.
//
// Functional options change only their own field or append to a single
// slice, so they compose with anything applied before or after. Note the
// difference from a fresh constructor: New starts from defaults with content
// hashing enabled, caching disabled, and no template functions, while
// applying a RendererOptions struct replaces the hook, selector, and boolean
// fields outright — a caller who wants content hashing after a struct must
// set ContentHash: true in the struct (or follow it with
// WithContentHash(true)).
type RendererOptions struct {
	// Filters are renderer-specific filters applied during Process().
	Filters []types.Filter

	// Transformers are renderer-specific transformers applied during Process().
	Transformers []types.Transformer

	// PostRenderers are renderer-specific post-renderers applied during Process().
	PostRenderers []types.PostRenderer

	// SourceSelectors are renderer-specific source selectors evaluated before rendering each source.
	SourceSelectors []SourceSelector

	// CacheOptions holds cache configuration. nil = caching disabled.
	// Applying a RendererOptions struct with a nil CacheOptions preserves
	// the previously configured cache (including disabled); a non-nil value
	// applies its field rules on top of the previous configuration.
	CacheOptions *cache.Options

	// SourceAnnotations enables automatic addition of source tracking annotations.
	// Applying a RendererOptions struct replaces this field, including with false.
	SourceAnnotations bool

	// ContentHash enables automatic addition of a SHA-256 content hash annotation.
	// Default: true (enabled) in a fresh constructor; applying a
	// RendererOptions struct replaces this field, including with false, so a
	// struct that wants hashing must set it to true explicitly.
	ContentHash bool

	// Funcs are renderer-level template functions available to all sources.
	// Applying a RendererOptions struct merges this map into the previously
	// registered functions — matching names overwrite, and an empty or nil
	// map keeps the existing functions.
	Funcs template.FuncMap
}

// ApplyTo applies the renderer options to the target configuration.
//
// Application follows the composition rules documented on RendererOptions:
// hooks, selectors, and booleans replace their target fields (including
// nil and false), a nil CacheOptions preserves the target's cache
// configuration while a non-nil one applies its field rules on top, and
// Funcs entries merge into the target's function map with matching names
// overwriting.
func (opts RendererOptions) ApplyTo(target *RendererOptions) {
	target.Filters = opts.Filters
	target.Transformers = opts.Transformers
	target.PostRenderers = opts.PostRenderers
	target.SourceSelectors = opts.SourceSelectors

	if opts.CacheOptions != nil {
		if target.CacheOptions == nil {
			target.CacheOptions = &cache.Options{}
		}
		opts.CacheOptions.ApplyTo(target.CacheOptions)
	}

	target.SourceAnnotations = opts.SourceAnnotations
	target.ContentHash = opts.ContentHash

	if len(opts.Funcs) > 0 {
		if target.Funcs == nil {
			target.Funcs = make(template.FuncMap, len(opts.Funcs))
		}

		maps.Copy(target.Funcs, opts.Funcs)
	}
}

// WithFilter adds a renderer-specific filter to this GoTemplate renderer's processing chain.
func WithFilter(f types.Filter) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.Filters = append(opts.Filters, f)
	})
}

// WithTransformer adds a renderer-specific transformer to this GoTemplate renderer's processing chain.
func WithTransformer(t types.Transformer) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.Transformers = append(opts.Transformers, t)
	})
}

// WithPostRenderer adds a renderer-specific post-renderer to this GoTemplate renderer's processing chain.
func WithPostRenderer(p types.PostRenderer) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.PostRenderers = append(opts.PostRenderers, p)
	})
}

// WithSourceSelector adds a source selector to this GoTemplate renderer.
// Source selectors are evaluated before rendering each source. If any selector
// returns false, the source is skipped entirely.
func WithSourceSelector(s SourceSelector) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.SourceSelectors = append(opts.SourceSelectors, s)
	})
}

// WithCache enables render result caching with the specified options.
func WithCache(opts ...cache.Option) RendererOption {
	return option.FunctionalOption[RendererOptions](func(rendererOpts *RendererOptions) {
		if rendererOpts.CacheOptions == nil {
			rendererOpts.CacheOptions = &cache.Options{}
		}

		for _, opt := range opts {
			opt.ApplyTo(rendererOpts.CacheOptions)
		}
	})
}

// WithSourceAnnotations enables or disables automatic addition of source tracking annotations.
func WithSourceAnnotations(enabled bool) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.SourceAnnotations = enabled
	})
}

// WithContentHash enables or disables automatic addition of a SHA-256 content hash annotation.
func WithContentHash(enabled bool) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		opts.ContentHash = enabled
	})
}

// WithFunc adds a renderer-level template function available to all sources.
func WithFunc(name string, fn any) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		if opts.Funcs == nil {
			opts.Funcs = template.FuncMap{}
		}

		opts.Funcs[name] = fn
	})
}

// WithFuncs adds multiple renderer-level template functions available to all sources.
func WithFuncs(funcs template.FuncMap) RendererOption {
	return option.FunctionalOption[RendererOptions](func(opts *RendererOptions) {
		if opts.Funcs == nil {
			opts.Funcs = template.FuncMap{}
		}

		maps.Copy(opts.Funcs, funcs)
	})
}
