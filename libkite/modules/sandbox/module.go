package sandbox

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

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
		m.module = libkite.NewTryModule(
			string(ModuleName),
			starlark.StringDict{
				"config":         starlark.NewBuiltin("sandbox.config", m.sandboxConfig),
				"run_script":     starlark.NewBuiltin("sandbox.run_script", m.runScript),
				"list_drivers":   starlark.NewBuiltin("sandbox.list_drivers", m.listDrivers),
				"default_driver": starlark.NewBuiltin("sandbox.default_driver", m.defaultDriver),
			},
		)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "config" }

// sandboxConfig creates a new configured Sandbox instance.
// Usage: sandbox.config(driver="default", image="", network="none", mounts=[], memory="512MB", cpus=2.0, pids=100, timeout="30s", env=[], cwd="", runtime="", no_exec=False, allowed_exec_paths=[])
func (m *Module) sandboxConfig(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Driver           string         `name:"driver"`
		Image            string         `name:"image"`
		Network          string         `name:"network"`
		Mounts           starlark.Value `name:"mounts"`
		Memory           starlark.Value `name:"memory"`
		CPUs             float64        `name:"cpus"`
		PIDs             int64          `name:"pids"`
		Timeout          starlark.Value `name:"timeout"`
		Env              starlark.Value `name:"env"`
		Cwd              string         `name:"cwd"`
		Runtime          string         `name:"runtime"`
		NoExec           bool           `name:"no_exec"`
		AllowedExecPaths starlark.Value `name:"allowed_exec_paths"`
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

	if p.Network == "" {
		p.Network = string(sandbox.NetworkNone)
	}
	switch sandbox.NetworkMode(p.Network) {
	case sandbox.NetworkNone, sandbox.NetworkLoopback, sandbox.NetworkHost:
	default:
		return nil, fmt.Errorf("sandbox.config: invalid network mode %q (want none, loopback, or host)", p.Network)
	}

	netMode := sandbox.NetworkMode(p.Network)
	if os.Getenv(sandbox.InsideEnvVar) == "1" {
		if netMode == sandbox.NetworkHost {
			if err := libkite.Check(thread, "http", "client", "config", "*"); err != nil {
				netMode = sandbox.NetworkNone
			}
		}
	}

	// Parse environment
	var envStrings []string
	if p.Env != nil && p.Env != starlark.None {
		parsedEnv, err := parseEnvValue(p.Env, "sandbox.config")
		if err != nil {
			return nil, err
		}
		envStrings = parsedEnv
	}

	// Parse allowed_exec_paths
	var allowedExecPaths []string
	if p.AllowedExecPaths != nil && p.AllowedExecPaths != starlark.None {
		switch v := p.AllowedExecPaths.(type) {
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				s, ok := starlark.AsString(v.Index(i))
				if !ok {
					return nil, fmt.Errorf("sandbox.config: allowed_exec_paths element %d is not a string", i)
				}
				allowedExecPaths = append(allowedExecPaths, s)
			}
		case starlark.Tuple:
			for i, elem := range v {
				s, ok := starlark.AsString(elem)
				if !ok {
					return nil, fmt.Errorf("sandbox.config: allowed_exec_paths element %d is not a string", i)
				}
				allowedExecPaths = append(allowedExecPaths, s)
			}
		default:
			return nil, fmt.Errorf("sandbox.config: allowed_exec_paths must be list or tuple, got %s", p.AllowedExecPaths.Type())
		}
	}

	if p.Cwd != "" {
		canonCwd := libkite.CanonicalizePath(p.Cwd)
		if err := libkite.Check(thread, "fs", "read", "config", canonCwd); err != nil {
			return nil, err
		}
	}

	if err := checkSandboxPermissions(thread, "config", "", netMode, parsedMounts); err != nil {
		return nil, err
	}

	return &Sandbox{
		driverName:       driver.Name(),
		driver:           driver,
		image:            p.Image,
		network:          netMode,
		mounts:           parsedMounts,
		maxMemoryMB:      maxMemMB,
		maxCPUs:          p.CPUs,
		maxPIDs:          p.PIDs,
		timeout:          parsedTimeout,
		env:              envStrings,
		cwd:              p.Cwd,
		runtime:          p.Runtime,
		noExec:           p.NoExec,
		allowedExecPaths: allowedExecPaths,
		moduleCfg:        m.config,
	}, nil
}

