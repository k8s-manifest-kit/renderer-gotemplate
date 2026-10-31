package cache_test

import (
	"testing"
	"time"

	utilcache "github.com/k8s-manifest-kit/pkg/util/cache"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/k8s-manifest-kit/renderer-gotemplate/pkg/cache"

	. "github.com/onsi/gomega"
)

func testObjects(name string) []unstructured.Unstructured {
	return []unstructured.Unstructured{
		{Object: map[string]any{
			"kind": "ConfigMap",
			"metadata": map[string]any{
				"name": name,
			},
		}},
	}
}

func TestNew(t *testing.T) {
	t.Run("should disable caching for nil options", func(t *testing.T) {
		g := NewWithT(t)

		g.Expect(cache.New(nil)).To(BeNil())
	})

	t.Run("should key default entries by the exported spec", func(t *testing.T) {
		g := NewWithT(t)

		c := cache.New(&utilcache.Options{})

		spec := cache.TemplateSpec{Path: "templates/*.tpl", Values: map[string]any{"Env": "prod"}}

		c.Set(spec, testObjects("default-key"))

		cached, found := c.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(cached).To(HaveLen(1))
		g.Expect(cached[0].GetName()).To(Equal("default-key"))

		// Different values produce a different default key.
		other := cache.TemplateSpec{Path: "templates/*.tpl", Values: map[string]any{"Env": "dev"}}
		_, found = c.Get(other)
		g.Expect(found).To(BeFalse())
	})

	t.Run("should preserve custom key functions", func(t *testing.T) {
		g := NewWithT(t)

		var received []any

		c := cache.New(&utilcache.Options{
			KeyFunc: func(key any) string {
				received = append(received, key)

				if spec, ok := key.(cache.TemplateSpec); ok {
					return spec.Path
				}

				return "unknown"
			},
		})

		spec := cache.TemplateSpec{Path: "templates/*.tpl", Values: map[string]any{"Env": "prod"}}

		c.Set(spec, testObjects("custom-key"))

		cached, found := c.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(cached[0].GetName()).To(Equal("custom-key"))

		// The custom key function saw the spec and keyed on the path only, so
		// different values still hit.
		other := cache.TemplateSpec{Path: "templates/*.tpl", Values: map[string]any{"Env": "dev"}}
		cached, found = c.Get(other)
		g.Expect(found).To(BeTrue())
		g.Expect(cached[0].GetName()).To(Equal("custom-key"))

		// Set plus two gets: every invocation received the exported spec.
		g.Expect(received).To(HaveLen(3))
		for _, key := range received {
			spec, ok := key.(cache.TemplateSpec)
			g.Expect(ok).To(BeTrue(), "key function should receive a TemplateSpec, got %T", key)
			g.Expect(spec.Path).To(Equal("templates/*.tpl"))
		}
	})

	t.Run("should honor the configured TTL", func(t *testing.T) {
		g := NewWithT(t)

		c := cache.New(&utilcache.Options{TTL: 100 * time.Millisecond})

		spec := cache.TemplateSpec{Path: "templates/*.tpl"}

		c.Set(spec, testObjects("expiring"))

		cached, found := c.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(cached[0].GetName()).To(Equal("expiring"))

		time.Sleep(150 * time.Millisecond)

		_, found = c.Get(spec)
		g.Expect(found).To(BeFalse())
	})

	t.Run("should not mutate the caller options", func(t *testing.T) {
		g := NewWithT(t)

		opts := &utilcache.Options{TTL: time.Minute}

		cache.New(opts)

		// The default key function injection must not leak into the caller's
		// options, and the TTL must be preserved as configured.
		g.Expect(opts.KeyFunc).To(BeNil())
		g.Expect(opts.TTL).To(Equal(time.Minute))
	})

	t.Run("should not share entries between instances", func(t *testing.T) {
		g := NewWithT(t)

		first := cache.New(&utilcache.Options{})
		second := cache.New(&utilcache.Options{})

		spec := cache.TemplateSpec{Path: "templates/*.tpl"}

		first.Set(spec, testObjects("first"))

		_, found := second.Get(spec)
		g.Expect(found).To(BeFalse())

		second.Set(spec, testObjects("second"))

		cached, found := first.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(cached[0].GetName()).To(Equal("first"))

		cached, found = second.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(cached[0].GetName()).To(Equal("second"))
	})

	t.Run("should deep clone stored and returned objects", func(t *testing.T) {
		g := NewWithT(t)

		c := cache.New(&utilcache.Options{})

		spec := cache.TemplateSpec{Path: "templates/*.tpl"}

		stored := testObjects("cloned")
		c.Set(spec, stored)

		// Mutating the caller's slice after Set must not affect the cache.
		stored[0].SetName("mutated-stored")

		first, found := c.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(first[0].GetName()).To(Equal("cloned"))

		// Mutating a returned slice must not affect later retrievals.
		first[0].SetName("mutated-returned")

		second, found := c.Get(spec)
		g.Expect(found).To(BeTrue())
		g.Expect(second[0].GetName()).To(Equal("cloned"))
	})
}
