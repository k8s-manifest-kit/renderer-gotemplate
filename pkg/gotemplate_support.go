package gotemplate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/k8s-manifest-kit/engine/pkg/types"
	utilerrors "github.com/k8s-manifest-kit/pkg/util/errors"
)

var errInvalidTemplateFuncMap = errors.New("invalid template function map")

// Values returns a Values function that always returns the provided static values.
// This is a convenience helper for the common case of non-dynamic values.
func Values(values map[string]any) func(context.Context) (types.Values, error) {
	return func(_ context.Context) (types.Values, error) {
		return types.Values(values), nil
	}
}

// sourceHolder wraps a Source with internal state for lazy loading and thread-safety.
type sourceHolder struct {
	Source

	// Mutex protects concurrent access to templates field
	mu *sync.RWMutex

	// Template functions registered at the renderer level.
	funcs template.FuncMap

	// Parsed templates (lazy-loaded on first Process call, protected by mu)
	templates *template.Template
}

// Validate checks if the Source configuration is valid.
func (h *sourceHolder) Validate() error {
	if h.FS == nil {
		return utilerrors.ErrFsRequired
	}
	if strings.TrimSpace(h.Path) == "" {
		return utilerrors.ErrPathEmpty
	}

	return nil
}

// LoadTemplates returns parsed templates, loading them lazily if needed.
// Thread-safe for concurrent use. Uses a double-checked read lock pattern
// to avoid write-lock contention on the hot path (templates already parsed).
func (h *sourceHolder) LoadTemplates() (*template.Template, error) {
	h.mu.RLock()
	if h.templates != nil {
		t := h.templates
		h.mu.RUnlock()

		return t, nil
	}
	h.mu.RUnlock()

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.templates != nil {
		return h.templates, nil
	}

	tmpl := template.New("")

	if err := addTemplateFuncs(tmpl, h.funcs); err != nil {
		return nil, fmt.Errorf("failed to register template functions (path: %s): %w", h.Path, err)
	}

	tmpl, err := tmpl.ParseFS(h.FS, h.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates (path: %s): %w", h.Path, err)
	}

	h.templates = tmpl.Option("missingkey=error")

	return h.templates, nil
}

func addTemplateFuncs(tmpl *template.Template, funcs template.FuncMap) (err error) {
	if len(funcs) == 0 {
		return nil
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", errInvalidTemplateFuncMap, recovered)
		}
	}()

	tmpl.Funcs(funcs)

	return nil
}