func checkSandboxPermissions(thread *starlark.Thread, fnName, target string, network sandbox.NetworkMode, mounts []sandbox.Mount) error {
	if err := libkite.Check(thread, "os", "exec", fnName, target); err != nil {
		return err
	}
	if network == sandbox.NetworkHost || network == sandbox.NetworkLoopback {
		if err := libkite.Check(thread, "http", "client", fnName, ""); err != nil {
			return err
		}
	}
	for _, mnt := range mounts {
		switch mnt.Type {
		case sandbox.MountBind:
			if mnt.Source != "" {
				canonSrc := libkite.CanonicalizePath(mnt.Source)
				if err := libkite.Check(thread, "fs", "read", fnName, canonSrc); err != nil {
					return err
				}
				if mnt.Mode == sandbox.MountRW {
					if err := libkite.Check(thread, "fs", "write", fnName, canonSrc); err != nil {
						return err
					}
				}
			}
		case sandbox.MountTmpfs:
			cleanDst := filepath.ToSlash(filepath.Clean(mnt.Destination))
			if cleanDst != "/tmp" && cleanDst != "/private/tmp" {
				canonDst := libkite.CanonicalizePath(mnt.Destination)
				if err := libkite.Check(thread, "fs", "read", fnName, canonDst); err != nil {
					return err
				}
				if mnt.Mode == sandbox.MountRW {
					if err := libkite.Check(thread, "fs", "write", fnName, canonDst); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
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

	scriptPath := libkite.CanonicalizePath(p.Path)
	if err := libkite.Check(thread, "fs", "read", "run_script", scriptPath); err != nil {
		return nil, err
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

	if err := checkSandboxPermissions(thread, "run_script", scriptPath, box.network, box.mounts); err != nil {
		return nil, err
	}

	netMode := box.network
	// Enforce non-escalation: If parent environment is already inside a sandbox,
	// ensure requested sub-sandbox permissions do not exceed parent.
	if os.Getenv(sandbox.InsideEnvVar) == "1" {
		if netMode == sandbox.NetworkHost {
			netMode = sandbox.NetworkNone
		}
	}

	// Prepare command arguments to execute kite with sub-script
	kiteBin, err := os.Executable()
	if err != nil {
		kiteBin = "kite"
	}

	cmdArgs := []string{kiteBin, "run", scriptPath}
	switch netMode {
	case sandbox.NetworkNone, "":
		cmdArgs = append(cmdArgs, "--sandbox-net-mode=none")
	case sandbox.NetworkLoopback:
		cmdArgs = append(cmdArgs, "--sandbox-net-mode=loopback")
	}

	// Append script vars
	if p.Vars != nil && p.Vars != starlark.None {
		dict, ok, err := libkite.ValueToDict(p.Vars)
		if err != nil {
			return nil, fmt.Errorf("run_script: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("run_script: vars must be dict or struct, got %s", p.Vars.Type())
		}
		for _, item := range dict.Items() {
			k, kOk := starlark.AsString(item[0])
			if !kOk || k == "" {
				return nil, fmt.Errorf("run_script: vars keys must be non-empty strings")
			}
			v, vOk := starlark.AsString(item[1])
			if !vOk {
				return nil, fmt.Errorf("run_script: vars[%q] value must be string, got %s", k, item[1].Type())
			}
			cmdArgs = append(cmdArgs, fmt.Sprintf("--var=%s=%s", k, v))
		}
	}

	// Clone mounts slice to avoid mutating the Sandbox object in place
	mounts := append([]sandbox.Mount(nil), box.mounts...)
	scriptDir := libkite.CanonicalizePath(filepath.Dir(scriptPath))
	if err := libkite.Check(thread, "fs", "read", "run_script", scriptDir); err != nil {
		return nil, err
	}
	hasMount := false
	for _, mnt := range mounts {
		if mnt.Destination == scriptDir || mnt.Source == scriptDir {
			hasMount = true
			break
		}
	}
	if !hasMount {
		mounts = append(mounts, sandbox.Mount{
			Source:      scriptDir,
			Destination: scriptDir,
			Type:        sandbox.MountBind,
			Mode:        sandbox.MountRO,
		})
	}

	spec := &sandbox.ExecutionSpec{
		Command:          cmdArgs,
		Cwd:              scriptDir,
		Env:              append(mergeBaselineEnv(m.config, box.env), fmt.Sprintf("%s=1", sandbox.InsideEnvVar)),
		Network:          netMode,
		Mounts:           mounts,
		MaxMemoryMB:      box.maxMemoryMB,
		MaxCPUs:          box.maxCPUs,
		MaxPIDs:          box.maxPIDs,
		Timeout:          box.timeout,
		Image:            box.image,
		Runtime:          box.runtime,
		NoExec:           box.noExec,
		AllowedExecPaths: box.allowedExecPaths,
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

func parseEnvValue(v starlark.Value, fnName string) ([]string, error) {
	if v == nil || v == starlark.None {
		return nil, nil
	}
	var envStrings []string
	switch val := v.(type) {
	case *starlark.List:
		for i := 0; i < val.Len(); i++ {
			s, ok := starlark.AsString(val.Index(i))
			if !ok {
				return nil, fmt.Errorf("%s: env list element %d is not a string", fnName, i)
			}
			envStrings = append(envStrings, s)
		}
	case starlark.Tuple:
		for i, elem := range val {
			s, ok := starlark.AsString(elem)
			if !ok {
				return nil, fmt.Errorf("%s: env tuple element %d is not a string", fnName, i)
			}
			envStrings = append(envStrings, s)
		}
	default:
		if dict, ok, err := libkite.ValueToDict(v); err != nil {
			return nil, fmt.Errorf("%s: env: %w", fnName, err)
		} else if ok {
			for _, item := range dict.Items() {
				k, kOk := starlark.AsString(item[0])
				if !kOk || k == "" {
					return nil, fmt.Errorf("%s: env keys must be non-empty strings", fnName)
				}
				vStr, vOk := starlark.AsString(item[1])
				if !vOk {
					return nil, fmt.Errorf("%s: env value for %q must be string, got %s", fnName, k, item[1].Type())
				}
				envStrings = append(envStrings, fmt.Sprintf("%s=%s", k, vStr))
			}
			return envStrings, nil
		}
		return nil, fmt.Errorf("%s: env must be list, tuple, dict, or struct, got %s", fnName, v.Type())
	}
	return envStrings, nil
}

func mergeBaselineEnv(cfg *libkite.ModuleConfig, custom []string) []string {
	envMap := sandbox.SanitizedBaselineEnv(os.Environ())
	if cfg != nil && len(cfg.Env) > 0 {
		maps.Copy(envMap, cfg.Env)
	}
	for _, entry := range custom {
		if k, v, ok := strings.Cut(entry, "="); ok && k != "" {
			envMap[k] = v
		}
	}
	res := make([]string, 0, len(envMap))
	for k, v := range envMap {
		res = append(res, fmt.Sprintf("%s=%s", k, v))
	}
	slices.Sort(res)
	return res
}

func parseMountElems(elems []starlark.Value) ([]sandbox.Mount, error) {
	var result []sandbox.Mount
	for idx, elem := range elems {
		if d, ok, err := libkite.ValueToDict(elem); err != nil {
			return nil, fmt.Errorf("mounts[%d]: %w", idx, err)
		} else if ok {
			elem = d
		}
		switch m := elem.(type) {
		case *starlark.Dict:
			var src, dst, modeStr, typeStr string
			if sVal, found, _ := m.Get(starlark.String("source")); found && sVal != starlark.None {
				var ok bool
				src, ok = starlark.AsString(sVal)
				if !ok {
					return nil, fmt.Errorf("mounts[%d].source must be string, got %s", idx, sVal.Type())
				}
			}
			if dVal, found, _ := m.Get(starlark.String("dest")); found && dVal != starlark.None {
				var ok bool
				dst, ok = starlark.AsString(dVal)
				if !ok {
					return nil, fmt.Errorf("mounts[%d].dest must be string, got %s", idx, dVal.Type())
				}
			} else if dVal, found, _ := m.Get(starlark.String("destination")); found && dVal != starlark.None {
				var ok bool
				dst, ok = starlark.AsString(dVal)
				if !ok {
					return nil, fmt.Errorf("mounts[%d].destination must be string, got %s", idx, dVal.Type())
				}
			}
			if mVal, found, _ := m.Get(starlark.String("mode")); found && mVal != starlark.None {
				var ok bool
				modeStr, ok = starlark.AsString(mVal)
				if !ok {
					return nil, fmt.Errorf("mounts[%d].mode must be string, got %s", idx, mVal.Type())
				}
			}
			if tVal, found, _ := m.Get(starlark.String("type")); found && tVal != starlark.None {
				var ok bool
				typeStr, ok = starlark.AsString(tVal)
				if !ok {
					return nil, fmt.Errorf("mounts[%d].type must be string, got %s", idx, tVal.Type())
				}
			}

			mountType := sandbox.MountBind
			switch strings.ToLower(strings.TrimSpace(typeStr)) {
			case "", "bind":
				mountType = sandbox.MountBind
			case "tmpfs":
				mountType = sandbox.MountTmpfs
			default:
				return nil, fmt.Errorf("mounts[%d]: unsupported mount type %q", idx, typeStr)
			}

			if dst == "" && src != "" {
				dst = src
			}
			if dst == "" {
				return nil, fmt.Errorf("mounts[%d]: missing destination", idx)
			}
			if mountType == sandbox.MountBind && src == "" {
				return nil, fmt.Errorf("mounts[%d]: bind mount requires source", idx)
			}

			mode := sandbox.MountRO
			if mountType == sandbox.MountTmpfs && modeStr == "" {
				mode = sandbox.MountRW
			} else {
				switch strings.ToLower(strings.TrimSpace(modeStr)) {
				case "", "ro", "read-only":
					mode = sandbox.MountRO
				case "rw", "read-write":
					mode = sandbox.MountRW
				default:
					return nil, fmt.Errorf("mounts[%d]: unsupported mount mode %q", idx, modeStr)
				}
			}

			result = append(result, sandbox.Mount{
				Source:      src,
				Destination: dst,
				Type:        mountType,
				Mode:        mode,
			})

		case starlark.String:
			raw := strings.TrimSpace(string(m))
			if raw == "" {
				return nil, fmt.Errorf("mounts[%d]: empty mount spec", idx)
			}
			if rest, isTmpfs := strings.CutPrefix(raw, "tmpfs:"); isTmpfs {
				mode := sandbox.MountRW
				dst := rest
				if colon := strings.LastIndex(rest, ":"); colon > 1 {
					candMode := strings.ToLower(strings.TrimSpace(rest[colon+1:]))
					switch candMode {
					case "ro", "read-only":
						mode = sandbox.MountRO
						dst = rest[:colon]
					case "rw", "read-write":
						mode = sandbox.MountRW
						dst = rest[:colon]
					}
				}
				if dst == "" {
					return nil, fmt.Errorf("mounts[%d]: missing tmpfs destination", idx)
				}
				result = append(result, sandbox.Mount{
					Destination: dst,
					Type:        sandbox.MountTmpfs,
					Mode:        mode,
				})
				continue
			}

			parts := strings.Split(raw, ":")
			if len(parts) < 2 {
				return nil, fmt.Errorf("mounts[%d]: invalid mount spec %q (expected source:dest[:mode])", idx, raw)
			}
			mode := sandbox.MountRO
			if len(parts) == 3 {
				switch strings.ToLower(strings.TrimSpace(parts[2])) {
				case "ro", "read-only":
					mode = sandbox.MountRO
				case "rw", "read-write":
					mode = sandbox.MountRW
				default:
					return nil, fmt.Errorf("mounts[%d]: invalid mount mode %q", idx, parts[2])
				}
			} else if len(parts) > 3 {
				return nil, fmt.Errorf("mounts[%d]: invalid mount spec %q", idx, raw)
			}
			if parts[0] == "" || parts[1] == "" {
				return nil, fmt.Errorf("mounts[%d]: invalid mount spec %q", idx, raw)
			}
			result = append(result, sandbox.Mount{
				Source:      parts[0],
				Destination: parts[1],
				Type:        sandbox.MountBind,
				Mode:        mode,
			})

		default:
			return nil, fmt.Errorf("mounts[%d]: expected string or dict, got %s", idx, elem.Type())
		}
	}
	return result, nil
}

func parseMounts(v starlark.Value) ([]sandbox.Mount, error) {
	switch list := v.(type) {
	case *starlark.List:
		elems := make([]starlark.Value, list.Len())
		for i := 0; i < list.Len(); i++ {
			elems[i] = list.Index(i)
		}
		return parseMountElems(elems)
	case starlark.Tuple:
		return parseMountElems([]starlark.Value(list))
	default:
		return nil, fmt.Errorf("expected list or tuple, got %s", v.Type())
	}
}

func parseMemory(v starlark.Value) (int64, error) {
	if i, err := starlark.AsInt32(v); err == nil {
		if i < 0 {
			return 0, fmt.Errorf("memory limit cannot be negative: %d", i)
		}
		return int64(i), nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return 0, fmt.Errorf("expected string or integer memory limit")
	}

	s = strings.TrimSpace(strings.ToUpper(s))
	var val int64
	var err error
	if strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M") {
		num := strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
		val, err = strconv.ParseInt(strings.TrimSpace(num), 10, 64)
	} else if strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G") {
		num := strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
		val, err = strconv.ParseInt(strings.TrimSpace(num), 10, 64)
		val *= 1024
	} else {
		val, err = strconv.ParseInt(s, 10, 64)
	}
	if err != nil {
		return 0, err
	}
	if val < 0 {
		return 0, fmt.Errorf("memory limit cannot be negative: %q", s)
	}
	return val, nil
}

func parseTimeout(v starlark.Value) (time.Duration, error) {
	if i, err := starlark.AsInt32(v); err == nil {
		if i < 0 {
			return 0, fmt.Errorf("timeout cannot be negative: %d", i)
		}
		return time.Duration(i) * time.Second, nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return 0, fmt.Errorf("expected string or integer timeout")
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if dur < 0 {
		return 0, fmt.Errorf("timeout cannot be negative: %v", dur)
	}
	return dur, nil
}
