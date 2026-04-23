package starbase

import (
	"go.starlark.net/starlark"
)

// ModuleName is the identifier for a module.
type ModuleName string

// Module is the interface for all starbase modules.
// This matches the existing starkite module interface exactly.
type Module interface {
	// Name returns the module's identifier
	Name() ModuleName

	// Load initializes the module and returns its exports.
	// The returned StringDict contains the module's namespace (e.g., "strings" -> module).
	Load(config *ModuleConfig) (starlark.StringDict, error)

	// Description returns a human-readable description of the module
	Description() string

	// Aliases returns global aliases that should be exposed at the top level.
	// For example, os module returns {"env": ..., "exec": ...}.
	// Return nil if the module has no global aliases.
	Aliases() starlark.StringDict

	// FactoryMethod returns the name of the factory function for stateful modules.
	// For example, "config" for ssh.config() which returns an SSH client.
	// Return "" if the module is not a factory module.
	FactoryMethod() string
}

// ModuleConfig provides configuration context for module loading.
type ModuleConfig struct {
	// DryRun indicates whether operations should be simulated
	DryRun bool

	// Debug enables debug logging
	Debug bool

	// TestMode enables test-only module features (e.g. ssh.testserver)
	TestMode bool

	// VarStore provides access to the variable store for modules that need it
	VarStore VarStore
}

// VarStore is the interface for variable access used by modules.
type VarStore interface {
	// Get retrieves a variable value by key
	Get(key string) (interface{}, bool)

	// GetWithDefault retrieves a variable value with a default fallback
	GetWithDefault(key string, defaultValue interface{}) interface{}

	// GetString retrieves a variable value as a string
	GetString(key string) string

	// Keys returns sorted, deduplicated variable names across all priority tiers
	Keys() []string
}
