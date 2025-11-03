# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The **GoTemplate Renderer** component of k8s-manifest-kit renders Kubernetes manifests from Go template files. It supports dynamic values, caching, filters/transformers, and source annotations.

**Part of the [k8s-manifest-kit](https://github.com/k8s-manifest-kit) organization.**

## Documentation

- **[README.md](README.md)** - Module overview and quick start
- **[docs/design.md](docs/design.md)** - Template rendering architecture and design decisions
- **[docs/development.md](docs/development.md)** - Coding conventions, testing guidelines, and contribution guide

## Quick Reference

### Core Types

```go
// Source represents a set of template files to render
type Source struct {
    FS     fs.FS                                           // Filesystem containing templates
    Path   string                                          // Glob pattern (e.g., "*.yaml.tpl")
    Values func(context.Context) (any, error)             // Dynamic values function
}

// Renderer implements types.Renderer
type Renderer struct {
    inputs []*sourceHolder
    opts   RendererOptions
}

func New(inputs []Source, opts ...RendererOption) (*Renderer, error)
func (r *Renderer) Process(ctx context.Context, renderTimeValues map[string]any) ([]unstructured.Unstructured, error)
func (r *Renderer) Name() string

// NewEngine creates an Engine with a gotemplate renderer (convenience function)
func NewEngine(source Source, opts ...RendererOption) (*engine.Engine, error)
```

### Basic Usage

**Direct Renderer:**
```go
renderer, err := gotemplate.New([]gotemplate.Source{
    {
        FS:   os.DirFS("/path/to/templates"),
        Path: "*.yaml.tpl",
        Values: gotemplate.Values(map[string]any{
            "name": "my-app",
        }),
    },
})

objects, err := renderer.Process(ctx, nil)
```

**Engine Integration:**
```go
e, err := gotemplate.NewEngine(gotemplate.Source{
    FS:   os.DirFS("/path/to/templates"),
    Path: "*.yaml.tpl",
})

objects, err := e.Render(ctx, engine.WithValues(map[string]any{
    "replicas": 3,
}))
```

### Renderer Options

```go
// Caching with TTL
gotemplate.WithCache(cache.WithTTL(5*time.Minute))

// Source annotations
gotemplate.WithSourceAnnotations(true)

// Renderer-level filter
gotemplate.WithFilter(gvk.Filter(corev1.SchemeGroupVersion.WithKind("Pod")))

// Renderer-level transformer
gotemplate.WithTransformer(labels.Set(map[string]string{"env": "prod"}))
```

### Value Merging

Source values and render-time values are deep merged (render-time takes precedence):

```go
source := gotemplate.Source{
    FS: templateFS,
    Path: "*.yaml",
    Values: gotemplate.Values(map[string]any{
        "name": "default",
        "config": map[string]any{"replicas": 1},
    }),
}

// Render with overrides
objects, _ := renderer.Process(ctx, map[string]any{
    "config": map[string]any{"replicas": 3}, // Overrides source value
})
```

### Template Syntax

Templates use Go's `text/template` syntax:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Name }}
  labels:
    app: {{ .App }}
    {{- range $key, $value := .Labels }}
    {{ $key }}: {{ $value }}
    {{- end }}
spec:
  containers:
  - name: {{ .Container.Name }}
    image: {{ .Container.Image }}
    {{- if .Container.Ports }}
    ports:
    {{- range .Container.Ports }}
    - containerPort: {{ . }}
    {{- end }}
    {{- end }}
```

## Development

**Run tests:**
```bash
make test
```

**Format and lint:**
```bash
make fmt
make lint
```

**Run specific test:**
```bash
go test -v ./pkg -run TestRenderer
```

**Run benchmarks:**
```bash
go test -v ./pkg -run=^$ -bench=.
```

For detailed development information:
- **Build commands**: See [docs/development.md#setup-and-build](docs/development.md#setup-and-build)
- **Coding conventions**: See [docs/development.md#coding-conventions](docs/development.md#coding-conventions)
- **Testing guidelines**: See [docs/development.md#testing-guidelines](docs/development.md#testing-guidelines)
- **Adding features**: See [docs/development.md#adding-features](docs/development.md#adding-features)
- **Code review guidelines**: See [docs/development.md#code-review-guidelines](docs/development.md#code-review-guidelines)

## Testing Conventions

- Use vanilla Gomega (dot import): `import . "github.com/onsi/gomega"`
- Use `NewWithT(t)` for test assertions
- Use `testing/fstest.MapFS` for test templates
- Use `t.Context()` instead of `context.Background()`
- Benchmark naming: `Benchmark<Component><TestName>`

Example test:
```go
func TestRenderer(t *testing.T) {
    t.Run("should render single template", func(t *testing.T) {
        g := NewWithT(t)
        
        fs := fstest.MapFS{
            "pod.yaml.tpl": &fstest.MapFile{Data: []byte(podTemplate)},
        }
        
        renderer, err := gotemplate.New([]gotemplate.Source{
            {FS: fs, Path: "*.tpl", Values: gotemplate.Values(testValues)},
        })
        g.Expect(err).ToNot(HaveOccurred())
        
        objects, err := renderer.Process(t.Context(), nil)
        g.Expect(err).ToNot(HaveOccurred())
        g.Expect(objects).To(HaveLen(1))
    })
}
```

## Key Concepts

### Rendering Flow

```
1. Load Templates (lazy, thread-safe)
   └─► Parse templates matching glob pattern
   
2. Merge Values
   └─► Deep merge source values + render-time values
   
3. Check Cache (if enabled)
   └─► Return cached result if found (deep cloned)
   
4. Execute Templates
   └─► Execute each template with merged values
   └─► Decode YAML output to unstructured objects
   
5. Add Source Annotations (if enabled)
   └─► manifest.k8s.io/source-type: "gotemplate"
   └─► manifest.k8s.io/source-path: glob pattern
   └─► manifest.k8s.io/source-file: template name
   
6. Apply Renderer-Level Filters/Transformers
   └─► Filter objects, transform remaining objects
   
7. Cache Result (if enabled)
   └─► Store with TTL
```

### Thread Safety

The renderer is safe for concurrent use:
- Template parsing protected by per-Source `sync.RWMutex`
- Lazy initialization with double-checked locking
- Multiple goroutines can call `Process()` simultaneously
- Caching uses thread-safe `sync.Map` internally

### Caching Strategy

Cache key computed from:
- Template path (glob pattern)
- Merged values (source + render-time)

Cache entries:
- Stored with TTL
- Deep cloned on retrieval to prevent pollution
- Automatically evicted on expiration

### Value Merging

Uses `util.DeepMerge()` for combining maps:
- Recursively merges nested maps
- Render-time values override source values
- Non-map values replaced entirely

## Coding Conventions

### Go Function Signatures

Each parameter must have its own type declaration:
```go
// Good
func renderSingle(
    ctx context.Context,
    holder *sourceHolder,
    renderTimeValues map[string]any,
) ([]unstructured.Unstructured, error)

// Bad
func renderSingle(ctx context.Context, holder *sourceHolder, renderTimeValues map[string]any) ([]unstructured.Unstructured, error)
```

### Error Handling

Provide context in error messages:
```go
// Good
if err := t.Execute(&buf, values); err != nil {
    return nil, fmt.Errorf("failed to execute template %s: %w", t.Name(), err)
}

// Bad
if err != nil {
    return nil, err
}
```

### Documentation Comments

Explain WHY, not WHAT:
```go
// Good - explains non-obvious behavior
// Skip the root template as it's used only for includes/defines
if t.Name() == "" {
    continue
}

// Bad - restates the code
// Check if name is empty
if t.Name() == "" {
    continue
}
```

## Module Structure

```
renderer-gotemplate/
├── pkg/
│   ├── gotemplate.go          # Main renderer implementation
│   ├── gotemplate_option.go   # Functional options (WithCache, WithFilter, etc.)
│   ├── gotemplate_support.go  # Helper types (sourceHolder, Values helper)
│   ├── gotemplate_test.go     # Comprehensive test suite
│   ├── engine.go              # NewEngine convenience function
│   └── engine_test.go         # NewEngine tests
├── docs/
│   ├── design.md              # Architecture and design decisions
│   └── development.md         # Development guidelines
├── .golangci.yml              # Linter configuration
├── Makefile                   # Build and test targets
├── CLAUDE.md                  # This file
└── README.md                  # Quick start and overview
```

## Dependencies

### Core Dependencies

- `github.com/k8s-manifest-kit/engine/pkg/types` - Renderer interface
- `github.com/k8s-manifest-kit/engine/pkg/pipeline` - Filter/transformer execution
- `github.com/k8s-manifest-kit/pkg/util` - Deep merge, options pattern
- `github.com/k8s-manifest-kit/pkg/util/cache` - TTL-based caching
- `github.com/k8s-manifest-kit/pkg/util/k8s` - YAML decoding
- `k8s.io/apimachinery` - Kubernetes types

### Test Dependencies

- `github.com/onsi/gomega` - BDD-style assertions
- `testing/fstest` - In-memory filesystem for tests
- `github.com/rs/xid` - Unique ID generation

## Common Patterns

### Thread-Safe Lazy Template Loading

```go
func (s *sourceHolder) LoadTemplates() (*template.Template, error) {
    // Try read lock first (fast path)
    s.mu.RLock()
    if s.templates != nil {
        defer s.mu.RUnlock()
        return s.templates, nil
    }
    s.mu.RUnlock()
    
    // Acquire write lock for parsing
    s.mu.Lock()
    defer s.mu.Unlock()
    
    // Double-check after acquiring write lock
    if s.templates != nil {
        return s.templates, nil
    }
    
    // Parse templates...
}
```

### Value Merging

```go
// Deep merge source and render-time values
sourceValues := map[string]any{/* from source.Values() */}
mergedValues := util.DeepMerge(sourceValues, renderTimeValues)
```

### Cache Key Generation

```go
// Use dump.ForHash for consistent keys
cacheKey := dump.ForHash(cacheKeyData{
    Path:   holder.Path,
    Values: values,
})
```

## Related Components

- **[engine](https://github.com/k8s-manifest-kit/engine)** - Core rendering pipeline and engine
- **[pkg](https://github.com/k8s-manifest-kit/pkg)** - Shared utilities (caching, deep merge, JQ)
- **[renderer-helm](https://github.com/k8s-manifest-kit/renderer-helm)** - Helm chart renderer
- **[renderer-kustomize](https://github.com/k8s-manifest-kit/renderer-kustomize)** - Kustomize renderer
- **[renderer-yaml](https://github.com/k8s-manifest-kit/renderer-yaml)** - Static YAML renderer

## Questions or Issues?

- Open an issue in the [renderer-gotemplate repository](https://github.com/k8s-manifest-kit/renderer-gotemplate)
- Refer to [docs/design.md](docs/design.md) for architecture details
- Check the [k8s-manifest-kit organization](https://github.com/k8s-manifest-kit) for related components

