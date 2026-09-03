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

## Values and pipeline

Source values are resolved for each render and merged with render-time values;
render-time values overlay source values. The renderer parses and executes the
selected templates, decodes YAML documents, applies source post-renderers,
then combines results and applies renderer-level filters, transformers, and
post-renderers.

Source annotations are opt-in. Content hashes use
`manifests.k8s-manifests-kit/content.hash` and are enabled by default. The
shared cache interface can be configured with cache options.

See [`../AGENTS.md`](../AGENTS.md) and [`development.md`](development.md).
