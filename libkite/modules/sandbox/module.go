package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
)

const ModuleName libkite.ModuleName = "sandbox"

// Module implements the Starlark sandbox built-in module.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "sandbox provides programmatic execution isolation and container/native sandboxing"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"config":         starlark.NewBuiltin("sandbox.config", m.sandboxConfig),
				"run_script":     starlark.NewBuiltin("sandbox.run_script", m.runScript),
				"list_drivers":   starlark.NewBuiltin("sandbox.list_drivers", m.listDrivers),
				"default_driver": starlark.NewBuiltin("sandbox.default_driver", m.defaultDriver),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "config" }

// sandboxConfig creates a new configured Sandbox instance.
// Usage: sandbox.config(driver="default", image="", network="none", mounts=[], memory="512MB", cpus=2.0, pids=100, timeout="30s", env=[], cwd="", runtime="")
func (m *Module) sandboxConfig(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Driver  string         `name:"driver"`
		Image   string         `name:"image"`
		Network string         `name:"network"`
		Mounts  starlark.Value `name:"mounts"`
		Memory  starlark.Value `name:"memory"`
		CPUs    float64        `name:"cpus"`
		PIDs    int64          `name:"pids"`
		Timeout starlark.Value `name:"timeout"`
		Env     starlark.Value `name:"env"`
		Cwd     string         `name:"cwd"`
		Runtime string         `name:"runtime"`
	}
	p.Driver = sandbox.DriverDefault
	p.Network = string(sandbox.NetworkNone)

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	driver, err := sandbox.Resolve(p.Driver)
	if err != nil {
		return nil, fmt.Errorf("sandbox.config: %w", err)
	}

	// Parse mounts
	var parsedMounts []sandbox.Mount
	if p.Mounts != nil && p.Mounts != starlark.None {
		mountsList, err := parseMounts(p.Mounts)
		if err != nil {
			return nil, fmt.Errorf("sandbox.config: invalid mounts: %w", err)
		}
		parsedMounts = mountsList
	}

	// Parse memory
	var maxMemMB int64
	if p.Memory != nil && p.Memory != starlark.None {
		mem, err := parseMemory(p.Memory)
		if err != nil {
			return nil, fmt.Errorf("sandbox.config: invalid memory: %w", err)
		}
		maxMemMB = mem
	}

	// Parse timeout
	var parsedTimeout time.Duration
	if p.Timeout != nil && p.Timeout != starlark.None {
		t, err := parseTimeout(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("sandbox.config: invalid timeout: %w", err)
		}
		parsedTimeout = t
	}

	// Parse environment
	var envStrings []string
	if p.Env != nil && p.Env != starlark.None {
		switch v := p.Env.(type) {
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				s, ok := starlark.AsString(v.Index(i))
				if !ok {
					return nil, fmt.Errorf("sandbox.config: env list element %d is not a string", i)
				}
				envStrings = append(envStrings, s)
			}
		case *starlark.Dict:
			for _, item := range v.Items() {
				k, _ := starlark.AsString(item[0])
				val, _ := starlark.AsString(item[1])
				envStrings = append(envStrings, fmt.Sprintf("%s=%s", k, val))
			}
		}
	}

	return &Sandbox{
		driverName:  driver.Name(),
		driver:      driver,
		image:       p.Image,
		network:     sandbox.NetworkMode(p.Network),
		mounts:      parsedMounts,
		maxMemoryMB: maxMemMB,
		maxCPUs:     p.CPUs,
		maxPIDs:     p.PIDs,
		timeout:     parsedTimeout,
		env:         envStrings,
		cwd:         p.Cwd,
		runtime:     p.Runtime,
	}, nil
}

