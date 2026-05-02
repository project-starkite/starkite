package wasm

import (
	"context"
	"fmt"
	"os"
	"sync"

	extism "github.com/extism/go-sdk"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/libkite"
)

// WasmModule wraps an Extism plugin behind the standard libkite.Module interface.
// Plugins are loaded lazily on first call to Load().
type WasmModule struct {
	manifest *PluginManifest
	wasmPath string

	once     sync.Once
	plugin   *extism.Plugin
	compiled *extism.CompiledPlugin
	module   starlark.Value
	loadErr  error
	config   *libkite.ModuleConfig
	mu       sync.Mutex
}

// NewWasmModule creates a WasmModule from a discovered plugin.
func NewWasmModule(discovered *DiscoveredPlugin) *WasmModule {
	return &WasmModule{
		manifest: discovered.Manifest,
		wasmPath: discovered.WasmPath,
	}
}

func (m *WasmModule) Name() libkite.ModuleName {
	return libkite.ModuleName(m.manifest.Name)
}

func (m *WasmModule) Description() string {
	return m.manifest.Description
}

func (m *WasmModule) Aliases() starlark.StringDict {
	return nil
}

func (m *WasmModule) FactoryMethod() string {
	return ""
}

// Load initializes the WASM plugin and builds the Starlark module.
// Uses sync.Once for lazy initialization matching the native module pattern.
func (m *WasmModule) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.loadErr = m.initialize()
	})
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return starlark.StringDict{string(m.Name()): m.module}, nil
}

// Close releases Extism plugin resources. Implements io.Closer.
func (m *WasmModule) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx := context.Background()
	if m.plugin != nil {
		m.plugin.Close(ctx)
		m.plugin = nil
	}
	if m.compiled != nil {
		m.compiled.Close(ctx)
		m.compiled = nil
	}
	return nil
}

// initialize performs the actual WASM loading and Starlark module construction.
func (m *WasmModule) initialize() error {
	// Read the .wasm file
	wasmData, err := os.ReadFile(m.wasmPath)
	if err != nil {
		return &WasmError{
			Plugin:  m.manifest.Name,
			Message: fmt.Sprintf("read wasm file: %v", err),
		}
	}

	// Build Extism manifest
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmData, Name: m.manifest.Name},
		},
	}

	pluginConfig := extism.PluginConfig{
		EnableWasi: true,
	}

	// Build host functions from manifest permissions
	hostCtx := &HostContext{
		config:     m.config,
		moduleName: m.manifest.Name,
	}
	hostFuncs := hostCtx.Build(m.manifest.Permissions)

	ctx := context.Background()

	// Create compiled plugin for concurrent access
	compiled, err := extism.NewCompiledPlugin(ctx, manifest, pluginConfig, hostFuncs)
	if err != nil {
		return &WasmError{
			Plugin:  m.manifest.Name,
			Message: fmt.Sprintf("compile wasm: %v", err),
		}
	}
	m.compiled = compiled

	// Create primary plugin instance
	plugin, err := compiled.Instance(ctx, extism.PluginInstanceConfig{})
	if err != nil {
		compiled.Close(ctx)
		return &WasmError{
			Plugin:  m.manifest.Name,
			Message: fmt.Sprintf("instantiate wasm: %v", err),
		}
	}
	m.plugin = plugin

	// Validate all declared exports exist
	for _, fn := range m.manifest.Functions {
		exportName := fn.ExportName()
		if !plugin.FunctionExists(exportName) {
			plugin.Close(ctx)
			compiled.Close(ctx)
			return &WasmError{
				Plugin:   m.manifest.Name,
				Function: exportName,
				Message:  "declared export not found in wasm binary",
			}
		}
	}

	// Build Starlark module with builtins for each function
	members := make(starlark.StringDict, len(m.manifest.Functions))
	for _, fn := range m.manifest.Functions {
		members[fn.Name] = m.makeBuiltin(fn)
	}

	m.module = &starlarkstruct.Module{
		Name:    m.manifest.Name,
		Members: members,
	}

	return nil
}

// makeBuiltin creates a starlark.Builtin closure for a WASM function.
func (m *WasmModule) makeBuiltin(fn FunctionManifest) *starlark.Builtin {
	qualifiedName := m.manifest.Name + "." + fn.Name
	exportName := fn.ExportName()
	params := fn.Params
	returnType := fn.Returns

	return starlark.NewBuiltin(qualifiedName, func(
		thread *starlark.Thread, b *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		// Check permission
		if err := libkite.Check(thread, m.manifest.Name, "wasm", fn.Name, ""); err != nil {
			return nil, err
		}

		// Set thread on host context for permission checks in host functions
		m.mu.Lock()
		hostCtx := &HostContext{
			config:     m.config,
			moduleName: m.manifest.Name,
			thread:     thread,
		}
		_ = hostCtx // Host context is set during Build; thread is needed at call time

		// Marshal args to JSON
		jsonInput, err := starlarkArgsToJSON(params, args, kwargs)
		if err != nil {
			m.mu.Unlock()
			return nil, &WasmError{
				Plugin:   m.manifest.Name,
				Function: fn.Name,
				Message:  fmt.Sprintf("marshal args: %v", err),
			}
		}

		// Call WASM function
		_, output, err := m.plugin.Call(exportName, jsonInput)
		m.mu.Unlock()

		if err != nil {
			return nil, &WasmError{
				Plugin:   m.manifest.Name,
				Function: fn.Name,
				Message:  "call failed",
				Cause:    err,
			}
		}

		// Unmarshal result
		if returnType == "" || returnType == "none" {
			return starlark.None, nil
		}

		result, err := jsonToStarlark(output, returnType)
		if err != nil {
			return nil, &WasmError{
				Plugin:   m.manifest.Name,
				Function: fn.Name,
				Message:  fmt.Sprintf("unmarshal result: %v", err),
			}
		}

		return result, nil
	})
}
