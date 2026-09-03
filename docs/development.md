# Go Template Renderer Development

## Prerequisites and commands

Use Go 1.26.8 and the Makefile targets:

```bash
make test
make fmt
make lint
make lint/fix
make check
```

## Testing

Test static and dynamic values, render-time value precedence, missing keys,
opt-in functions, YAML decode errors, lazy parsing under concurrent renders,
source selection, cache behavior, and source/content annotations. Keep the
template function surface explicit and avoid adding helpers implicitly.

See [`design.md`](design.md) and [`../AGENTS.md`](../AGENTS.md).
