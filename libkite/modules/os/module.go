// Package osmod provides OS operations for starkite.
// Named osmod to avoid conflict with Go's os package.
package osmod

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "os"

func defaultShell() string {
	if runtime.GOOS == "windows" {
		if comspec := os.Getenv("COMSPEC"); comspec != "" {
			return comspec
		}
		return "cmd.exe"
	}
	return "/bin/sh"
}

// Module implements OS operations.
type Module struct {
	once    sync.Once
	module  starlark.Value
	aliases starlark.StringDict
	config  *libkite.ModuleConfig

	// Provider configuration for exec
	shell   string
	env     map[string]string
	workDir string
	timeout time.Duration
	mu      sync.RWMutex
}

func New() *Module {
	return &Module{
		shell:   defaultShell(),
		env:     make(map[string]string),
		timeout: 60 * time.Second,
	}
}

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "os provides operating system operations: env, cwd, exec, which, user info"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			// Environment
			"env":    starlark.NewBuiltin("os.env", m.env_),
			"setenv": starlark.NewBuiltin("os.setenv", m.setenv),

			// Working directory
			"cwd":   starlark.NewBuiltin("os.cwd", m.cwd),
			"chdir": starlark.NewBuiltin("os.chdir", m.chdir),

			// System
			"hostname": starlark.NewBuiltin("os.hostname", m.hostname),

			// Process
			"pid":  starlark.NewBuiltin("os.pid", m.pid),
			"ppid": starlark.NewBuiltin("os.ppid", m.ppid),
			"exit": starlark.NewBuiltin("os.exit", m.exit),

			// Command execution
			"exec":      starlark.NewBuiltin("os.exec", m.execCmd),
			"try_exec":  starlark.NewBuiltin("os.try_exec", m.tryExecCmd),
			"shell":     starlark.NewBuiltin("os.shell", m.shellCmd),
			"try_shell": starlark.NewBuiltin("os.try_shell", m.tryShellCmd),
			"which":     starlark.NewBuiltin("os.which", m.which),

			// User info
			"username": starlark.NewBuiltin("os.username", m.username),
			"userid":   starlark.NewBuiltin("os.userid", m.userid),
			"groupid":  starlark.NewBuiltin("os.groupid", m.groupid),
			"home":     starlark.NewBuiltin("os.home", m.home),
		}

		m.module = libkite.NewTryModule(string(ModuleName), members)

		// Create global aliases
		m.aliases = starlark.StringDict{
			"env":       starlark.NewBuiltin("env", m.env_),
			"setenv":    starlark.NewBuiltin("setenv", m.setenv),
			"cwd":       starlark.NewBuiltin("cwd", m.cwd),
			"chdir":     starlark.NewBuiltin("chdir", m.chdir),
			"hostname":  starlark.NewBuiltin("hostname", m.hostname),
			"pid":       starlark.NewBuiltin("pid", m.pid),
			"ppid":      starlark.NewBuiltin("ppid", m.ppid),
			"exit":      starlark.NewBuiltin("exit", m.exit),
			"exec":      starlark.NewBuiltin("exec", m.execCmd),
			"try_exec":  starlark.NewBuiltin("try_exec", m.tryExecCmd),
			"shell":     starlark.NewBuiltin("shell", m.shellCmd),
			"try_shell": starlark.NewBuiltin("try_shell", m.tryShellCmd),
			"which":     starlark.NewBuiltin("which", m.which),
			"username":  starlark.NewBuiltin("username", m.username),
			"userid":    starlark.NewBuiltin("userid", m.userid),
			"groupid":   starlark.NewBuiltin("groupid", m.groupid),
			"home":      starlark.NewBuiltin("home", m.home),
			// User alias struct
			"user": m.createUserAlias(),
		}
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return m.aliases
}

func (m *Module) FactoryMethod() string { return "" }

