# Agent Guide: renderer-gotemplate

`renderer-gotemplate` renders Kubernetes manifests from Go `text/template` files with dynamic values, caching, filters, transformers, and source tracking.

## Documentation

- [README](README.md) — overview and template-function usage.
- [Design](docs/design.md) — rendering architecture.
- [Development](docs/development.md) — workflow and tests.

## Public API

The package is imported from `github.com/k8s-manifest-kit/renderer-gotemplate/pkg`.

- `gotemplate.New([]gotemplate.Source{...}, opts...)` creates a renderer.
- `gotemplate.NewEngine(source, opts...)` creates an `engine.Engine` for one source.
- `Source` contains `FS`, `Path`, `Values`, and source-specific `PostRenderers`.
- Options include filters, transformers, post-renderers, source selectors, caching, source annotations, content hashes, and renderer-level template functions.
- `Values(map[string]any)` supplies static values.
- `ToYAML`, `Indent`, and `Nindent` are opt-in helpers. They are not registered automatically; use `WithFunc` or `WithFuncs`.

Templates are parsed lazily per source with double-checked locking and use `missingkey=error`. Source values and render-time values are deep-merged, with render-time values taking precedence. Content hashes are enabled by default; source annotations are disabled by default.

## Development

Run commands from this directory:

```bash
make test
make fmt
make lint
make lint/fix
make check
```

Use `testing/fstest.MapFS`, test invalid templates and missing values, preserve error context, and verify concurrent lazy loading when changing template initialization.

