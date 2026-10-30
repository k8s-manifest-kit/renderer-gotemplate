package gotemplate

import (
	"github.com/k8s-manifest-kit/pkg/util/cache"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TemplateSpec contains the data used to generate cache keys for rendered templates.
type TemplateSpec struct {
	Path   string
	Values any
}

// newCache creates a cache instance with GoTemplate-specific default KeyFunc.
// Every call returns an independent cache, so each registered source owns its
// entries regardless of the configured KeyFunc; key functions only discriminate
// within a single source.
func newCache(opts *cache.Options) cache.Interface[[]unstructured.Unstructured] {
	if opts == nil {
		return nil
	}

	co := *opts

	// Inject default KeyFunc for GoTemplate
	if co.KeyFunc == nil {
		co.KeyFunc = cache.DefaultKeyFunc
	}

	return cache.NewRenderCache(co)
}
