# Go Template Renderer

`renderer-gotemplate` renders Kubernetes manifests from Go `text/template`
files stored in an `io/fs` filesystem. It supports static or dynamic values,
optional template functions, source selection, post-renderers, content hashes,
and caching.

## Installation

```bash
go get github.com/k8s-manifest-kit/renderer-gotemplate
```

## Quick start

```go
e, err := gotemplate.NewEngine(gotemplate.Source{
    FS:   os.DirFS("."),
    Path: "manifests/*.yaml.tmpl",
    Values: func(context.Context) (types.Values, error) {
        return types.Values{"name": "demo"}, nil
    },
})
if err != nil {
    return err
}

objects, err := e.Render(ctx)
```

Template helpers such as `ToYAML`, `Indent`, and `Nindent` are opt-in through
`WithFunc` or `WithFuncs`. Templates use `missingkey=error`; parsing is lazy
and synchronized. Render-time values are merged with source values according
to the renderer's documented precedence.

See [`docs/design.md`](docs/design.md), [`docs/development.md`](docs/development.md),
and [`AGENTS.md`](AGENTS.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
