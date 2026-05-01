package libkite

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
	strict     bool
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

// SetStrict toggles duplicate-detection. When true, Register panics on a
// duplicate module name and LoadAll surfaces an error if two modules export
// the same top-level name or global alias. Off by default to preserve the
// lenient behavior the lean editions rely on. The all-edition opts in.
func (r *Registry) SetStrict(strict bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strict = strict
}

// Register adds a module to the registry.
// This should be called before LoadAll.
func (r *Registry) Register(m Module) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.strict {
		if existing, ok := r.modules[m.Name()]; ok {
			panic(fmt.Errorf("libkite: duplicate module registration: %q (existing: %T, new: %T)",
				m.Name(), existing, m))
		}
	}
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
				if r.strict {
					if _, exists := r.loaded[k]; exists {
						r.loadErrors = append(r.loadErrors,
							fmt.Errorf("libkite: duplicate export %q from module %s", k, name))
						continue
					}
				}
				r.loaded[k] = v
			}

			// Collect global aliases
			if aliases := mod.Aliases(); aliases != nil {
				for k, v := range aliases {
					if r.strict {
						if _, exists := r.aliases[k]; exists {
							r.loadErrors = append(r.loadErrors,
								fmt.Errorf("libkite: duplicate global alias %q from module %s", k, name))
							continue
						}
					}
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