// createUserAlias creates a struct with user-related functions.
func (m *Module) createUserAlias() starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("user"), starlark.StringDict{
		"name": starlark.NewBuiltin("user.name", m.username),
		"id":   starlark.NewBuiltin("user.id", m.userid),
		"gid":  starlark.NewBuiltin("user.gid", m.groupid),
		"home": starlark.NewBuiltin("user.home", m.home),
	})
}

// env_ returns the value of an environment variable.
// Named env_ to avoid conflict with the env field.
func (m *Module) env_(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string `name:"name" position:"0" required:"true"`
		Default string `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "os", "env", "env", p.Name); err != nil {
		return nil, err
	}

	if value, ok := os.LookupEnv(p.Name); ok {
		return starlark.String(value), nil
	}
	return starlark.String(p.Default), nil
}

// setenv sets an environment variable.
func (m *Module) setenv(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name  string `name:"name" position:"0" required:"true"`
		Value string `name:"value" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "os", "env", "setenv", p.Name); err != nil {
		return nil, err
	}

	if err := os.Setenv(p.Name, p.Value); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

// cwd returns the current working directory.
func (m *Module) cwd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("cwd takes no arguments")
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return starlark.String(dir), nil
}

// chdir changes the current working directory.
func (m *Module) chdir(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "os", "process", "chdir", p.Path); err != nil {
		return nil, err
	}

	if err := os.Chdir(p.Path); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

// hostname returns the system hostname.
func (m *Module) hostname(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("hostname takes no arguments")
	}

	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	return starlark.String(host), nil
}

// pid returns the current process ID.
func (m *Module) pid(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("pid takes no arguments")
	}
	return starlark.MakeInt(os.Getpid()), nil
}

// ppid returns the parent process ID.
func (m *Module) ppid(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("ppid takes no arguments")
	}
	return starlark.MakeInt(os.Getppid()), nil
}

