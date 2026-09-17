package osmod

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.starlark.net/starlark"
)

// Shell represents a configured shell execution object.
type Shell struct {
	module     *Module
	command    string
	flag       string
	cwd        string
	env        map[string]string
	timeout    time.Duration
	timeoutStr string
	useridVal  starlark.Value
	groupidVal starlark.Value
}

var (
	_ starlark.Value    = (*Shell)(nil)
	_ starlark.HasAttrs = (*Shell)(nil)
)

func defaultFlagForCommand(cmd string) string {
	normalized := strings.ReplaceAll(cmd, "\\", "/")
	base := strings.ToLower(filepath.Base(normalized))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "pwsh", "powershell":
		return "-Command"
	case "cmd":
		return "/c"
	default:
		return "-c"
	}
}

func (s *Shell) String() string {
	return fmt.Sprintf("Shell(command=%q, flag=%q, cwd=%q, timeout=%q)", s.command, s.flag, s.cwd, s.timeoutStr)
}

func (s *Shell) Type() string          { return "Shell" }
func (s *Shell) Freeze()               {} // Shell is immutable
func (s *Shell) Truth() starlark.Bool  { return starlark.True }
func (s *Shell) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: Shell") }

func (s *Shell) Attr(name string) (starlark.Value, error) {
	switch name {
	case "command":
		return starlark.String(s.command), nil
	case "flag":
		return starlark.String(s.flag), nil
	case "cwd":
		return starlark.String(s.cwd), nil
	case "timeout":
		return starlark.String(s.timeoutStr), nil
	case "exec":
		return starlark.NewBuiltin("Shell.exec", s.exec), nil
	case "try_exec":
		return starlark.NewBuiltin("Shell.try_exec", s.tryExec), nil
	}
	return nil, nil
}

func (s *Shell) AttrNames() []string {
	return []string{"command", "cwd", "exec", "flag", "timeout", "try_exec"}
}

func (s *Shell) exec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, fmt.Errorf("Shell.exec: not implemented yet")
}

func (s *Shell) tryExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return nil, fmt.Errorf("Shell.try_exec: not implemented yet")
}

// shell constructs a new Shell execution object.
func (m *Module) shell(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		commandVal starlark.Value = starlark.None
		flagVal    starlark.Value = starlark.None
		envVal     starlark.Value = starlark.None
		cwdVal     starlark.Value = starlark.None
		timeoutVal starlark.Value = starlark.None
		useridVal  starlark.Value = starlark.None
		groupidVal starlark.Value = starlark.None
	)

	if len(args) > 1 {
		return nil, fmt.Errorf("os.shell: expected at most 1 positional argument (command), got %d", len(args))
	}
	if len(args) == 1 {
		commandVal = args[0]
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "command":
			if len(args) == 1 {
				return nil, fmt.Errorf("os.shell: got multiple values for keyword argument 'command'")
			}
			commandVal = kv[1]
		case "flag":
			flagVal = kv[1]
		case "env":
			envVal = kv[1]
		case "cwd":
			cwdVal = kv[1]
		case "timeout":
			timeoutVal = kv[1]
		case "userid":
			useridVal = kv[1]
		case "groupid":
			groupidVal = kv[1]
		default:
			return nil, fmt.Errorf("os.shell: unexpected keyword argument %q", key)
		}
	}

	var command, flag string

	if commandVal != starlark.None {
		s, ok := starlark.AsString(commandVal)
		if !ok {
			return nil, fmt.Errorf("os.shell: command must be a string, got %s", commandVal.Type())
		}
		command = s
	}

	if flagVal != starlark.None {
		s, ok := starlark.AsString(flagVal)
		if !ok {
			return nil, fmt.Errorf("os.shell: flag must be a string, got %s", flagVal.Type())
		}
		flag = s
	}

	if command == "" {
		defCmd, defFlag := defaultShell()
		command = defCmd
		if flag == "" {
			flag = defFlag
		}
	} else if flag == "" {
		flag = defaultFlagForCommand(command)
	}

	var cwd string
	if cwdVal != starlark.None {
		s, ok := starlark.AsString(cwdVal)
		if !ok {
			return nil, fmt.Errorf("os.shell: cwd must be a string, got %s", cwdVal.Type())
		}
		cwd = s
	}

	envMap := make(map[string]string)
	if envVal != starlark.None {
		d, ok := envVal.(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("os.shell: env must be a dict, got %s", envVal.Type())
		}
		for _, item := range d.Items() {
			k, ok1 := starlark.AsString(item[0])
			v, ok2 := starlark.AsString(item[1])
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("os.shell: env keys and values must be strings")
			}
			envMap[k] = v
		}
	}

	timeout := 60 * time.Second
	timeoutStr := "60s"
	if timeoutVal != starlark.None {
		s, ok := starlark.AsString(timeoutVal)
		if !ok {
			return nil, fmt.Errorf("os.shell: timeout must be a duration string, got %s", timeoutVal.Type())
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("os.shell: invalid timeout %q: %w", s, err)
		}
		timeout = d
		timeoutStr = s
	}

	if (useridVal != starlark.None || groupidVal != starlark.None) && !supportsUserSwitch {
		return nil, fmt.Errorf("os.shell: userid and groupid execution switching is not supported on this platform")
	}

	if useridVal != starlark.None {
		if _, ok := starlark.AsString(useridVal); !ok {
			if _, ok := useridVal.(starlark.Int); !ok {
				return nil, fmt.Errorf("os.shell: userid must be a string or integer, got %s", useridVal.Type())
			}
		}
	}

	if groupidVal != starlark.None {
		if _, ok := starlark.AsString(groupidVal); !ok {
			if _, ok := groupidVal.(starlark.Int); !ok {
				return nil, fmt.Errorf("os.shell: groupid must be a string or integer, got %s", groupidVal.Type())
			}
		}
	}

	return &Shell{
		module:     m,
		command:    command,
		flag:       flag,
		cwd:        cwd,
		env:        envMap,
		timeout:    timeout,
		timeoutStr: timeoutStr,
		useridVal:  useridVal,
		groupidVal: groupidVal,
	}, nil
}
