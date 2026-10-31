package cache

import (
	utilcache "github.com/k8s-manifest-kit/pkg/util/cache"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TemplateSpec contains the data used to generate cache keys for rendered templates.
type TemplateSpec struct {
	Path   string
	Values any
}

// New creates a cache instance with GoTemplate-specific default KeyFunc.
// Every call returns an independent cache, so each registered source owns its
// entries regardless of the configured KeyFunc; key functions only discriminate
// within a single source.
func New(opts *utilcache.Options) utilcache.Interface[[]unstructured.Unstructured] {
	if opts == nil {
		return nil
	}

	co := *opts

	// Inject default KeyFunc for GoTemplate
	if co.KeyFunc == nil {
		co.KeyFunc = utilcache.DefaultKeyFunc
	}

	return utilcache.NewRenderCache(co)
}