// exit exits the script with an optional exit code.
func (m *Module) exit(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Code int `name:"code" position:"0"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "os", "process", "exit", fmt.Sprintf("%d", p.Code)); err != nil {
		return nil, err
	}

	os.Exit(p.Code)
	return starlark.None, nil // Never reached
}

// execCmd executes a command.
// cmdResult holds the raw output from a command execution.
type cmdResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error // non-nil for actual failures (timeout, not found); nil for clean exit (even non-zero)
}

// runCmd parses args, checks permissions, and executes a command (either shell-wrapped or direct).
// Returns cmdResult for the caller to interpret, or a Go error for arg parsing failures.
func (m *Module) runCmd(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple, useShell bool) (*cmdResult, error) {
	funcPrefix := "exec"
	if useShell {
		funcPrefix = "shell"
	}

	if len(args) < 1 {
		return nil, fmt.Errorf("%s: expected at least 1 argument (cmd)", funcPrefix)
	}
	cmdStr, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("%s: cmd must be a string, got %s", funcPrefix, args[0].Type())
	}

	var execArgs []string
	if !useShell {
		if len(args) > 2 {
			return nil, fmt.Errorf("exec: expected at most 2 positional arguments (cmd, args), got %d", len(args))
		}
		if len(args) > 1 {
			switch seq := args[1].(type) {
			case *starlark.List:
				for i := 0; i < seq.Len(); i++ {
					val := seq.Index(i)
					s, ok := starlark.AsString(val)
					if !ok {
						return nil, fmt.Errorf("exec: argument at index %d must be a string, got %s", i, val.Type())
					}
					execArgs = append(execArgs, s)
				}
			case starlark.Tuple:
				for i, val := range seq {
					s, ok := starlark.AsString(val)
					if !ok {
						return nil, fmt.Errorf("exec: argument at index %d must be a string, got %s", i, val.Type())
					}
					execArgs = append(execArgs, s)
				}
			default:
				return nil, fmt.Errorf("exec: second argument must be a list or tuple of strings, got %s", seq.Type())
			}
		} else {
			// Fallback: split cmdStr by whitespace for backward compatibility.
			fields := strings.Fields(cmdStr)
			if len(fields) > 0 {
				cmdStr = fields[0]
				execArgs = fields[1:]
			}
		}
	} else {
		// Shell execution
		if len(args) > 1 {
			return nil, fmt.Errorf("shell: expected at most 1 positional argument (cmd)")
		}
	}

	var shellStr starlark.Value = starlark.None
	var envDict starlark.Value = starlark.None
	var cwdStr starlark.Value = starlark.None
	var timeoutStr starlark.Value = starlark.None
	var useridVal starlark.Value = starlark.None
	var groupidVal starlark.Value = starlark.None
	var inputVal starlark.Value = starlark.None
	var outputVal starlark.Value = starlark.None

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "shell":
			shellStr = kv[1]
		case "env":
			envDict = kv[1]
		case "cwd":
			cwdStr = kv[1]
		case "timeout":
			timeoutStr = kv[1]
		case "userid":
			useridVal = kv[1]
		case "groupid":
			groupidVal = kv[1]
		case "input":
			inputVal = kv[1]
		case "output":
			outputVal = kv[1]
		}
	}

	var targetUID uint32
	var targetGID uint32
	var hasUID bool
	var hasGID bool

	if (useridVal != starlark.None || groupidVal != starlark.None) && !supportsUserSwitch {
		return nil, fmt.Errorf("os.%s: userid and groupid execution switching is not supported on this platform", funcPrefix)
	}

	if useridVal != starlark.None {
		hasUID = true
		if s, ok := starlark.AsString(useridVal); ok {
			u, err := user.Lookup(s)
			if err != nil {
				return nil, fmt.Errorf("os.%s: failed to resolve username %q: %w", funcPrefix, s, err)
			}
			var uid int
			if _, err := fmt.Sscanf(u.Uid, "%d", &uid); err != nil {
				return nil, fmt.Errorf("os.%s: invalid system UID %q for user %q: %w", funcPrefix, u.Uid, s, err)
			}
			targetUID = uint32(uid)
		} else if i, ok := useridVal.(starlark.Int); ok {
			uid, ok := i.Int64()
			if !ok || uid < 0 {
				return nil, fmt.Errorf("os.%s: userid cannot be negative or out of range", funcPrefix)
			}
			targetUID = uint32(uid)
		} else {
			return nil, fmt.Errorf("os.%s: userid must be a string or integer, got %s", funcPrefix, useridVal.Type())
		}
	}

	if groupidVal != starlark.None {
		hasGID = true
		if s, ok := starlark.AsString(groupidVal); ok {
			g, err := user.LookupGroup(s)
			if err != nil {
				return nil, fmt.Errorf("os.%s: failed to resolve groupname %q: %w", funcPrefix, s, err)
			}
			var gid int
			if _, err := fmt.Sscanf(g.Gid, "%d", &gid); err != nil {
				return nil, fmt.Errorf("os.%s: invalid system GID %q for group %q: %w", funcPrefix, g.Gid, s, err)
			}
			targetGID = uint32(gid)
		} else if i, ok := groupidVal.(starlark.Int); ok {
			gid, ok := i.Int64()
			if !ok || gid < 0 {
				return nil, fmt.Errorf("os.%s: groupid cannot be negative or out of range", funcPrefix)
			}
			targetGID = uint32(gid)
		} else {
			return nil, fmt.Errorf("os.%s: groupid must be a string or integer, got %s", funcPrefix, groupidVal.Type())
		}
	}

	m.mu.RLock()
	shell := m.shell
	workDir := m.workDir
	timeout := m.timeout
	baseEnv := make(map[string]string, len(m.env))
	maps.Copy(baseEnv, m.env)
	m.mu.RUnlock()

	if s, ok := starlark.AsString(shellStr); ok && s != "" {
		shell = s
	}
	if s, ok := starlark.AsString(cwdStr); ok && s != "" {
		workDir = s
	}

	var permCategory string
	if useShell {
		permCategory = "shell"
	} else {
		permCategory = "exec"
	}

	execTarget := resolveExecTarget(cmdStr, workDir, baseEnv)
	if err := libkite.Check(thread, "os", permCategory, permCategory, execTarget); err != nil {
		return nil, err
	}
	if hasUID || hasGID {
		if err := libkite.Check(thread, "os", permCategory, "switch_identity", execTarget); err != nil {
			return nil, err
		}
	}

	if s, ok := starlark.AsString(timeoutStr); ok && s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, fmt.Errorf("os.%s: invalid timeout %q: %w", funcPrefix, s, err)
		}
		timeout = d
	} else if timeoutStr != starlark.None {
		return nil, fmt.Errorf("os.%s: timeout must be a duration string (e.g. \"60s\"), got %s", funcPrefix, timeoutStr.Type())
	}

	if d, ok := envDict.(*starlark.Dict); ok {
		for _, item := range d.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				if v, ok := starlark.AsString(item[1]); ok {
					baseEnv[k] = v
				}
			}
		}
	}

	var inputReader io.Reader
	if inputVal != starlark.None {
		switch v := inputVal.(type) {
		case starlark.String:
			inputReader = strings.NewReader(string(v))
		case starlark.Bytes:
			inputReader = bytes.NewReader([]byte(v))
		default:
			if sr, ok := inputVal.(libkite.StarlarkReader); ok {
				inputReader = sr.Reader()
			} else {
				return nil, fmt.Errorf("os.%s: input must be a string, bytes, or io.reader, got %s", funcPrefix, inputVal.Type())
			}
		}
	}

	var stdoutWriter io.Writer
	if outputVal != starlark.None {
		if sw, ok := outputVal.(libkite.StarlarkWriter); ok {
			stdoutWriter = sw.Writer()
		} else {
			return nil, fmt.Errorf("os.%s: output must be an io.writer, got %s", funcPrefix, outputVal.Type())
		}
	}

	defer func() {
		if inputReader != nil {
			if rc, ok := inputReader.(io.Closer); ok {
				rc.Close()
			} else if closer, ok := inputVal.(io.Closer); ok {
				closer.Close()
			}
		}
		if stdoutWriter != nil {
			if wc, ok := stdoutWriter.(io.Closer); ok {
				wc.Close()
			} else if closer, ok := outputVal.(io.Closer); ok {
				closer.Close()
			}
		}
	}()

	var cmd *exec.Cmd
	if useShell {
		if runtime.GOOS == "windows" {
			base := strings.ToLower(filepath.Base(shell))
			switch {
			case base == "cmd.exe" || base == "cmd":
				cmd = exec.Command(shell, "/c", cmdStr)
			case base == "powershell.exe" || base == "powershell" || base == "pwsh.exe" || base == "pwsh":
				cmd = exec.Command(shell, "-Command", cmdStr)
			default:
				cmd = exec.Command(shell, "-c", cmdStr)
			}
		} else {
			cmd = exec.Command(shell, "-c", cmdStr)
		}
	} else {
		cmd = exec.Command(cmdStr, execArgs...)
	}

	configureCredential(cmd, targetUID, targetGID, hasUID, hasGID)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Env = os.Environ()
	for k, v := range baseEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	if inputReader != nil {
		cmd.Stdin = inputReader
	}

	var stdout, stderr bytes.Buffer
	if stdoutWriter != nil {
		cmd.Stdout = stdoutWriter
	} else {
		cmd.Stdout = &stdout
	}
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		runErr = fmt.Errorf("command timed out after %v", timeout)
	}

	res := &cmdResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
	}
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			res.exitCode = exitErr.ExitCode()
		} else {
			res.err = runErr
		}
	}
	return res, nil
}

// resolveExecTarget returns the filesystem path that a command's permission
// check should be matched against: the resolved location of the binary the
// command runs. The first shell token is taken as the binary; an absolute or
// path-qualified token is cleaned (relative to workDir), and a bare name is
// looked up on PATH (honoring a PATH override in env). When the binary cannot
// be located, the bare command token is returned so a denial still names
// something meaningful.
func resolveExecTarget(cmdStr, workDir string, env map[string]string) string {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return cmdStr
	}
	bin := fields[0]

	// The command runs in workDir, or the process cwd when workDir is unset.
	// Resolve relative binary paths against that same directory so the
	// permission check matches the location the command actually runs from.
	baseDir := workDir
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}

	// Path-qualified (absolute or contains a separator): clean it, resolving
	// relative paths against the base directory.
	if strings.ContainsRune(bin, filepath.Separator) || filepath.IsAbs(bin) {
		if !filepath.IsAbs(bin) && baseDir != "" {
			bin = filepath.Join(baseDir, bin)
		}
		return filepath.Clean(bin)
	}

	// Bare name: resolve on PATH (prefer an explicit PATH override in env).
	if p := env["PATH"]; p != "" {
		for _, dir := range filepath.SplitList(p) {
			cand := filepath.Join(dir, bin)
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
				return cand
			}
		}
	}
	if resolved, err := exec.LookPath(bin); err == nil {
		return resolved
	}
	return bin
}

// execCmd runs a command and returns the output as a string.
// On non-zero exit, returns a Starlark error. On success with non-empty stderr,
// returns stderr + stdout combined so warnings are not silently lost.
func (m *Module) execCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if m.config != nil && m.config.DryRun {
		if len(args) < 1 {
			return nil, fmt.Errorf("exec: expected at least 1 argument (cmd)")
		}
		cmdStr, _ := starlark.AsString(args[0])
		return starlark.String(fmt.Sprintf("[DRY RUN] Would execute: %s", cmdStr)), nil
	}

	res, err := m.runCmd(thread, args, kwargs, false)
	if err != nil {
		return nil, err
	}
	if res.err != nil {
		return nil, res.err
	}
	if res.exitCode != 0 {
		errMsg := strings.TrimSpace(res.stderr)
		if errMsg == "" {
			errMsg = strings.TrimSpace(res.stdout)
		}
		return nil, fmt.Errorf("command failed (exit code %d): %s", res.exitCode, errMsg)
	}
	if res.stderr != "" {
		return starlark.String(res.stderr + " " + res.stdout), nil
	}
	return starlark.String(res.stdout), nil
}

// tryExecCmd runs a command and returns an ExecResult directly.
// Never raises a Starlark error — all outcomes are captured in the ExecResult.
func (m *Module) tryExecCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if m.config != nil && m.config.DryRun {
		if len(args) < 1 {
			return nil, fmt.Errorf("exec: expected at least 1 argument (cmd)")
		}
		cmdStr, _ := starlark.AsString(args[0])
		return &ExecResult{
			stdout: fmt.Sprintf("[DRY RUN] Would execute: %s", cmdStr),
		}, nil
	}

	res, err := m.runCmd(thread, args, kwargs, false)
	if err != nil {
		return nil, err // arg parsing / permission errors are real errors
	}
	if res.err != nil {
		return &ExecResult{exitCode: -1, errMsg: res.err.Error()}, nil
	}
	return &ExecResult{
		stdout:   res.stdout,
		stderr:   res.stderr,
		exitCode: res.exitCode,
	}, nil
}

// shellCmd executes a command using a shell and returns the output as a string.
func (m *Module) shellCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if m.config != nil && m.config.DryRun {
		if len(args) < 1 {
			return nil, fmt.Errorf("shell: expected at least 1 argument (cmd)")
		}
		cmdStr, _ := starlark.AsString(args[0])
		return starlark.String(fmt.Sprintf("[DRY RUN] Would execute: %s", cmdStr)), nil
	}

	res, err := m.runCmd(thread, args, kwargs, true)
	if err != nil {
		return nil, err
	}
	if res.err != nil {
		return nil, res.err
	}
	if res.exitCode != 0 {
		errMsg := strings.TrimSpace(res.stderr)
		if errMsg == "" {
			errMsg = strings.TrimSpace(res.stdout)
		}
		return nil, fmt.Errorf("command failed (exit code %d): %s", res.exitCode, errMsg)
	}
	if res.stderr != "" {
		return starlark.String(res.stderr + " " + res.stdout), nil
	}
	return starlark.String(res.stdout), nil
}

// tryShellCmd executes a command using a shell and returns an ExecResult directly.
func (m *Module) tryShellCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if m.config != nil && m.config.DryRun {
		if len(args) < 1 {
			return nil, fmt.Errorf("shell: expected at least 1 argument (cmd)")
		}
		cmdStr, _ := starlark.AsString(args[0])
		return &ExecResult{
			stdout: fmt.Sprintf("[DRY RUN] Would execute: %s", cmdStr),
		}, nil
	}

	res, err := m.runCmd(thread, args, kwargs, true)
	if err != nil {
		return nil, err // arg parsing / permission errors are real errors
	}
	if res.err != nil {
		return &ExecResult{exitCode: -1, errMsg: res.err.Error()}, nil
	}
	return &ExecResult{
		stdout:   res.stdout,
		stderr:   res.stderr,
		exitCode: res.exitCode,
	}, nil
}

// ExecResult is a Starlark value returned by try_exec with flattened access
// to stdout, stderr, code, ok, and error — no nested .value required.
// Truth() returns ok, so `if result:` works as a success check.
type ExecResult struct {
	stdout   string
	stderr   string
	exitCode int
	errMsg   string // non-empty for actual failures (timeout, not found)
}

var (
	_ starlark.Value    = (*ExecResult)(nil)
	_ starlark.HasAttrs = (*ExecResult)(nil)
)

func (r *ExecResult) isOK() bool { return r.exitCode == 0 && r.errMsg == "" }

func (r *ExecResult) String() string {
	if r.isOK() {
		return fmt.Sprintf("ExecResult(ok=True, code=%d)", r.exitCode)
	}
	return fmt.Sprintf("ExecResult(ok=False, code=%d, error=%q)", r.exitCode, r.errorString())
}

func (r *ExecResult) Type() string         { return "ExecResult" }
func (r *ExecResult) Freeze()              {}
func (r *ExecResult) Truth() starlark.Bool { return starlark.Bool(r.isOK()) }
func (r *ExecResult) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: ExecResult")
}

func (r *ExecResult) errorString() string {
	if r.errMsg != "" {
		return r.errMsg
	}
	if r.exitCode != 0 {
		return strings.TrimSpace(r.stderr)
	}
	return ""
}

func (r *ExecResult) Attr(name string) (starlark.Value, error) {
	switch name {
	case "ok":
		return starlark.Bool(r.isOK()), nil
	case "stdout":
		return starlark.String(r.stdout), nil
	case "stderr":
		return starlark.String(r.stderr), nil
	case "code":
		return starlark.MakeInt(r.exitCode), nil
	case "error":
		return starlark.String(r.errorString()), nil
	}
	return nil, nil
}

func (r *ExecResult) AttrNames() []string {
	return []string{"code", "error", "ok", "stderr", "stdout"}
}

// which finds an executable in PATH.
func (m *Module) which(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name string `name:"name" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "os", "exec", "which", p.Name); err != nil {
		return nil, err
	}

	path, err := exec.LookPath(p.Name)
	if err != nil {
		return starlark.None, nil
	}
	return starlark.String(path), nil
}

// username returns the current user's username.
func (m *Module) username(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("username takes no arguments")
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return starlark.String(u.Username), nil
}

// userid returns the current user's UID.
func (m *Module) userid(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("userid takes no arguments")
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return starlark.String(u.Uid), nil
}

// groupid returns the current user's primary GID.
func (m *Module) groupid(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("groupid takes no arguments")
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return starlark.String(u.Gid), nil
}

// home returns the current user's home directory.
func (m *Module) home(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("home takes no arguments")
	}

	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	return starlark.String(u.HomeDir), nil
}
