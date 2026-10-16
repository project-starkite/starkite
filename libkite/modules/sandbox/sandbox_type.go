package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
)

// Sandbox is a Starlark value representing a configured sandbox isolation container.
type Sandbox struct {
	driverName       string
	driver           sandbox.Driver
	image            string
	network          sandbox.NetworkMode
	mounts           []sandbox.Mount
	maxMemoryMB      int64
	maxCPUs          float64
	maxPIDs          int64
	timeout          time.Duration
	env              []string
	cwd              string
	runtime          string
	noExec           bool
	allowedExecPaths []string
	frozen           bool
}

var (
	_ starlark.Value    = (*Sandbox)(nil)
	_ starlark.HasAttrs = (*Sandbox)(nil)
)

func (s *Sandbox) String() string {
	return fmt.Sprintf("sandbox.Sandbox(driver=%q, network=%q, image=%q)", s.driverName, s.network, s.image)
}

func (s *Sandbox) Type() string         { return "sandbox.Sandbox" }
func (s *Sandbox) Freeze()              { s.frozen = true }
func (s *Sandbox) Truth() starlark.Bool { return starlark.True }
func (s *Sandbox) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: sandbox.Sandbox")
}

func (s *Sandbox) Attr(name string) (starlark.Value, error) {
	switch name {
	case "driver":
		return starlark.String(s.driverName), nil
	case "image":
		return starlark.String(s.image), nil
	case "network":
		return starlark.String(string(s.network)), nil
	case "memory":
		return starlark.MakeInt64(s.maxMemoryMB), nil
	case "cpus":
		return starlark.Float(s.maxCPUs), nil
	case "timeout":
		return starlark.String(s.timeout.String()), nil
	case "cwd":
		return starlark.String(s.cwd), nil
	case "no_exec":
		return starlark.Bool(s.noExec), nil
	case "allowed_exec_paths":
		elems := make([]starlark.Value, len(s.allowedExecPaths))
		for i, p := range s.allowedExecPaths {
			elems[i] = starlark.String(p)
		}
		return starlark.NewList(elems), nil
	case "exec":
		return starlark.NewBuiltin("Sandbox.exec", s.boxExec), nil
	default:
		return nil, nil
	}
}

func (s *Sandbox) AttrNames() []string {
	names := []string{"allowed_exec_paths", "cpus", "cwd", "driver", "exec", "image", "memory", "network", "no_exec", "timeout"}
	sort.Strings(names)
	return names
}

// boxExec executes a command inside the sandbox jail.
// Usage: box.exec(cmd, args=[], env=None, timeout=None, cwd=None)
func (s *Sandbox) boxExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Cmd     starlark.Value `name:"cmd" position:"0" required:"true"`
		Args    starlark.Value `name:"args" position:"1"`
		Env     starlark.Value `name:"env"`
		Timeout starlark.Value `name:"timeout"`
		Cwd     string         `name:"cwd"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	cmdStr, ok := starlark.AsString(p.Cmd)
	if !ok {
		return nil, fmt.Errorf("exec: expected string for cmd, got %s", p.Cmd.Type())
	}

	var commandList []string
	if p.Args != nil && p.Args != starlark.None {
		commandList = []string{cmdStr}
		switch v := p.Args.(type) {
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				argStr, ok := starlark.AsString(v.Index(i))
				if !ok {
					return nil, fmt.Errorf("exec: args element %d is not a string", i)
				}
				commandList = append(commandList, argStr)
			}
		case starlark.Tuple:
			for i, elem := range v {
				argStr, ok := starlark.AsString(elem)
				if !ok {
					return nil, fmt.Errorf("exec: args element %d is not a string", i)
				}
				commandList = append(commandList, argStr)
			}
		default:
			return nil, fmt.Errorf("exec: args must be list or tuple, got %s", p.Args.Type())
		}
	} else {
		commandList = strings.Fields(cmdStr)
		if len(commandList) == 0 {
			commandList = []string{cmdStr}
		}
	}

	// Environment merging
	envList := append([]string(nil), s.env...)
	if p.Env != nil && p.Env != starlark.None {
		switch v := p.Env.(type) {
		case *starlark.Dict:
			for _, item := range v.Items() {
				k, _ := starlark.AsString(item[0])
				val, _ := starlark.AsString(item[1])
				envList = append(envList, fmt.Sprintf("%s=%s", k, val))
			}
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				e, _ := starlark.AsString(v.Index(i))
				envList = append(envList, e)
			}
		}
	}

	// Timeout resolution
	execTimeout := s.timeout
	if p.Timeout != nil && p.Timeout != starlark.None {
		if durStr, ok := starlark.AsString(p.Timeout); ok {
			if parsed, err := time.ParseDuration(durStr); err == nil {
				execTimeout = parsed
			}
		} else if durSec, err := starlark.AsInt32(p.Timeout); err == nil {
			execTimeout = time.Duration(durSec) * time.Second
		}
	}

	execCwd := s.cwd
	if p.Cwd != "" {
		execCwd = p.Cwd
	}

	execTarget := commandList[0]
	if strings.ContainsRune(execTarget, filepath.Separator) || filepath.IsAbs(execTarget) {
		if !filepath.IsAbs(execTarget) && execCwd != "" {
			execTarget = filepath.Join(execCwd, execTarget)
		}
		execTarget = libkite.CanonicalizePath(execTarget)
	}
	if err := checkSandboxPermissions(thread, "exec", execTarget, s.network, s.mounts); err != nil {
		return nil, err
	}

	netMode := s.network
	if os.Getenv(sandbox.InsideEnvVar) == "1" && netMode == sandbox.NetworkHost {
		netMode = sandbox.NetworkNone
	}

	spec := &sandbox.ExecutionSpec{
		Command:          commandList,
		Cwd:              execCwd,
		Env:              envList,
		Network:          netMode,
		Mounts:           s.mounts,
		MaxMemoryMB:      s.maxMemoryMB,
		MaxCPUs:          s.maxCPUs,
		MaxPIDs:          s.maxPIDs,
		Timeout:          execTimeout,
		Image:            s.image,
		Runtime:          s.runtime,
		NoExec:           s.noExec,
		AllowedExecPaths: s.allowedExecPaths,
	}

	ctx := context.Background()
	if rt := libkite.GetRuntime(thread); rt != nil {
		ctx = rt.Context()
	}

	res, err := s.driver.Exec(ctx, spec)
	if err != nil {
		if res != nil {
			return NewBoxExecResult(res.Stdout, res.Stderr, res.ExitCode, err.Error(), res.Duration, res.KilledByOOM, res.TimedOut), nil
		}
		return NewBoxExecResult("", "", -1, err.Error(), 0, false, false), nil
	}

	return NewBoxExecResult(res.Stdout, res.Stderr, res.ExitCode, "", res.Duration, res.KilledByOOM, res.TimedOut), nil
}
