# Go Template Renderer Development

## Prerequisites and commands

Build and test with the Go version required by [`go.mod`](../go.mod) and the
Makefile targets:

```bash
make test        # unit tests
make test/race   # unit tests with the race detector
make fmt         # format code (golangci-lint fmt and go fmt)
make lint        # golangci-lint
make lint/fix    # golangci-lint with --fix
make check       # lint and vulncheck
```

`make check` runs lint and vulnerability checks only; it is not a substitute
for `make test` or `make test/race`, and formatting is a separate target.
There is no live integration target: tests run offline against
`testing/fstest.MapFS` filesystems.

## Code conventions

- Give each parameter its own type: `name string, namespace string`, not
  `name, namespace string`.
- Wrap errors with operation and source context using `%w`, and use
  `errors.Is` for sentinel checks. Rendering and validation errors should
  name the source path, template, or key needed for diagnosis without
  echoing input contents.
- Keep shared implementation helpers in `_support.go` files and shared
  test-only helpers in `_support_test.go` files, following the test layout
  rules below; leave cohesive feature files intact.
- Format with `make fmt`. The formatter set (gofmt, goimports, gci) and the
  linter configuration in [`.golangci.yml`](../.golangci.yml) define the
  style; do not weaken the configuration to accommodate an implementation
  choice.

### Multiline strings

- Declare every multiline string — fixtures, manifests, templates, and
  expected output — as one descriptive, standalone, package-level
  `const name =` declaration with a raw string literal.
- Never embed multiline string literals in function or method bodies or
  declare them as function-local constants.
- Never group multiline constants in `const (...)`. Related single-line
  scalar constants can still be grouped, and single-line inline strings
  are unaffected.
- Keep large literal content separate from execution logic for
  readability. Preserve meaningful content indentation; the declaration
  layout must not introduce unintended bytes.
- A function may format the named template into a local variable: the
  rule concerns where the literal is declared, not where its string
  value is used.

```go
const fixtureYAML = `apiVersion: v1
kind: ConfigMap
`
```

## Testing

Tests use Gomega with `t.Context()` and live in the external
`gotemplate_test` package, exercising the public API. Cover static and
dynamic values, render-time value precedence, missing keys, opt-in
functions, YAML decode errors, source selection, cache behavior, and
source/content annotations. Keep the template function surface explicit:
helpers such as `ToYAML`, `Indent`, and `Nindent` are registered through
`WithFunc` or `WithFuncs` only, never added implicitly. Templates execute
with `missingkey=error`; preserve that behavior in tests.

Concurrent rendering is covered by `TestRendererConcurrency`, which shares
one renderer across goroutines released through a start barrier, joins all
workers before any assertion, gives each worker independently owned values
and results, and asserts on the test goroutine. Cover both cold renders
(first successful parse, live output) and warm renders (already-parsed
templates and cache hits). Use the render-origin annotations
(`types.RenderOriginLive` and `types.RenderOriginCache`) as deterministic
cache evidence; callback counts are not cache-hit or cache-miss evidence.

Functions registered with `WithFunc` or `WithFuncs`, source `Values`
functions, and post-renderers run inside `Process` and are not synchronized
by the renderer: when a renderer instance is shared by overlapping calls,
such caller-supplied code must be safe for concurrent use. Register only
pure functions in concurrency tests.

### Test layout

- Colocate tests with implementation. A `test/` subdirectory is a separate
  Go package, not a cosmetic folder; use the existing package boundaries.
- Organize larger suites by concern as `<component>_<concern>_test.go` and
  retain a cohesive `<component>_test.go` for core API behavior. Files such
  as `_values_test.go`, `_cache_test.go`, `_metadata_test.go`, and
  `_concurrency_test.go` are used where those concerns exist; they are not
  mandatory for every package.
- Place benchmarks in `<component>_benchmark_test.go` in the same package
  directory, retaining benchmark names and setup/timing behavior.
- Put shared test helpers in `<component>_support_test.go` and substantial
  feature-specific fixture machinery in
  `<component>_<concern>_support_test.go`. Small test-local helpers and
  scenario assertions can remain beside their tests.
- Support files contain helper, fixture, and type declarations — not test,
  benchmark, fuzz, or example entry points. `_support_test.go` is a naming
  convention; only `_test.go` has Go's test-file significance.
- Keep tests discoverable without excessive fragmentation: there is no hard
  line limit, no one-file-per-test rule, no shared cross-repository harness,
  no fixture-storage migration, and no automatic test-speed claim.
- Filenames alone do not change which suite a test runs in: renaming a file
  does not turn an offline test into a live integration test or exclude it
  from the offline suite.

### Regression anchors

- `TestRendererConcurrency` (`pkg/gotemplate_concurrency_test.go`): shared
  renderer, lazy parsing, request-specific values, per-source cache
  isolation under colliding keys, and independence of returned nested
  objects from other results, later hits, and the source inputs.
- `TestSourceCacheIsolation` (`pkg/gotemplate_cache_test.go`): per-source
  cache partitioning under equal keys and constant custom key functions,
  reapplication of source post-renderers on cache hits, and the parse-once
  template snapshot.
- `TestRenderTimeValues` (`pkg/gotemplate_values_test.go`): render-time values
  deep-merge over source values and take precedence.

Template lifetime is distinct from output-cache expiration: cache misses and
expiry re-execute the memoized templates but never reparse them, and
refreshing memoized sources requires constructing a new renderer. A custom
key function that ignores values only changes entry selection within one
source; it never merges rendered output across sources. Details in
[`design.md`](design.md).

See [`design.md`](design.md) and [`../AGENTS.md`](../AGENTS.md).
