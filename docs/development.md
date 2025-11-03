# Development Guide: GoTemplate Renderer

This document provides coding conventions, testing guidelines, and contribution practices for developing the GoTemplate renderer component of k8s-manifest-kit.

For architectural information and design decisions, see [design.md](design.md).

**Part of the [k8s-manifest-kit](https://github.com/k8s-manifest-kit) organization.**

## Table of Contents

1. [Setup and Build](#setup-and-build)
2. [Coding Conventions](#coding-conventions)
3. [Testing Guidelines](#testing-guidelines)
4. [Adding Features](#adding-features)
5. [Code Review Guidelines](#code-review-guidelines)

## Setup and Build

### Prerequisites

- Go 1.24.8 or later
- Access to k8s-manifest-kit/engine and k8s-manifest-kit/pkg modules (local or via replace directives)

### Build Commands

```bash
# Run all tests
make test

# Format code
make fmt

# Run linter
make lint

# Fix linting issues automatically
make lint/fix

# Clean build artifacts and test cache
make clean

# Update dependencies
make deps
```

### Test Commands

```bash
# Run all tests with verbose output
go test -v ./pkg

# Run a specific test
go test -v ./pkg -run TestRenderer

# Run benchmarks
go test -v ./pkg -run=^$ -bench=.

# Run tests with race detector
go test -v -race ./pkg
```

## Coding Conventions

### Go Function Signatures

Following the project-wide conventions:

**Each parameter must have its own type declaration:**
```go
// Good
func renderSingle(
    ctx context.Context,
    holder *sourceHolder,
    renderTimeValues map[string]any,
) ([]unstructured.Unstructured, error)

// Bad - grouped parameters
func renderSingle(
    ctx context.Context,
    holder *sourceHolder, renderTimeValues map[string]any,
) ([]unstructured.Unstructured, error)
```

### Functional Options Pattern

All renderer configuration uses the functional options pattern:

```go
// Define renderer options
type RendererOption = util.Option[RendererOptions]

// Function-based option
func WithCache(opts ...cache.Option) RendererOption {
    return util.FunctionalOption[RendererOptions](func(ro *RendererOptions) {
        ro.Cache = cache.New(opts...)
    })
}

// Struct-based option for bulk configuration
type RendererOptions struct {
    Filters           []types.Filter
    Transformers      []types.Transformer
    Cache             *cache.Cache[[]unstructured.Unstructured]
    SourceAnnotations bool
}

func (opts RendererOptions) ApplyTo(ro *RendererOptions) {
    // Apply each field
}
```

### Error Handling

- Wrap errors using `fmt.Errorf` with `%w` verb for proper error chains
- Provide context in error messages (template name, path, etc.)
- Return detailed errors that help users debug template issues

```go
// Good - provides context
if err := t.Execute(&buf, values); err != nil {
    return nil, fmt.Errorf("failed to execute template %s: %w", t.Name(), err)
}

// Bad - generic error
if err != nil {
    return nil, err
}
```

### Documentation

**Comments should explain WHY, not WHAT:**

```go
// Good - explains non-obvious behavior
// Skip the root template as it's used only for includes/defines
if t.Name() == "" {
    continue
}

// Bad - restates the code
// Check if template name is empty
if t.Name() == "" {
    continue
}
```

**Focus on:**
- Non-obvious behavior (caching, merging logic)
- Edge cases (empty templates, nil values)
- Thread-safety guarantees
- Performance characteristics

## Testing Guidelines

### Test Structure

Use **Gomega** for assertions with `NewWithT(t)`:

```go
func TestRenderer(t *testing.T) {
    t.Run("should render single template", func(t *testing.T) {
        g := NewWithT(t)
        
        renderer, err := gotemplate.New(sources)
        g.Expect(err).ToNot(HaveOccurred())
        
        objects, err := renderer.Process(ctx, nil)
        g.Expect(err).ToNot(HaveOccurred())
        g.Expect(objects).To(HaveLen(1))
    })
}
```

### Test Coverage

The renderer should have tests for:

1. **Basic Rendering**
   - Single template rendering
   - Multiple template rendering
   - Empty templates
   - Non-existent templates

2. **Value Handling**
   - Static values via `Values()`
   - Dynamic values via function
   - Render-time value merging
   - Non-map values (strings, numbers)

3. **Caching**
   - Cache hits and misses
   - Cache key computation
   - Deep cloning from cache
   - TTL expiration

4. **Filters and Transformers**
   - Renderer-level filters
   - Renderer-level transformers
   - Combined filters and transformers

5. **Error Handling**
   - Invalid template syntax
   - Missing values in templates
   - Malformed YAML output
   - Invalid source configuration

6. **Thread Safety**
   - Concurrent rendering
   - Template parsing under contention

7. **Source Annotations**
   - Annotations added when enabled
   - No annotations when disabled
   - Correct annotation values

### Test Data

Use `testing/fstest.MapFS` for test templates:

```go
fs := fstest.MapFS{
    "templates/pod.yaml.tpl": &fstest.MapFile{
        Data: []byte(`
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Name }}
`),
    },
}

renderer, _ := gotemplate.New([]gotemplate.Source{
    {
        FS:   fs,
        Path: "templates/*.tpl",
        Values: gotemplate.Values(map[string]any{"Name": "test-pod"}),
    },
})
```

### Benchmarks

Include benchmarks for performance-critical paths:

```go
func BenchmarkGoTemplateRenderWithCache(b *testing.B) {
    renderer, _ := gotemplate.New(
        sources,
        gotemplate.WithCache(),
    )
    
    // Warm up cache
    _, _ = renderer.Process(b.Context(), nil)
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for range b.N {
        _, err := renderer.Process(b.Context(), nil)
        if err != nil {
            b.Fatalf("failed to render: %v", err)
        }
    }
}
```

## Adding Features

### Adding Template Functions

To add custom template functions:

1. Create a function map in `gotemplate_support.go`
2. Apply during template parsing
3. Document in `design.md`
4. Add tests in `gotemplate_test.go`

Example:

```go
func customFuncs() template.FuncMap {
    return template.FuncMap{
        "upper": strings.ToUpper,
        "lower": strings.ToLower,
    }
}

// In template parsing
tmpl := template.New("").Funcs(customFuncs())
```

### Adding New Options

1. Add field to `RendererOptions` in `gotemplate_option.go`
2. Create `WithXXX()` functional option
3. Update struct-based option `ApplyTo()` method
4. Document in godoc comments
5. Add tests

Example:

```go
// In RendererOptions
type RendererOptions struct {
    // ... existing fields
    CustomDelimiters [2]string
}

// Functional option
func WithDelimiters(left string, right string) RendererOption {
    return util.FunctionalOption[RendererOptions](func(ro *RendererOptions) {
        ro.CustomDelimiters = [2]string{left, right}
    })
}
```

### Adding Validation

Validation belongs in `gotemplate_support.go`:

```go
func (s *sourceHolder) Validate() error {
    if s.FS == nil {
        return utilerrors.NewValidationError("FS", "filesystem is required")
    }
    if s.Path == "" {
        return utilerrors.NewValidationError("Path", "path pattern is required")
    }
    return nil
}
```

## Code Review Guidelines

### Before Submitting

- [ ] All tests pass (`make test`)
- [ ] Code is formatted (`make fmt`)
- [ ] No linter errors (`make lint`)
- [ ] New functionality has tests
- [ ] Public APIs have godoc comments
- [ ] Error messages provide context
- [ ] Benchmarks included for performance-critical changes

### What Reviewers Look For

1. **Correctness**
   - Templates render correctly
   - Values merge properly
   - Caching behaves as expected
   - Thread-safe operations

2. **Error Handling**
   - Errors provide debugging context
   - Edge cases handled gracefully
   - Validation catches configuration issues

3. **Performance**
   - Lazy template parsing
   - Efficient caching
   - Minimal allocations in hot paths
   - Deep cloning only when necessary

4. **Maintainability**
   - Clear, focused functions
   - Minimal complexity
   - Consistent with codebase patterns
   - Well-documented non-obvious behavior

5. **Testing**
   - Comprehensive test coverage
   - Tests are clear and focused
   - Edge cases tested
   - Benchmarks for performance changes

### Common Patterns

**Thread-Safe Lazy Initialization:**
```go
type sourceHolder struct {
    Source
    mu        *sync.RWMutex
    templates *template.Template
}

func (s *sourceHolder) LoadTemplates() (*template.Template, error) {
    // Try read lock first (fast path)
    s.mu.RLock()
    if s.templates != nil {
        defer s.mu.RUnlock()
        return s.templates, nil
    }
    s.mu.RUnlock()
    
    // Need to parse, acquire write lock
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Double-check after acquiring write lock
    if s.templates != nil {
        return s.templates, nil
    }
    
    // Parse templates...
    s.templates = parsed
    return s.templates, nil
}
```

**Deep Value Merging:**
```go
// Use util.DeepMerge for combining source and render-time values
mergedValues := util.DeepMerge(sourceValues, renderTimeValues)
```

**Cache Key Computation:**
```go
// Use dump.ForHash for consistent cache key generation
cacheKey := dump.ForHash(cacheKeyData{
    Path:   holder.Path,
    Values: values,
})
```

## Dependencies

### Core Dependencies

- `github.com/k8s-manifest-kit/engine/pkg/types` - Core type definitions
- `github.com/k8s-manifest-kit/engine/pkg/pipeline` - Filter/transformer pipeline
- `github.com/k8s-manifest-kit/pkg/util` - Shared utilities (DeepMerge, Options)
- `github.com/k8s-manifest-kit/pkg/util/cache` - TTL-based caching
- `github.com/k8s-manifest-kit/pkg/util/k8s` - Kubernetes helpers (DecodeYAML)
- `k8s.io/apimachinery` - Kubernetes types

### Test Dependencies

- `github.com/onsi/gomega` - BDD-style assertions
- `testing/fstest` - In-memory filesystem for tests
- `github.com/rs/xid` - Unique ID generation for cache testing

## Questions or Issues?

- Open an issue in the [renderer-gotemplate repository](https://github.com/k8s-manifest-kit/renderer-gotemplate)
- Refer to [design.md](design.md) for architecture details
- Check the [k8s-manifest-kit organization](https://github.com/k8s-manifest-kit) for related components

