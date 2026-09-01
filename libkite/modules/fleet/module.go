// Package fleet provides compute resource fleet management for starkite.
package fleet

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	fleetpkg "github.com/project-starkite/starkite/libkite/fleet"
)

const ModuleName libkite.ModuleName = "fleet"

// Module implements compute fleet management.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

// New creates a new Fleet module instance.
func New() *Module { return &Module{} }

// Name returns the module identifier.
func (m *Module) Name() libkite.ModuleName { return ModuleName }

// Description returns the module documentation.
func (m *Module) Description() string {
	return "fleet provides compute resource fleet management: new, file, hosts_file"
}

// Load initializes the module and its built-in functions.
func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		members := starlark.StringDict{
			"new":        starlark.NewBuiltin("fleet.new", m.newFleet),
			"file":       starlark.NewBuiltin("fleet.file", m.file),
			"hosts_file": starlark.NewBuiltin("fleet.hosts_file", m.hostsFile),
			"host_file":  starlark.NewBuiltin("fleet.host_file", m.hostsFile),
		}
		m.module = libkite.NewTryModule(string(ModuleName), members)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

// Aliases returns global aliases for the module.
func (m *Module) Aliases() starlark.StringDict { return nil }

// FactoryMethod returns the primary constructor if any.
func (m *Module) FactoryMethod() string { return "new" }

// newFleet is the canonical factory constructor for creating a Fleet.
// Signatures:
//   - fleet.new() -> empty fleet
//   - fleet.new(source) -> positional source (list, callable, json string, dict)
//   - fleet.new(file="path/to/hosts.yaml")
//   - fleet.new(hosts_file="/etc/hosts", loopback=False)
//   - fleet.new(source=..., list=..., function=..., json=...)
func (m *Module) newFleet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var filePath string
	var hostsFilePath string
	var includeLoopback bool
	var rawSource starlark.Value
	var rawList starlark.Value
	var rawFn starlark.Callable
	var rawJSON string

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "file":
			if s, ok := starlark.AsString(kv[1]); ok {
				filePath = s
			}
		case "hosts_file", "host_file":
			if s, ok := starlark.AsString(kv[1]); ok {
				hostsFilePath = s
			} else if b, ok := kv[1].(starlark.Bool); ok && bool(b) {
				hostsFilePath = "/etc/hosts"
			}
		case "loopback":
			if b, ok := kv[1].(starlark.Bool); ok {
				includeLoopback = bool(b)
			}
		case "source":
			rawSource = kv[1]
		case "list":
			rawList = kv[1]
		case "function", "fn":
			if c, ok := kv[1].(starlark.Callable); ok {
				rawFn = c
			} else {
				return nil, fmt.Errorf("fleet.new: function must be a callable, got %s", kv[1].Type())
			}
		case "json":
			if s, ok := starlark.AsString(kv[1]); ok {
				rawJSON = s
			}
		default:
			return nil, fmt.Errorf("fleet.new: unexpected keyword argument %q", key)
		}
	}

	// 1. Dispatch file
	if filePath != "" {
		if err := libkite.Check(thread, "fs", "read", "read_file", filePath); err != nil {
			return nil, err
		}
		return fleetpkg.FromFile(filePath)
	}

	// 2. Dispatch hosts_file
	if hostsFilePath != "" {
		if err := libkite.Check(thread, "fs", "read", "read_file", hostsFilePath); err != nil {
			return nil, err
		}
		return fleetpkg.FromHostsFile(hostsFilePath, includeLoopback)
	}

	// 3. Dispatch explicit keywords
	if rawList != nil {
		return fleetpkg.FromSource(thread, rawList)
	}
	if rawFn != nil {
		return fleetpkg.FromSource(thread, rawFn)
	}
	if rawJSON != "" {
		return fleetpkg.FromJSON([]byte(rawJSON))
	}
	if rawSource != nil {
		return fleetpkg.FromSource(thread, rawSource)
	}

	// 4. Dispatch positional argument
	if len(args) == 1 {
		return fleetpkg.FromSource(thread, args[0])
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("fleet.new expects at most 1 positional argument, got %d", len(args))
	}

	// 5. Default empty fleet
	return fleetpkg.New(nil), nil
}

// file loads a fleet from a static YAML or JSON file.
// Usage: fleet.file("path/to/hosts.yaml")
func (m *Module) file(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "fs", "read", "read_file", p.Path); err != nil {
		return nil, err
	}

	return fleetpkg.FromFile(p.Path)
}

// hostsFile loads compute resources from a standard POSIX hosts file (default: /etc/hosts).
// Usage: fleet.hosts_file() OR fleet.hosts_file("infrastructure/hosts", loopback=False)
func (m *Module) hostsFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path     string `name:"path" position:"0"`
		Loopback bool   `name:"loopback"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	path := p.Path
	if path == "" {
		path = "/etc/hosts"
	}

	if err := libkite.Check(thread, "fs", "read", "read_file", path); err != nil {
		return nil, err
	}

	return fleetpkg.FromHostsFile(path, p.Loopback)
}
