package libkite

import (
	"context"
	"sync"

	"go.starlark.net/starlark"
)

type inFlightLoad struct {
	done   chan struct{}
	result starlark.StringDict
	err    error
}

// ModuleCache caches loaded external modules to prevent duplicate execution and
// coordinates concurrent loads of the same module path.
type ModuleCache struct {
	mu       sync.RWMutex
	modules  map[string]starlark.StringDict
	inflight map[string]*inFlightLoad
}

// NewModuleCache creates a new module cache.
func NewModuleCache() *ModuleCache {
	return &ModuleCache{
		modules:  make(map[string]starlark.StringDict),
		inflight: make(map[string]*inFlightLoad),
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
}

// loadOrJoin coordinates concurrent module loads for the same path. When
// cached or another goroutine is already loading path, it waits (unless ctx is
// cancelled) and returns that result with owner == false. Otherwise it marks the
// caller as the sole loader (owner == true), returning a finish callback that
// MUST be deferred to publish either exports or err.
func (c *ModuleCache) loadOrJoin(ctx context.Context, path string) (
	cached starlark.StringDict,
	owner bool,
	finish func(starlark.StringDict, error),
	err error,
) {
	c.mu.Lock()
	if m, ok := c.modules[path]; ok {
		c.mu.Unlock()
		return m, false, nil, nil
	}
	if entry, loading := c.inflight[path]; loading {
		c.mu.Unlock()
		if ctx != nil && ctx.Done() != nil {
			select {
			case <-entry.done:
			case <-ctx.Done():
				return nil, false, nil, ctx.Err()
			}
		} else {
			<-entry.done
		}
		if entry.err == nil && entry.result != nil {
			return entry.result, false, nil, nil
		}
		return nil, false, nil, entry.err
	}

	entry := &inFlightLoad{done: make(chan struct{})}
	c.inflight[path] = entry
	c.mu.Unlock()

	var once sync.Once
	finish = func(res starlark.StringDict, loadErr error) {
		once.Do(func() {
			c.mu.Lock()
			if loadErr == nil && res != nil {
				c.modules[path] = res
			}
			entry.result = res
			entry.err = loadErr
			delete(c.inflight, path)
			close(entry.done)
			c.mu.Unlock()
		})
	}
	return nil, true, finish, nil
}

// Clear removes all cached modules.
func (c *ModuleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modules = make(map[string]starlark.StringDict)
	c.inflight = make(map[string]*inFlightLoad)
}

// Size returns the number of cached modules.
func (c *ModuleCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.modules)
}
