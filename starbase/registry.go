package starbase

import (
	"fmt"
	"io"
	"sync"

	"go.starlark.net/starlark"
)

// Registry manages all available modules and provides loading functionality.
type Registry struct {
	modules    map[ModuleName]Module
	loaded     starlark.StringDict
	aliases    starlark.StringDict
	config     *ModuleConfig
	loadOnce   sync.Once
	mu         sync.RWMutex
	loadErrors []error
}

// NewRegistry creates a new module registry with the given configuration.
func NewRegistry(config *ModuleConfig) *Registry {
	if config == nil {
		config = &ModuleConfig{}
	}
	return &Registry{
		modules: make(map[ModuleName]Module),
		loaded:  make(starlark.StringDict),
		aliases: make(starlark.StringDict),
		config:  config,
	}
}

// Register adds a module to the registry.
// This should be called before LoadAll.
func (r *Registry) Register(m Module) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modules[m.Name()] = m
}

// LoadAll loads all registered modules and returns the combined exports.
// This includes both namespaced exports (e.g., "strings") and global aliases (e.g., "printf").
// The function is safe to call multiple times; modules are only loaded once.
func (r *Registry) LoadAll() (starlark.StringDict, error) {
	var loadErr error

	r.loadOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()

		for name, mod := range r.modules {
			exports, err := mod.Load(r.config)
			if err != nil {
				r.loadErrors = append(r.loadErrors, fmt.Errorf("failed to load module %s: %w", name, err))
				continue
			}

			// Add module exports to loaded map
			for k, v := range exports {
				r.loaded[k] = v
			}

			// Collect global aliases
			if aliases := mod.Aliases(); aliases != nil {
				for k, v := range aliases {
					r.aliases[k] = v
				}
			}
		}

		if len(r.loadErrors) > 0 {
			loadErr = fmt.Errorf("module loading errors: %v", r.loadErrors)
		}
	})

	return r.loaded, loadErr
}

// GetAliases returns all global aliases from loaded modules.
// Must be called after LoadAll.
func (r *Registry) GetAliases() starlark.StringDict {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.aliases
}

// Get retrieves a specific module by name.
func (r *Registry) Get(name ModuleName) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.modules[name]
	return m, ok
}

// GetLoaded returns the loaded exports for a module by name.
func (r *Registry) GetLoaded(name string) (starlark.Value, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.loaded[name]
	return v, ok
}

// All returns all registered modules.
func (r *Registry) All() map[ModuleName]Module {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[ModuleName]Module, len(r.modules))
	for k, v := range r.modules {
		result[k] = v
	}
	return result
}

// Names returns the names of all registered modules.
func (r *Registry) Names() []ModuleName {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]ModuleName, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}

// Close releases resources held by modules that implement io.Closer.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, mod := range r.modules {
		if closer, ok := mod.(io.Closer); ok {
			closer.Close()
		}
	}
}

// Predeclared returns all predeclared symbols for the Starlark runtime.
// This combines module namespaces and global aliases.
func (r *Registry) Predeclared() starlark.StringDict {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(starlark.StringDict, len(r.loaded)+len(r.aliases))

	// Add namespaced modules
	for k, v := range r.loaded {
		result[k] = v
	}

	// Add global aliases
	for k, v := range r.aliases {
		result[k] = v
	}

	return result
}