// runScript executes a child Starlark script under sandbox isolation with strict non-escalation guarantees.
// Usage: sandbox.run_script(path, config=None, vars=None)
func (m *Module) runScript(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path   string         `name:"path" position:"0" required:"true"`
		Config starlark.Value `name:"config"`
		Vars   starlark.Value `name:"vars"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	scriptPath := p.Path
	if !filepath.IsAbs(scriptPath) {
		if abs, err := filepath.Abs(scriptPath); err == nil {
			scriptPath = abs
		}
	}

	var box *Sandbox
	if p.Config != nil && p.Config != starlark.None {
		if sb, ok := p.Config.(*Sandbox); ok {
			box = sb
		} else {
			return nil, fmt.Errorf("run_script: expected sandbox.Sandbox for config, got %s", p.Config.Type())
		}
	} else {
		// Default sandbox
		drv, err := sandbox.Resolve(sandbox.DriverDefault)
		if err != nil {
			return nil, err
		}
		box = &Sandbox{
			driverName: drv.Name(),
			driver:     drv,
			network:    sandbox.NetworkNone,
		}
	}

	// Enforce non-escalation: If parent environment is already inside a sandbox,
	// ensure requested sub-sandbox permissions do not exceed parent.
	if os.Getenv(sandbox.InsideEnvVar) == "1" {
		// Child cannot elevate network if parent is restricted
		if box.network == sandbox.NetworkHost {
			box.network = sandbox.NetworkNone
		}
	}

	// Prepare command arguments to execute kite with sub-script
	kiteBin, err := os.Executable()
	if err != nil {
		kiteBin = "kite"
	}

	cmdArgs := []string{kiteBin, "run", scriptPath}

	// Append script vars
	if p.Vars != nil && p.Vars != starlark.None {
		if dict, ok := p.Vars.(*starlark.Dict); ok {
			for _, item := range dict.Items() {
				k, _ := starlark.AsString(item[0])
				v, _ := starlark.AsString(item[1])
				cmdArgs = append(cmdArgs, fmt.Sprintf("--var=%s=%s", k, v))
			}
		}
	}

	// Include script parent directory in mounts if not already mounted
	scriptDir := filepath.Dir(scriptPath)
	hasMount := false
	for _, mnt := range box.mounts {
		if mnt.Destination == scriptDir || mnt.Source == scriptDir {
			hasMount = true
			break
		}
	}
	if !hasMount {
		box.mounts = append(box.mounts, sandbox.Mount{
			Source:      scriptDir,
			Destination: scriptDir,
			Type:        sandbox.MountBind,
			Mode:        sandbox.MountRO,
		})
	}

	spec := &sandbox.ExecutionSpec{
		Command:     cmdArgs,
		Cwd:         scriptDir,
		Env:         append(box.env, fmt.Sprintf("%s=1", sandbox.InsideEnvVar)),
		Network:     box.network,
		Mounts:      box.mounts,
		MaxMemoryMB: box.maxMemoryMB,
		MaxCPUs:     box.maxCPUs,
		MaxPIDs:     box.maxPIDs,
		Timeout:     box.timeout,
		Image:       box.image,
		Runtime:     box.runtime,
	}

	ctx := context.Background()
	if rt := libkite.GetRuntime(thread); rt != nil {
		ctx = rt.Context()
	}

	res, execErr := box.driver.Exec(ctx, spec)
	if execErr != nil {
		if res != nil {
			return NewBoxExecResult(res.Stdout, res.Stderr, res.ExitCode, execErr.Error(), res.Duration, res.KilledByOOM, res.TimedOut), nil
		}
		return NewBoxExecResult("", "", -1, execErr.Error(), 0, false, false), nil
	}

	return NewBoxExecResult(res.Stdout, res.Stderr, res.ExitCode, "", res.Duration, res.KilledByOOM, res.TimedOut), nil
}

// listDrivers returns all registered driver names.
func (m *Module) listDrivers(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	drivers := sandbox.List()
	elems := make([]starlark.Value, len(drivers))
	for i, d := range drivers {
		elems[i] = starlark.String(d)
	}
	return starlark.NewList(elems), nil
}

// defaultDriver returns the resolved name of the default driver on this platform.
func (m *Module) defaultDriver(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	drv, err := sandbox.Resolve(sandbox.DriverDefault)
	if err != nil {
		return nil, err
	}
	return starlark.String(drv.Name()), nil
}

func parseMounts(v starlark.Value) ([]sandbox.Mount, error) {
	var result []sandbox.Mount

	switch list := v.(type) {
	case *starlark.List:
		for i := 0; i < list.Len(); i++ {
			elem := list.Index(i)
			switch m := elem.(type) {
			case *starlark.Dict:
				var src, dst, modeStr, typeStr string
				if sVal, found, _ := m.Get(starlark.String("source")); found {
					src, _ = starlark.AsString(sVal)
				}
				if dVal, found, _ := m.Get(starlark.String("dest")); found {
					dst, _ = starlark.AsString(dVal)
				} else if dVal, found, _ := m.Get(starlark.String("destination")); found {
					dst, _ = starlark.AsString(dVal)
				}
				if mVal, found, _ := m.Get(starlark.String("mode")); found {
					modeStr, _ = starlark.AsString(mVal)
				}
				if tVal, found, _ := m.Get(starlark.String("type")); found {
					typeStr, _ = starlark.AsString(tVal)
				}

				if dst == "" {
					dst = src
				}
				mode := sandbox.MountRO
				if modeStr == "rw" || modeStr == "read-write" {
					mode = sandbox.MountRW
				}
				mountType := sandbox.MountBind
				if typeStr == "tmpfs" {
					mountType = sandbox.MountTmpfs
				}

				result = append(result, sandbox.Mount{
					Source:      src,
					Destination: dst,
					Type:        mountType,
					Mode:        mode,
				})

			case starlark.String:
				parts := strings.Split(string(m), ":")
				if len(parts) >= 2 {
					mode := sandbox.MountRO
					if len(parts) >= 3 && (parts[2] == "rw" || parts[2] == "read-write") {
						mode = sandbox.MountRW
					}
					result = append(result, sandbox.Mount{
						Source:      parts[0],
						Destination: parts[1],
						Type:        sandbox.MountBind,
						Mode:        mode,
					})
				}
			}
		}
	}

	return result, nil
}

func parseMemory(v starlark.Value) (int64, error) {
	if i, err := starlark.AsInt32(v); err == nil {
		return int64(i), nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return 0, fmt.Errorf("expected string or integer memory limit")
	}

	s = strings.TrimSpace(strings.ToUpper(s))
	if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M") {
		num := strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
		val, err := strconv.ParseInt(strings.TrimSpace(num), 10, 64)
		return val, err
	}
	if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G") {
		num := strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
		val, err := strconv.ParseInt(strings.TrimSpace(num), 10, 64)
		return val * 1024, err
	}

	val, err := strconv.ParseInt(s, 10, 64)
	return val, err
}

func parseTimeout(v starlark.Value) (time.Duration, error) {
	if i, err := starlark.AsInt32(v); err == nil {
		return time.Duration(i) * time.Second, nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return 0, fmt.Errorf("expected string or integer timeout")
	}
	return time.ParseDuration(s)
}
