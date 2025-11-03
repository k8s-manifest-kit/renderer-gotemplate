# Design Document: GoTemplate Renderer

## 1. Introduction

This document outlines the design of the **GoTemplate Renderer** component of the k8s-manifest-kit ecosystem. The GoTemplate renderer enables rendering Kubernetes manifests from Go template files with support for dynamic values, caching, and source annotations.

**Part of the [k8s-manifest-kit](https://github.com/k8s-manifest-kit) organization.**

## 2. Overview

The GoTemplate renderer provides a way to generate Kubernetes manifests from Go template files (`.tpl`, `.gotmpl`, etc.) using Go's `text/template` package. It supports:

- **Multiple template sources** with glob pattern matching
- **Dynamic values** via functions or static maps
- **Render-time value merging** for parameterized rendering
- **TTL-based caching** with automatic deep cloning
- **Renderer-level filters and transformers**
- **Source annotations** for tracking template origin
- **Thread-safe concurrent rendering**

## 3. Core Concepts

### 3.1. Package Structure

```
renderer-gotemplate/
├── pkg/
│   ├── gotemplate.go          # Main renderer implementation
│   ├── gotemplate_option.go   # Functional options
│   ├── gotemplate_support.go  # Helper types and validation
│   ├── gotemplate_test.go     # Comprehensive test suite
│   ├── engine.go              # NewEngine convenience function
│   └── engine_test.go         # NewEngine tests
```

### 3.2. Core Types

#### Source

Represents a set of template files to render:

```go
type Source struct {
    // FS is the filesystem containing template files
    FS fs.FS
    
    // Path specifies the glob pattern to match template files
    // Examples: "templates/*.tpl", "**/*.yaml.gotmpl"
    Path string
    
    // Values provides data to be substituted into templates
    // Function is called during rendering to obtain dynamic values
    Values func(context.Context) (any, error)
}
```

#### Renderer

Implements the `types.Renderer` interface:

```go
type Renderer struct {
    inputs []*sourceHolder
    opts   RendererOptions
}

func New(inputs []Source, opts ...RendererOption) (*Renderer, error)
func (r *Renderer) Process(ctx context.Context, renderTimeValues map[string]any) ([]unstructured.Unstructured, error)
func (r *Renderer) Name() string
```

### 3.3. Rendering Flow

```
┌─────────────────────────────────────────────────────┐
│ 1. Load Templates (with lazy initialization)       │
│    - Parse template files matching glob pattern    │
│    - Cache parsed templates per source             │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 2. Merge Values                                     │
│    - Get source values (if function provided)      │
│    - Deep merge with render-time values            │
│    - Render-time values take precedence            │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 3. Check Cache (if enabled)                        │
│    - Compute cache key from path + values          │
│    - Return cached result if found                 │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 4. Execute Templates                                │
│    - For each template in source                   │
│    - Execute with merged values                    │
│    - Decode YAML output to unstructured objects    │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 5. Add Source Annotations (if enabled)             │
│    - manifest.k8s.io/source-type: "gotemplate"     │
│    - manifest.k8s.io/source-path: glob pattern     │
│    - manifest.k8s.io/source-file: template name    │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 6. Apply Renderer-Level Filters/Transformers       │
│    - Filter objects based on criteria              │
│    - Transform objects (e.g., add labels)          │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────────────────────┐
│ 7. Cache Result (if enabled)                       │
│    - Store result with TTL                         │
└────────────────┬────────────────────────────────────┘
                 │
                 ▼
           Return Objects
```

## 4. Key Features

### 4.1. Template Execution

Templates are executed using Go's `text/template` package. Values are accessible via dot notation:

```yaml
# pod.yaml.tpl
apiVersion: v1
kind: Pod
metadata:
  name: {{ .Name }}
  labels:
    app: {{ .App }}
spec:
  containers:
  - name: {{ .Container.Name }}
    image: {{ .Container.Image }}
```

### 4.2. Value Merging

Source values and render-time values are deep merged, with render-time values taking precedence:

```go
// Source values (static or from function)
source := gotemplate.Source{
    FS: templateFS,
    Path: "*.yaml",
    Values: gotemplate.Values(map[string]any{
        "name": "default-name",
        "labels": map[string]string{"env": "dev"},
    }),
}

// Render with overrides
objects, _ := renderer.Process(ctx, map[string]any{
    "labels": map[string]string{"env": "prod"}, // Overrides source value
})
```

### 4.3. Caching

TTL-based caching with automatic deep cloning to prevent cache pollution:

```go
renderer, _ := gotemplate.New(
    sources,
    gotemplate.WithCache(cache.WithTTL(5*time.Minute)),
)
```

Cache key is computed from:
- Template path (glob pattern)
- Merged values (source + render-time)

### 4.4. Thread Safety

The renderer is safe for concurrent use:
- Template parsing is protected by per-Source mutexes
- Lazy initialization ensures templates are parsed only once
- Multiple goroutines can call `Process()` simultaneously

### 4.5. Filters and Transformers

Renderer-level filters and transformers are applied after template execution:

```go
renderer, _ := gotemplate.New(
    sources,
    gotemplate.WithFilter(gvk.Filter(corev1.SchemeGroupVersion.WithKind("Pod"))),
    gotemplate.WithTransformer(labels.Set(map[string]string{"managed-by": "gotemplate"})),
)
```

## 5. Usage Patterns

### 5.1. Simple Rendering (Direct Renderer)

```go
import gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

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

### 5.2. Engine Integration (Convenience Function)

```go
import gotemplate "github.com/k8s-manifest-kit/renderer-gotemplate/pkg"

e, err := gotemplate.NewEngine(gotemplate.Source{
    FS:   os.DirFS("/path/to/templates"),
    Path: "*.yaml.tpl",
})

objects, err := e.Render(ctx, engine.WithValues(map[string]any{
    "replicas": 3,
}))
```

### 5.3. Production Configuration

```go
renderer, err := gotemplate.New(
    sources,
    // Enable caching with 10-minute TTL
    gotemplate.WithCache(cache.WithTTL(10*time.Minute)),
    
    // Add source annotations
    gotemplate.WithSourceAnnotations(true),
    
    // Filter to only Pods and Deployments
    gotemplate.WithFilter(gvk.Or(
        gvk.Filter(appsv1.SchemeGroupVersion.WithKind("Deployment")),
        gvk.Filter(corev1.SchemeGroupVersion.WithKind("Pod")),
    )),
    
    // Add common labels
    gotemplate.WithTransformer(labels.Set(map[string]string{
        "managed-by": "gotemplate",
        "version": "v1.0",
    })),
)
```

## 6. Design Decisions

### 6.1. Why text/template?

- **Standard library**: No external dependencies for template execution
- **Familiar syntax**: Go developers already know the template syntax
- **Type safety**: Can work with strongly typed values
- **Performance**: Fast template parsing and execution

### 6.2. Lazy Template Parsing

Templates are parsed on first use rather than at renderer creation:
- **Faster initialization**: No upfront parsing cost
- **Thread-safe**: Per-source mutex protects concurrent parsing
- **Memory efficient**: Only parse templates that are actually used

### 6.3. Deep Value Merging

Source and render-time values are deep merged to support:
- **Base configuration** in source values
- **Runtime overrides** via render-time values
- **Partial updates** without replacing entire structures

### 6.4. Cache Key Design

Cache keys include both path and values to ensure:
- **Correctness**: Different values produce different cache entries
- **Efficiency**: Same path+values reuse cached results
- **Safety**: Deep cloning prevents cache pollution

## 7. Error Handling

The renderer provides detailed error context:

```go
// Template parsing error
error rendering gotemplate pattern templates/*.tpl: 
    failed to parse templates: template: templates/pod.yaml.tpl:5: 
    unexpected "}" in operand

// Template execution error
error rendering gotemplate pattern templates/*.tpl:
    failed to execute template pod.yaml.tpl:
    template: pod.yaml.tpl:3:14: executing "pod.yaml.tpl" at <.Missing>:
    map has no entry for key "Missing"

// YAML decoding error
error rendering gotemplate pattern templates/*.tpl:
    failed to decode YAML from template pod.yaml.tpl:
    yaml: line 5: mapping values are not allowed in this context
```

## 8. Testing Strategy

The renderer includes comprehensive tests:

- **Unit tests**: Template parsing, execution, caching
- **Integration tests**: End-to-end rendering with filters/transformers
- **Benchmark tests**: Performance with and without caching
- **Thread-safety tests**: Concurrent rendering validation
- **Error tests**: Invalid templates, missing values, malformed YAML

## 9. Future Enhancements

Potential improvements for future versions:

1. **Sprig functions**: Include Sprig template functions
2. **Custom delimiters**: Support alternative delimiters (e.g., `[[ ]]`)
3. **Template includes**: Better support for template composition
4. **Validation**: Pre-render template validation
5. **Metrics**: Template execution timing and cache hit rates

## 10. Related Components

- **[engine](https://github.com/k8s-manifest-kit/engine)**: Core rendering pipeline
- **[pkg](https://github.com/k8s-manifest-kit/pkg)**: Shared utilities (caching, deep merge, JQ)
- **[renderer-helm](https://github.com/k8s-manifest-kit/renderer-helm)**: Helm chart renderer
- **[renderer-kustomize](https://github.com/k8s-manifest-kit/renderer-kustomize)**: Kustomize renderer
- **[renderer-yaml](https://github.com/k8s-manifest-kit/renderer-yaml)**: Static YAML renderer

