package libkite

import (
	"sync"

	"go.starlark.net/starlark"
)

// ModuleCache caches loaded external modules to prevent duplicate execution.
// It also tracks modules being loaded to detect circular dependencies.
type ModuleCache struct {
	mu      sync.RWMutex
	modules map[string]starlark.StringDict
	loading map[string]bool // Tracks modules being loaded (for circular dependency detection)
}

// NewModuleCache creates a new module cache.
func NewModuleCache() *ModuleCache {
	return &ModuleCache{
		modules: make(map[string]starlark.StringDict),
		loading: make(map[string]bool),
	}
}

// Get retrieves a cached module by path.
func (c *ModuleCache) Get(path string) (starlark.StringDict, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.modules[path]
	return m, ok
}

// Set caches a loaded module by path.
func (c *ModuleCache) Set(path string, module starlark.StringDict) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modules[path] = module
	delete(c.loading, path)
}

// StartLoading marks a module as being loaded (for circular dependency detection).
// Returns false if the module is already being loaded (circular dependency).
func (c *ModuleCache) StartLoading(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loading[path] {
		return false // Already loading - circular dependency
	}
	c.loading[path] = true
	return true
}

// StopLoading marks a module as no longer loading (called on error).
func (c *ModuleCache) StopLoading(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.loading, path)
}

// Clear removes all cached modules.
func (c *ModuleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modules = make(map[string]starlark.StringDict)
	c.loading = make(map[string]bool)
}

// Size returns the number of cached modules.
func (c *ModuleCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.modules)
}
