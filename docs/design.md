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

Source annotations are opt-in. Content hashes use
`manifests.k8s-manifests-kit/content.hash` and are enabled by default.

## Output caching

When caching is enabled, every registered source renders through its own
isolated output cache built from the renderer's cache options. Sources never
reuse each other's rendered output, even when their cache keys collide: two
sources with the same path and values, or a custom key function returning a
constant, still get separate entries. Cache keys are `TemplateSpec` values
(path plus merged values), so custom key functions receive the same input as
before and remain meaningful within a single source. Entries are deep-cloned
on get and set, and source post-renderers run after cache retrieval, so they
are reapplied on cache hits.

Templates are parsed once per source after the first successful load. Output
cache expiration or a cache miss re-executes the memoized templates but never
reparses them, so filesystem changes after the first load are not picked up;
construct a new renderer to refresh memoized sources.

See [`../AGENTS.md`](../AGENTS.md) and [`development.md`](development.md).
