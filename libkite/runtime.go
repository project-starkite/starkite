package libkite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// runtimeKey is the thread.Local key for the Runtime reference.
const runtimeKey = "libkite.runtime"

// Runtime executes Starlark code with configured modules and permissions.
type Runtime struct {
	config      *Config
	registry    *Registry
	permissions *PermissionChecker
	globals     starlark.StringDict
	mu          sync.RWMutex

	// Thread and predeclared symbols
	thread  *starlark.Thread
	predecl starlark.StringDict

	// Signal handling
	signalHandlers map[string]starlark.Callable
	signalChan     chan os.Signal
	signalMu       sync.RWMutex

	// Deferred functions
	deferredFuncs []starlark.Callable
	deferMu       sync.Mutex

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Module cache for external modules
	moduleCache *ModuleCache
}

// New creates a Runtime with the given config.
// If config is nil, uses secure defaults (trusted permissions for CLI tools).
func New(config *Config) (*Runtime, error) {
	if config == nil {
		config = &Config{}
	}

	// Set defaults
	if config.OutputFormat == "" {
		config.OutputFormat = "text"
	}

	ctx, cancel := context.WithCancel(context.Background())

	rt := &Runtime{
		config:         config,
		globals:        make(starlark.StringDict),
		signalHandlers: make(map[string]starlark.Callable),
		ctx:            ctx,
		cancel:         cancel,
		moduleCache:    NewModuleCache(),
	}

	// Create permission checker if permissions are configured
	if config.Permissions != nil {
		checker, err := NewPermissionChecker(config.Permissions)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid permission config: %w", err)
		}
		rt.permissions = checker
	}

	// Create module config
	moduleConfig := config.ModuleConfig
	if moduleConfig == nil {
		moduleConfig = &ModuleConfig{
			DryRun: config.DryRun,
			Debug:  config.Debug,
		}
		if config.VarStore != nil {
			moduleConfig.VarStore = config.VarStore
		}
	}

	// Use provided registry or create new one
	registry := config.Registry
	if registry == nil {
		registry = NewRegistry(moduleConfig)
	}
	rt.registry = registry

	// Load all modules
	if _, err := rt.registry.LoadAll(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load modules: %w", err)
	}

	// Build predeclared symbols
	rt.predecl = rt.buildPredeclared()

	// Convert user globals to Starlark values
	for k, v := range config.Globals {
		sv, err := toStarlarkValue(v)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid global %q: %w", k, err)
		}
		rt.globals[k] = sv
		rt.predecl[k] = sv
	}

	// Create Starlark thread
	rt.thread = &starlark.Thread{
		Name: "libkite",
		Load: rt.loadModule,
		Print: func(thread *starlark.Thread, msg string) {
			if config.Print != nil {
				config.Print(thread, msg)
			} else {
				fmt.Println(msg)
			}
		},
	}

	// Set permissions on main thread if configured
	if rt.permissions != nil {
		SetPermissions(rt.thread, rt.permissions)
	}

	// Store runtime reference in thread.Local for modules that need it
	rt.thread.SetLocal(runtimeKey, rt)

	// Set up signal handling
	rt.setupSignalHandling()

	return rt, nil
}

// NewTrusted creates a Runtime that allows ALL operations.
// Accepts an optional Config struct and/or functional options.
// Use for CLI tools, trusted scripts, or when you manage security externally.
func NewTrusted(config *Config, opts ...ConfigOption) (*Runtime, error) {
	if config == nil {
		config = &Config{}
	}
	for _, opt := range opts {
		opt(config)
	}
	config.Permissions = TrustedPermissions()
	return New(config)
}

// NewSandboxed creates a Runtime with only safe modules allowed.
// Accepts an optional Config struct and/or functional options.
func NewSandboxed(config *Config, opts ...ConfigOption) (*Runtime, error) {
	if config == nil {
		config = &Config{}
	}
	for _, opt := range opts {
		opt(config)
	}
	config.Permissions = SandboxedPermissions()
	return New(config)
}

// buildPredeclared builds the predeclared symbol table.
func (rt *Runtime) buildPredeclared() starlark.StringDict {
	predecl := rt.registry.Predeclared()

	// Add runtime control functions
	predecl["fail"] = starlark.NewBuiltin("fail", rt.builtinFail)
	predecl["exit"] = starlark.NewBuiltin("exit", rt.builtinExit)
	predecl["defer"] = starlark.NewBuiltin("defer", rt.builtinDefer)
	predecl["on_signal"] = starlark.NewBuiltin("on_signal", rt.builtinOnSignal)
	predecl["Result"] = starlark.NewBuiltin("Result", rt.builtinResult)

	return predecl
}

