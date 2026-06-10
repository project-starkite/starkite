package libkite

import (
	"log/slog"

	"go.starlark.net/starlark"
)

// Config configures a libkite Runtime.
type Config struct {
	// Permissions defines the permission policy.
	// If nil, all operations are allowed (trusted mode for CLI tools).
	Permissions *PermissionConfig

	// Modules specifies which modules to enable.
	// If nil, all registered modules are enabled.
	Modules []string

	// Globals are variables injected into every script.
	Globals map[string]interface{}

	// Print overrides the default print function.
	// If nil, output goes to stdout.
	Print func(thread *starlark.Thread, msg string)

	// ModuleConfig is passed to modules during loading.
	ModuleConfig *ModuleConfig

	// Registry is the module registry to use.
	// If nil, a new registry is created.
	Registry *Registry

	// ScriptPath is the path to the script being executed.
	// Used for relative path resolution and error messages.
	ScriptPath string

	// WorkDir is the working directory for script execution.
	// If empty, uses the current directory.
	WorkDir string

	// Debug enables debug logging.
	Debug bool

	// DryRun simulates operations without making changes.
	DryRun bool

	// OutputFormat specifies the output format: text, json, yaml, table.
	OutputFormat string

	// TestMode enables test mode (for test_* functions).
	TestMode bool

	// VarStore provides access to variables.
	// Uses the VarStore interface for loose coupling.
	VarStore VarStore

	// EntryPoint names a function the runtime invokes automatically once the
	// script's top-level code finishes, when the script defines it as a
	// callable and does not call it itself. Empty disables automatic
	// invocation. The CLI sets this to "main".
	EntryPoint string

	// RequireEntryPoint makes a missing EntryPoint a hard error: the file must
	// define EntryPoint, otherwise it is a library and not directly runnable.
	// The CLI sets this for module runs (a directory or namespace/name), not
	// for loose script files.
	RequireEntryPoint bool

	// Logger receives runtime session diagnostics, such as a skipped automatic
	// entry-point invocation. If nil, a text logger to stderr is used.
	Logger *slog.Logger
}

// ConfigOption is a functional option for Config.
type ConfigOption func(*Config)

// WithPermissions sets the permission config.
func WithPermissions(perms *PermissionConfig) ConfigOption {
	return func(c *Config) {
		c.Permissions = perms
	}
}

// WithTrusted sets trusted permissions (allow all).
func WithTrusted() ConfigOption {
	return func(c *Config) {
		c.Permissions = AllowAllPermissions()
	}
}

// WithSandboxed sets sandboxed permissions (safe modules only).
func WithSandboxed() ConfigOption {
	return func(c *Config) {
		c.Permissions = DenyAllPermissions()
	}
}

// WithModules sets which modules to enable.
func WithModules(modules ...string) ConfigOption {
	return func(c *Config) {
		c.Modules = modules
	}
}

// WithGlobals sets global variables.
func WithGlobals(globals map[string]interface{}) ConfigOption {
	return func(c *Config) {
		c.Globals = globals
	}
}

// WithPrint sets the print function.
func WithPrint(fn func(thread *starlark.Thread, msg string)) ConfigOption {
	return func(c *Config) {
		c.Print = fn
	}
}

// WithRegistry sets the module registry.
func WithRegistry(r *Registry) ConfigOption {
	return func(c *Config) {
		c.Registry = r
	}
}

// WithScriptPath sets the script path.
func WithScriptPath(path string) ConfigOption {
	return func(c *Config) {
		c.ScriptPath = path
	}
}

// WithWorkDir sets the working directory.
func WithWorkDir(dir string) ConfigOption {
	return func(c *Config) {
		c.WorkDir = dir
	}
}

// WithDebug enables debug mode.
func WithDebug(debug bool) ConfigOption {
	return func(c *Config) {
		c.Debug = debug
	}
}

// WithDryRun enables dry-run mode.
func WithDryRun(dryRun bool) ConfigOption {
	return func(c *Config) {
		c.DryRun = dryRun
	}
}

// WithOutputFormat sets the output format.
func WithOutputFormat(format string) ConfigOption {
	return func(c *Config) {
		c.OutputFormat = format
	}
}

// WithTestMode enables test mode.
func WithTestMode(testMode bool) ConfigOption {
	return func(c *Config) {
		c.TestMode = testMode
	}
}

// WithVarStore sets the variable store.
func WithVarStore(vs VarStore) ConfigOption {
	return func(c *Config) {
		c.VarStore = vs
	}
}

// WithEntryPoint sets the automatic entry-point function name.
func WithEntryPoint(name string) ConfigOption {
	return func(c *Config) {
		c.EntryPoint = name
	}
}

// WithLogger sets the logger for runtime session diagnostics.
func WithLogger(l *slog.Logger) ConfigOption {
	return func(c *Config) {
		c.Logger = l
	}
}

// NewConfig creates a new Config with the given options.
func NewConfig(opts ...ConfigOption) *Config {
	cfg := &Config{
		OutputFormat: "text",
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}
