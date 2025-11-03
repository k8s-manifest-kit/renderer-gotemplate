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