// Registry returns the module registry.
func (rt *Runtime) Registry() *Registry {
	return rt.registry
}

// Permissions returns the permission checker.
func (rt *Runtime) Permissions() *PermissionChecker {
	return rt.permissions
}

// NewThread creates a new Starlark thread with permissions set.
// This is used internally and can be used by embedders for custom execution.
func (rt *Runtime) NewThread(name string) *starlark.Thread {
	thread := &starlark.Thread{
		Name: name,
		Load: rt.loadModule,
	}

	// Set print function
	if rt.config.Print != nil {
		thread.Print = rt.config.Print
	} else {
		thread.Print = func(_ *starlark.Thread, msg string) {
			fmt.Println(msg)
		}
	}

	// Set permission checker in thread.Local
	if rt.permissions != nil {
		SetPermissions(thread, rt.permissions)
	}

	// Store runtime reference in thread.Local
	thread.SetLocal(runtimeKey, rt)

	return thread
}

// GetRuntime retrieves the Runtime from a thread's local storage.
// Returns nil if no runtime is set.
func GetRuntime(thread *starlark.Thread) *Runtime {
	if thread == nil {
		return nil
	}
	rt, _ := thread.Local(runtimeKey).(*Runtime)
	return rt
}

// Execute runs a script. The ctx parameter cancels a running script: when
// ctx.Done() fires, the Starlark thread receives Cancel() and execution exits
// at the next safe point.
func (rt *Runtime) Execute(ctx context.Context, code string) error {
	scriptPath := rt.config.ScriptPath
	if scriptPath == "" {
		scriptPath = "<script>"
	}

	stop := watchCtx(ctx, rt.thread)
	defer stop()

	_, err := starlark.ExecFileOptions(
		&syntax.FileOptions{},
		rt.thread,
		scriptPath,
		code,
		rt.predecl,
	)

	// Run deferred functions in LIFO order
	rt.runDeferred()

	if err != nil {
		if code, ok := unwrapExit(err); ok {
			if code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		}
		if evalErr, ok := err.(*starlark.EvalError); ok {
			return NewScriptError("script error", ExitScriptError, evalErr)
		}
		return err
	}

	return nil
}

// unwrapExit walks the error chain for *exitError and returns its code.
func unwrapExit(err error) (int, bool) {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code, true
	}
	return 0, false
}

// ExecuteRepl runs code in REPL mode, preserving state between calls.
// An exit() call inside the REPL fragment surfaces as *ExitError.
func (rt *Runtime) ExecuteRepl(ctx context.Context, code string) error {
	stop := watchCtx(ctx, rt.thread)
	defer stop()

	globals, err := starlark.ExecFileOptions(
		&syntax.FileOptions{},
		rt.thread,
		"<repl>",
		code,
		rt.predecl,
	)

	if err != nil {
		if exitCode, ok := unwrapExit(err); ok {
			if exitCode != 0 {
				return &ExitError{Code: exitCode}
			}
			return nil
		}
		return err
	}

	rt.mu.Lock()
	for k, v := range globals {
		rt.globals[k] = v
		rt.predecl[k] = v
	}
	rt.mu.Unlock()

	return nil
}

// Call invokes a top-level callable named `name` with positional and keyword
// arguments. Each value in args and kwargs is converted to a starlark.Value
// via startype before the call. Pass nil for either slot if unused.
//
// The ctx parameter cancels a running call: when ctx.Done() fires, the
// Starlark thread receives Cancel() and the execution loop exits at its next
// safe point. Pass context.Background() when no cancellation is desired.
//
// Call uses a fresh thread per invocation so concurrent callers do not share
// state. Values returned may wrap live runtime resources — consume them
// before calling Close.
func (rt *Runtime) Call(ctx context.Context, name string, args []any, kwargs map[string]any) (starlark.Value, error) {
	rt.mu.RLock()
	val, ok := rt.globals[name]
	rt.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("libkite: global %q not defined", name)
	}
	fn, ok := val.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("libkite: global %q is not callable (type %s)", name, val.Type())
	}
	pos, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	kw, err := convertKwargs(kwargs)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	thread := rt.NewThread("call:" + name)
	stop := watchCtx(ctx, thread)
	defer stop()
	return starlark.Call(thread, fn, pos, kw)
}

