# renderer-gotemplate

Go template renderer for Kubernetes manifests

Part of the [k8s-manifest-kit](https://github.com/k8s-manifest-kit) organization.

## Status

🚧 **Under Development** - This repository is being set up.

## Installation

```bash
go get github.com/k8s-manifest-kit/renderer-gotemplate
```

## Documentation

See the main [docs repository](https://github.com/k8s-manifest-kit/docs) for comprehensive documentation.

## Template Functions

Custom template functions can be registered per renderer using functional options.
The package also exposes a small set of convenience helpers that callers can opt
into explicitly:

- `ToYAML`
- `Indent`
- `Nindent`

These helpers are **not** registered by default. If you want the broader
Sprig helper set, register Sprig's function map explicitly through
`WithFuncs(...)`.

```go
renderer, err := gotemplate.New(
    []gotemplate.Source{
        {
            FS:   os.DirFS("./templates"),
            Path: "*.yaml.tpl",
        },
    },
    gotemplate.WithFunc("upper", strings.ToUpper),
    gotemplate.WithFuncs(template.FuncMap{
        "toYAML":  gotemplate.ToYAML,
        "indent":  gotemplate.Indent,
        "nindent": gotemplate.Nindent,
    }),
)
```

## Contributing

Contributions are welcome! Please see our [contributing guidelines](https://github.com/k8s-manifest-kit/docs/blob/main/CONTRIBUTING.md).

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
