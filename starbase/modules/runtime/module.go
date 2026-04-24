// Package runtime provides runtime and platform information for starkite.
package runtime

import (
	"fmt"
	goruntime "runtime"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/starbase"
)

// Version is set at build time
var Version = "dev"

const ModuleName starbase.ModuleName = "runtime"

// Module implements runtime information functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "runtime provides platform and runtime information: platform, arch, cpu_count, uname"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			"platform":   starlark.NewBuiltin("runtime.platform", m.platform),
			"arch":       starlark.NewBuiltin("runtime.arch", m.arch),
			"cpu_count":  starlark.NewBuiltin("runtime.cpu_count", m.cpuCount),
			"uname":      starlark.NewBuiltin("runtime.uname", m.uname),
			"version":    starlark.NewBuiltin("runtime.version", m.version),
			"go_version": starlark.NewBuiltin("runtime.go_version", m.goVersion),
		}

		m.module = &starlarkstruct.Module{
			Name:    string(ModuleName),
			Members: members,
		}
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return nil // No global aliases for runtime
}

func (m *Module) FactoryMethod() string { return "" }

// platform returns the operating system name.
func (m *Module) platform(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("platform takes no arguments")
	}
	return starlark.String(goruntime.GOOS), nil
}

// arch returns the CPU architecture.
func (m *Module) arch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("arch takes no arguments")
	}
	return starlark.String(goruntime.GOARCH), nil
}

// cpuCount returns the number of CPUs.
func (m *Module) cpuCount(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("cpu_count takes no arguments")
	}
	return starlark.MakeInt(goruntime.NumCPU()), nil
}

// version returns the starkite version.
func (m *Module) version(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("version takes no arguments")
	}
	return starlark.String(Version), nil
}

// goVersion returns the Go runtime version.
func (m *Module) goVersion(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("go_version takes no arguments")
	}
	return starlark.String(goruntime.Version()), nil
}
