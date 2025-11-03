package gotemplate

import (
	"k8s.io/apimachinery/pkg/util/dump"
)

// TemplateSpec contains the data used to generate cache keys for rendered templates.
type TemplateSpec struct {
	Path   string
	Values any
}

// CacheKeyFunc generates a cache key from template specification.
type CacheKeyFunc func(TemplateSpec) string

// DefaultCacheKey returns a CacheKeyFunc that uses reflection-based hashing of all template
// specification fields. This is the safest option but may be slower for large value structures.
//
// Security Considerations:
// Cache keys are generated from template values which may contain sensitive data such as
// passwords, API tokens, or other secrets. The resulting hash is deterministic and could
// potentially leak information if logged or exposed. For templates with sensitive values:
//   - Avoid logging cache keys in production environments
//   - Consider using FastCacheKey() or PathOnlyCacheKey() which ignore values
//   - Implement a custom CacheKeyFunc that excludes sensitive fields
//
// Example with sensitive data:
//
//	// If your values contain secrets, consider alternative cache key strategies
//	renderer := gotemplate.New(sources, gotemplate.WithCacheKeyFunc(gotemplate.FastCacheKey()))
func DefaultCacheKey() CacheKeyFunc {
	return func(spec TemplateSpec) string {
		return dump.ForHash(spec)
	}
}

// FastCacheKey returns a CacheKeyFunc that generates keys based only on template path,
// ignoring values. Use this when values don't affect the rendered output, when performance
// is critical, or when values may contain sensitive data that should not be included in
// cache keys.
func FastCacheKey() CacheKeyFunc {
	return func(spec TemplateSpec) string {
		return spec.Path
	}
}

// PathOnlyCacheKey returns a CacheKeyFunc that generates keys based only on template path.
// Use this when rendering the same templates multiple times with identical values, or when you want
// maximum cache hit rates regardless of values.
func PathOnlyCacheKey() CacheKeyFunc {
	return func(spec TemplateSpec) string {
		return spec.Path
	}
}
