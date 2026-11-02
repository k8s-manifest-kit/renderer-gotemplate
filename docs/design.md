# Go Template Renderer Design

## Source model

`gotemplate.Source` contains an `io/fs.FS`, a template path or glob, an
optional `Values func(context.Context) (types.Values, error)`, and optional
source-specific post-renderers. Source selectors can exclude sources before
parsing.

Templates use Go `text/template` with `missingkey=error`. Parsing is lazy and
guarded by synchronization so the same renderer can be used concurrently.
Only explicitly configured functions are available: use `WithFunc` or
`WithFuncs` to register helpers such as `ToYAML`, `Indent`, and `Nindent`.

Functions registered with `WithFunc` or `WithFuncs`, source `Values`
functions, and post-renderers run inside `Process`. The renderer does not
synchronize this caller-supplied code: it must be safe for concurrent use
whenever a renderer instance is shared by overlapping calls.

## Values and pipeline

Source values are resolved for each render and merged with render-time values;
render-time values overlay source values. The renderer parses and executes the
selected templates, decodes YAML documents, applies source post-renderers,
then combines results and applies renderer-level filters, transformers, and
post-renderers.

Both value entry points are cycle-guarded before the shared clone and merge
helpers recurse into them. Resolved source values are validated on every
render — including output-cache hits, since values resolve before the cache
lookup — and render-time values are validated per selected source before the
per-source clone; each rejection wraps the shared `maps.ErrCycle` and names
the source's path pattern, and the whole supported tree is rejected even when
the overlay would replace the cyclic branch. The traversal matches the clone
and merge types exactly — nested `map[string]any` and `[]any` — so shared
references that do not form cycles stay valid, while retained values of other
types, template functions, and encoders are outside the guarantee. Deselected
sources neither validate render-time values nor invoke their callbacks; the
engine's global rejection is a separate, earlier boundary.

Source annotations are opt-in. Content hashes use
`manifests.k8s-manifests-kit/content.hash` and are enabled by default.

### Option composition

Renderer options apply left to right. Functional options touch a single
field — `WithFilter`, `WithTransformer`, `WithPostRenderer`, and
`WithSourceSelector` append, the boolean setters overwrite only their own
field, and `WithFunc`/`WithFuncs` merge into the registered template
functions — so they compose with anything around them. A
`RendererOptions` struct is stronger: it replaces the hook, selector, and
boolean fields outright (including with nil and false), while a nil
`CacheOptions` preserves what earlier options configured and its `Funcs`
map merges into the previously registered functions, with matching names
overwriting and an empty map leaving them untouched.

A hook before a struct survives only if the struct carries it; a hook after
a struct is kept:

```go
// The struct clears the filter added before it and disables hashing:
// its zero-valued Filters and ContentHash replace the previous state,
// while its Funcs still merge with functions registered earlier.
renderer, _ := gotemplate.New(sources,
    gotemplate.WithFilter(keepPods),
    gotemplate.RendererOptions{SourceAnnotations: true},
    gotemplate.WithContentHash(true), // re-enables what the struct turned off
)

// Hook after struct: the post-renderer is appended to the struct's state.
renderer, _ := gotemplate.New(sources,
    gotemplate.RendererOptions{SourceAnnotations: true, ContentHash: true},
    gotemplate.WithPostRenderer(label),
)
```

A struct that wants hashing must set `ContentHash: true` explicitly; a
`WithContentHash(true)` after the struct achieves the same. These rules
govern the renderer's own options only — the engine's additive options
(`engine.WithFilter` and friends) follow the engine's rules, not these.

### Hash stages

Objects decoded from a rendered template are annotated in one step after
decoding and before the result is stored in the source's output cache:
source annotations are applied first (when enabled), then the content hash.
The render-origin stamp — live or cache — follows cache storage and
retrieval, and the source, renderer, and engine hooks run last.

Consequences callers can rely on: the hash covers the complete object at
hashing time, including any metadata already present — previously applied
source annotations participate, as does any pre-existing annotation. It is
not a hash of the template's source bytes, and it is not idempotent. The
render-origin stamp and cache retrieval never rehash, and mutations made by
later hooks need not match the stored digest. The full annotation phase
contract, including phase ordering and option reuse, is the engine's
[annotation contract](../../engine/docs/design.md#annotations); this
renderer follows it at the points described above.

## Output caching

When caching is enabled, every registered source renders through its own
isolated output cache built from the renderer's cache options. Sources never
reuse each other's rendered output, even when their cache keys collide: two
sources with the same path and values, or a custom key function returning a
constant, still get separate entries. Cache keys are `cache.TemplateSpec`
values (path plus merged values) from
`github.com/k8s-manifest-kit/renderer-gotemplate/pkg/cache`, so custom key
functions receive the same input as before and remain meaningful within a
single source. Entries are deep-cloned on get and set, and source
post-renderers run after cache retrieval, so they are reapplied on cache
hits.

Templates are parsed once per source after the first successful load. Output
cache expiration or a cache miss re-executes the memoized templates but never
reparses them, so filesystem changes after the first load are not picked up;
construct a new renderer to refresh memoized sources.

Breaking change: `TemplateSpec` and the cache constructor moved from the root
`gotemplate` package to
`github.com/k8s-manifest-kit/renderer-gotemplate/pkg/cache`
(`cache.TemplateSpec`, `cache.New`). The old root symbols are gone; key
functions that type-asserted `gotemplate.TemplateSpec` must assert
`cache.TemplateSpec` instead.

See [`../AGENTS.md`](../AGENTS.md) and [`development.md`](development.md).
