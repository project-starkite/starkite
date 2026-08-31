package sandbox

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
)

// Driver name constants for standard sandbox backends.
const (
	DriverDefault  = "default"
	DriverAuto     = "auto"
	DriverLandlock = "landlock"
	DriverSeatbelt = "seatbelt"
	DriverPodman   = "podman"
	DriverDocker   = "docker"
	DriverNerdctl  = "nerdctl"
	DriverGVisor   = "gvisor"
	DriverWasm     = "wasm"
)

// Registry manages available sandbox drivers in a thread-safe manner.
type Registry struct {
	mu      sync.RWMutex
	drivers map[string]Driver
}

// NewRegistry creates an empty sandbox driver registry.
func NewRegistry() *Registry {
	return &Registry{
		drivers: make(map[string]Driver),
	}
}

var defaultRegistry = NewRegistry()

// Register adds a sandbox driver to the default global registry.
func Register(driver Driver) {
	defaultRegistry.Register(driver)
}

// Unregister removes a driver by name from the default global registry.
func Unregister(name string) {
	defaultRegistry.Unregister(name)
}

// Get retrieves a driver by name from the default global registry.
func Get(name string) (Driver, error) {
	return defaultRegistry.Get(name)
}

// List returns a sorted list of registered driver names from the default global registry.
func List() []string {
	return defaultRegistry.List()
}

// AutoDetect returns the preferred available driver for the current host OS.
func AutoDetect() (Driver, error) {
	return defaultRegistry.AutoDetect()
}

// Resolve resolves a driver name (supporting "default" and "auto" aliases)
// to a usable Driver instance from the default global registry.
func Resolve(name string) (Driver, error) {
	return defaultRegistry.Resolve(name)
}

// Register adds a sandbox driver to the registry.
func (r *Registry) Register(driver Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if driver != nil {
		r.drivers[driver.Name()] = driver
	}
}

// Unregister removes a driver by name from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.drivers, name)
}

// Get retrieves a driver by name.
func (r *Registry) Get(name string) (Driver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.drivers[name]
	if !ok {
		return nil, fmt.Errorf("sandbox: driver %q not registered", name)
	}
	return d, nil
}

// List returns a sorted slice of all registered driver names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.drivers))
	for name := range r.drivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AutoDetect selects the preferred driver for the current operating system.
func (r *Registry) AutoDetect() (Driver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	switch runtime.GOOS {
	case "linux":
		// Prefer Landlock on Linux (native in-process kernel LSM)
		if d, ok := r.drivers[DriverLandlock]; ok && d.Available() {
			return d, nil
		}
		// Fallback to gVisor or Podman if Landlock is unavailable
		if d, ok := r.drivers[DriverGVisor]; ok && d.Available() {
			return d, nil
		}
		if d, ok := r.drivers[DriverPodman]; ok && d.Available() {
			return d, nil
		}
		if d, ok := r.drivers[DriverDocker]; ok && d.Available() {
			return d, nil
		}
	case "darwin":
		// Prefer Seatbelt on macOS
		if d, ok := r.drivers[DriverSeatbelt]; ok && d.Available() {
			return d, nil
		}
		// Fallback to Docker or Podman
		if d, ok := r.drivers[DriverPodman]; ok && d.Available() {
			return d, nil
		}
		if d, ok := r.drivers[DriverDocker]; ok && d.Available() {
			return d, nil
		}
	case "windows":
		// Prefer container providers on Windows
		if d, ok := r.drivers[DriverDocker]; ok && d.Available() {
			return d, nil
		}
		if d, ok := r.drivers[DriverPodman]; ok && d.Available() {
			return d, nil
		}
	}

	// Fallback to any available registered driver
	for _, name := range []string{DriverLandlock, DriverSeatbelt, DriverPodman, DriverDocker, DriverNerdctl, DriverGVisor, DriverWasm} {
		if d, ok := r.drivers[name]; ok && d.Available() {
			return d, nil
		}
	}

	return nil, fmt.Errorf("sandbox: no usable sandbox driver available on %s", runtime.GOOS)
}

// Resolve resolves a driver name (supporting "default" and "auto" aliases)
// to a usable Driver instance.
func (r *Registry) Resolve(name string) (Driver, error) {
	if name == "" || name == DriverDefault || name == DriverAuto {
		return r.AutoDetect()
	}
	d, err := r.Get(name)
	if err != nil {
		return nil, err
	}
	if !d.Available() {
		return nil, fmt.Errorf("sandbox: driver %q is not available on %s", name, runtime.GOOS)
	}
	return d, nil
}