// Eval evaluates src as a single Starlark expression against the runtime's
// predeclared symbols and accumulated globals. Returns the value of the
// expression.
//
// Errors if src is not a valid expression. Use ExecuteRepl for statements.
func (rt *Runtime) Eval(ctx context.Context, src string) (starlark.Value, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	rt.mu.RLock()
	env := make(starlark.StringDict, len(rt.predecl))
	for k, v := range rt.predecl {
		env[k] = v
	}
	rt.mu.RUnlock()
	thread := rt.NewThread("eval")
	stop := watchCtx(ctx, thread)
	defer stop()
	return starlark.EvalOptions(&syntax.FileOptions{}, thread, "<eval>", src, env)
}

// CallFn invokes an already-resolved Starlark callable with positional and
// keyword args. Skips the global-name lookup that Call does — useful when the
// caller already holds a callable (e.g., from GetGlobalVal).
func (rt *Runtime) CallFn(ctx context.Context, fn starlark.Callable, args []any, kwargs map[string]any) (starlark.Value, error) {
	if fn == nil {
		return nil, fmt.Errorf("libkite: CallFn: fn is nil")
	}
	pos, err := convertArgs(args)
	if err != nil {
		return nil, err
	}
	kw, err := convertKwargs(kwargs)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	thread := rt.NewThread("callfn:" + fn.Name())
	stop := watchCtx(ctx, thread)
	defer stop()
	return starlark.Call(thread, fn, pos, kw)
}

// GetGlobalVal returns the starlark.Value bound to `name` in the runtime's
// globals, or (nil, false) if no such global exists.
func (rt *Runtime) GetGlobalVal(name string) (starlark.Value, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	v, ok := rt.globals[name]
	return v, ok
}

// watchCtx wires ctx.Done() to thread.Cancel. Returns a stop func that must
// be called when Starlark execution completes so the watcher goroutine exits
// cleanly.
func watchCtx(ctx context.Context, thread *starlark.Thread) func() {
	if ctx == nil || ctx.Done() == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			thread.Cancel(ctx.Err().Error())
		case <-done:
		}
	}()
	return func() { close(done) }
}

// convertArgs converts a Go positional-arg slice to starlark.Tuple.
func convertArgs(in []any) (starlark.Tuple, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(starlark.Tuple, 0, len(in))
	for i, v := range in {
		sv, err := startype.Go[any](v).ToStarlarkValue()
		if err != nil {
			return nil, fmt.Errorf("libkite: arg %d: %w", i, err)
		}
		out = append(out, sv)
	}
	return out, nil
}

// convertKwargs converts a Go keyword-arg map to the starlark.Tuple slice
// form expected by starlark.Call.
func convertKwargs(in map[string]any) ([]starlark.Tuple, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]starlark.Tuple, 0, len(in))
	for k, v := range in {
		sv, err := startype.Go[any](v).ToStarlarkValue()
		if err != nil {
			return nil, fmt.Errorf("libkite: kwarg %q: %w", k, err)
		}
		out = append(out, starlark.Tuple{starlark.String(k), sv})
	}
	return out, nil
}

// ExecuteTests finds and runs all test_* functions.
func (rt *Runtime) ExecuteTests(ctx context.Context, code string) ([]TestResult, error) {
	return rt.ExecuteTestsWithConfig(ctx, code, TestConfig{})
}

// ExecuteTestsWithConfig finds and runs test_* functions with configuration.
// A module-level exit() returns *ExitError to the caller. An exit() inside a
// test function fails that test with a visible message (not a silent exit).
func (rt *Runtime) ExecuteTestsWithConfig(ctx context.Context, code string, cfg TestConfig) ([]TestResult, error) {
	scriptPath := rt.config.ScriptPath
	if scriptPath == "" {
		scriptPath = "<test>"
	}

	stop := watchCtx(ctx, rt.thread)
	globals, err := starlark.ExecFileOptions(
		&syntax.FileOptions{},
		rt.thread,
		scriptPath,
		code,
		rt.predecl,
	)
	stop()

	if err != nil {
		if exitCode, ok := unwrapExit(err); ok {
			if exitCode != 0 {
				return nil, &ExitError{Code: exitCode}
			}
			return nil, nil
		}
		return nil, err
	}

	// Collect test functions and sort by name for consistent ordering
	var testNames []string
	testFuncs := make(map[string]starlark.Callable)
	var setupFn, teardownFn starlark.Callable

	for name, val := range globals {
		if fn, ok := val.(starlark.Callable); ok {
			switch name {
			case "setup":
				setupFn = fn
			case "teardown":
				teardownFn = fn
			default:
				if len(name) > 5 && name[:5] == "test_" {
					// Apply filter if specified
					if cfg.Filter == "" || strings.Contains(name, cfg.Filter) {
						testNames = append(testNames, name)
						testFuncs[name] = fn
					}
				}
			}
		}
	}

	// Sort test names for consistent ordering
	sort.Strings(testNames)

	// Run tests
	var results []TestResult
	for _, name := range testNames {
		fn := testFuncs[name]
		result := rt.runTest(ctx, name, fn, setupFn, teardownFn)
		results = append(results, result)
	}

	return results, nil
}

func (rt *Runtime) runTest(ctx context.Context, name string, fn, setupFn, teardownFn starlark.Callable) TestResult {
	start := time.Now()

	// Run setup if defined
	if setupFn != nil {
		if _, err := starlark.Call(rt.thread, setupFn, nil, nil); err != nil {
			return TestResult{
				Name:     name,
				Passed:   false,
				Duration: time.Since(start),
				Error:    fmt.Errorf("setup failed: %w", err),
			}
		}
	}

	// Run the test with ctx wired so a runaway or canceled test stops promptly.
	testThread := rt.NewThread("test:" + name)
	stopWatch := watchCtx(ctx, testThread)
	_, err := starlark.Call(testThread, fn, nil, nil)
	stopWatch()

	// Run teardown if defined (always runs, even if test failed)
	if teardownFn != nil {
		if _, teardownErr := starlark.Call(rt.thread, teardownFn, nil, nil); teardownErr != nil {
			// If test passed but teardown failed, report teardown error
			if err == nil {
				err = fmt.Errorf("teardown failed: %w", teardownErr)
			}
		}
	}

	if err != nil {
		// exit() inside a test becomes a visible failure, not a silent exit.
		if exitCode, ok := unwrapExit(err); ok {
			return TestResult{
				Name:     name,
				Passed:   false,
				Duration: time.Since(start),
				Error:    fmt.Errorf("test called exit(%d): %w", exitCode, &ExitError{Code: exitCode}),
			}
		}
		if skipErr, ok := err.(*SkipError); ok {
			return TestResult{
				Name:     name,
				Passed:   true,
				Skipped:  true,
				Duration: time.Since(start),
				Error:    skipErr,
			}
		}
		// Check for wrapped skip error in Starlark error
		if strings.Contains(err.Error(), "test skipped") {
			return TestResult{
				Name:     name,
				Passed:   true,
				Skipped:  true,
				Duration: time.Since(start),
				Error:    err,
			}
		}
	}

	return TestResult{
		Name:     name,
		Passed:   err == nil,
		Duration: time.Since(start),
		Error:    err,
	}
}

// PrintVariables prints all currently loaded variables to stdout.
func (rt *Runtime) PrintVariables() {
	if rt.config.VarStore == nil {
		fmt.Println("No variables loaded")
		return
	}

	// Use the All() method if available
	type allProvider interface {
		All() map[string]interface{}
	}

	if ap, ok := rt.config.VarStore.(allProvider); ok {
		vars := ap.All()
		if len(vars) == 0 {
			fmt.Println("No variables loaded")
			return
		}

		fmt.Println("Variables:")
		for k, v := range vars {
			fmt.Printf("  %s = %v\n", k, v)
		}
	} else {
		fmt.Println("Variable store does not support listing all variables")
	}
}

// Close cleans up runtime resources.
func (rt *Runtime) Close() {
	rt.cancel()
	rt.stopSignalHandling()
	rt.registry.Close()
}

// Cleanup is an alias for Close (for backward compatibility).
func (rt *Runtime) Cleanup() {
	rt.Close()
}

// toStarlarkValue converts a Go value to a Starlark value.
func toStarlarkValue(v interface{}) (starlark.Value, error) {
	return startype.Go[any](v).ToStarlarkValue()
}
