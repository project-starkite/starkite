package ssh

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/crypto/ssh"

	"github.com/project-starkite/starkite/libkite"
	kiteexec "github.com/project-starkite/starkite/libkite/exec"
)

func newSSHResult(host, cmd, stdout, stderr string, code int, ok, dryRun bool) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("SSHResult"), starlark.StringDict{
		"host":    starlark.String(host),
		"cmd":     starlark.String(cmd),
		"stdout":  starlark.String(stdout),
		"stderr":  starlark.String(stderr),
		"code":    starlark.MakeInt(code),
		"ok":      starlark.Bool(ok),
		"dry_run": starlark.Bool(dryRun),
	})
}

func newSSHBatchResult(host string, ok, stoppedEarly, dryRun bool, steps []starlark.Value) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("SSHBatchResult"), starlark.StringDict{
		"host":          starlark.String(host),
		"ok":            starlark.Bool(ok),
		"stopped_early": starlark.Bool(stoppedEarly),
		"dry_run":       starlark.Bool(dryRun),
		"steps":         starlark.NewList(steps),
	})
}

func buildSSHCommand(cmd string, cwd string, env map[string]string, sudo bool, asUser string) string {
	finalCmd := cmd
	if cwd != "" {
		finalCmd = fmt.Sprintf("cd %s && %s", cwd, cmd)
	}
	if len(env) > 0 {
		var envParts []string
		for k, v := range env {
			envParts = append(envParts, fmt.Sprintf("%s=%q", k, v))
		}
		finalCmd = strings.Join(envParts, " ") + " " + finalCmd
	}
	if sudo {
		if asUser != "" {
			finalCmd = fmt.Sprintf("sudo -u %s %s", asUser, finalCmd)
		} else {
			finalCmd = "sudo " + finalCmd
		}
	}
	return finalCmd
}

// exec executes a single command or a multi-command pipeline on remote hosts.
func (c *SSHClient) exec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Extract env dict and commands manually
	var envDict *starlark.Dict
	var rawCommands starlark.Value
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "env":
			if d, ok := kv[1].(*starlark.Dict); ok {
				envDict = d
			}
		case "commands":
			rawCommands = kv[1]
		default:
			filteredKwargs = append(filteredKwargs, kv)
		}
	}

	var positionalCommands bool
	// Check if positional argument is a command list: client.exec(["cmd1", "cmd2"])
	if len(args) == 1 && rawCommands == nil {
		switch seq := args[0].(type) {
		case *starlark.List:
			rawCommands = seq
			positionalCommands = true
		case starlark.Tuple:
			rawCommands = seq
			positionalCommands = true
		}
	}

	// 1. Multi-Command Pipeline Execution
	if rawCommands != nil && rawCommands != starlark.None {
		var commands []string
		switch seq := rawCommands.(type) {
		case *starlark.List:
			for i := 0; i < seq.Len(); i++ {
				if s, ok := starlark.AsString(seq.Index(i)); ok {
					commands = append(commands, s)
				} else {
					return nil, fmt.Errorf("ssh.exec: commands item %d must be a string, got %s", i, seq.Index(i).Type())
				}
			}
		case starlark.Tuple:
			for i, val := range seq {
				if s, ok := starlark.AsString(val); ok {
					commands = append(commands, s)
				} else {
					return nil, fmt.Errorf("ssh.exec: commands item %d must be a string, got %s", i, val.Type())
				}
			}
		default:
			return nil, fmt.Errorf("ssh.exec: commands must be a list of strings, got %s", rawCommands.Type())
		}

		var p struct {
			Sudo           bool   `name:"sudo"`
			AsUser         string `name:"as_user"`
			Cwd            string `name:"cwd"`
			ExecMaxWorkers int    `name:"exec_max_workers"`
			MaxWorkers     int    `name:"max_workers"`
			ExecOnError    string `name:"exec_on_error"`
		}
		// If positional args was the commands list, don't pass it into startype
		remainingArgs := args
		if positionalCommands {
			remainingArgs = nil
		}
		if err := startype.Args(remainingArgs, filteredKwargs).Go(&p); err != nil {
			return nil, err
		}

		maxWorkers := c.execMaxWorkers
		if p.ExecMaxWorkers > 0 {
			maxWorkers = p.ExecMaxWorkers
		} else if p.MaxWorkers > 0 {
			maxWorkers = p.MaxWorkers
		}

		execOnError := c.execOnError
		if p.ExecOnError != "" {
			if p.ExecOnError != "stop" && p.ExecOnError != "continue" {
				return nil, fmt.Errorf("ssh.exec: invalid exec_on_error %q (must be \"stop\" or \"continue\")", p.ExecOnError)
			}
			execOnError = p.ExecOnError
		}

		// Permission checks for all commands on each host
		for _, host := range c.hosts {
			for _, cmd := range commands {
				if err := libkite.Check(thread, "ssh", "connect", "exec", host+":"+cmd); err != nil {
					return nil, err
				}
			}
		}

		sudo := p.Sudo || c.defaultSudo
		asUser := p.AsUser
		if asUser == "" {
			asUser = c.defaultAsUser
		}
		cwd := p.Cwd
		if cwd == "" {
			cwd = c.defaultCwd
		}

		env := make(map[string]string)
		maps.Copy(env, c.defaultEnv)
		if envDict != nil {
			for _, item := range envDict.Items() {
				if k, ok := starlark.AsString(item[0]); ok {
					if v, ok := starlark.AsString(item[1]); ok {
						env[k] = v
					}
				}
			}
		}

		if c.dryRun {
			return c.dryRunExecBatchResults(commands, sudo, asUser, cwd, env), nil
		}

		if len(c.hosts) == 0 {
			return starlark.NewList(nil), nil
		}

		if c.execPolicy == "concurrent" {
			return c.execBatchConcurrent(commands, sudo, asUser, cwd, env, execOnError, maxWorkers)
		}
		return c.execBatchLinear(commands, sudo, asUser, cwd, env, execOnError)
	}

	// 2. Single Command Execution
	var positionalArgs []string
	filteredArgs := args
	if len(args) > 1 {
		switch seq := args[1].(type) {
		case *starlark.List:
			for i := 0; i < seq.Len(); i++ {
				if s, ok := starlark.AsString(seq.Index(i)); ok {
					positionalArgs = append(positionalArgs, s)
				} else {
					return nil, fmt.Errorf("ssh.exec: argument at index %d must be a string, got %s", i, seq.Index(i).Type())
				}
			}
			filteredArgs = starlark.Tuple{args[0]}
		case starlark.Tuple:
			for i, val := range seq {
				if s, ok := starlark.AsString(val); ok {
					positionalArgs = append(positionalArgs, s)
				} else {
					return nil, fmt.Errorf("ssh.exec: argument at index %d must be a string, got %s", i, val.Type())
				}
			}
			filteredArgs = starlark.Tuple{args[0]}
		}
	}

	var p struct {
		Cmd            string `name:"cmd" position:"0" required:"true"`
		Sudo           bool   `name:"sudo"`
		AsUser         string `name:"as_user"`
		Cwd            string `name:"cwd"`
		ExecMaxWorkers int    `name:"exec_max_workers"`
		MaxWorkers     int    `name:"max_workers"`
	}
	if err := startype.Args(filteredArgs, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	maxWorkers := c.execMaxWorkers
	if p.ExecMaxWorkers > 0 {
		maxWorkers = p.ExecMaxWorkers
	} else if p.MaxWorkers > 0 {
		maxWorkers = p.MaxWorkers
	}

	cmdToRun := p.Cmd
	if len(positionalArgs) > 0 {
		cmdToRun = kiteexec.Join(p.Cmd, positionalArgs)
	}

	// Permission check for SSH exec - check each host
	for _, host := range c.hosts {
		if err := libkite.Check(thread, "ssh", "connect", "exec", host+":"+p.Cmd); err != nil {
			return nil, err
		}
	}

	// Apply defaults
	sudo := p.Sudo || c.defaultSudo
	asUser := p.AsUser
	if asUser == "" {
		asUser = c.defaultAsUser
	}
	cwd := p.Cwd
	if cwd == "" {
		cwd = c.defaultCwd
	}

	// Build environment
	env := make(map[string]string)
	maps.Copy(env, c.defaultEnv)
	if envDict != nil {
		for _, item := range envDict.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				if v, ok := starlark.AsString(item[1]); ok {
					env[k] = v
				}
			}
		}
	}

	// Build final command
	finalCmd := buildSSHCommand(cmdToRun, cwd, env, sudo, asUser)

	if c.dryRun {
		return c.dryRunExecResults(finalCmd), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	// Execute on all hosts
	if c.execPolicy == "concurrent" {
		return c.execConcurrent(finalCmd, maxWorkers)
	}
	return c.execLinear(finalCmd)
}

func (c *SSHClient) dryRunExecResults(cmd string) starlark.Value {
	results := make([]starlark.Value, len(c.hosts))
	for i, host := range c.hosts {
		results[i] = newSSHResult(host, cmd, fmt.Sprintf("[DRY RUN] Would execute on %s: %s", host, cmd), "", 0, true, true)
	}
	return starlark.NewList(results)
}

func (c *SSHClient) dryRunExecBatchResults(commands []string, sudo bool, asUser, cwd string, env map[string]string) starlark.Value {
	results := make([]starlark.Value, len(c.hosts))
	for i, host := range c.hosts {
		steps := make([]starlark.Value, len(commands))
		for k, cmd := range commands {
			finalCmd := buildSSHCommand(cmd, cwd, env, sudo, asUser)
			steps[k] = newSSHResult(host, cmd, fmt.Sprintf("[DRY RUN] Would execute on %s: %s", host, finalCmd), "", 0, true, true)
		}
		results[i] = newSSHBatchResult(host, true, false, true, steps)
	}
	return starlark.NewList(results)
}

func (c *SSHClient) execConcurrent(cmd string, maxWorkers ...int) (starlark.Value, error) {
	workers := c.execMaxWorkers
	if len(maxWorkers) > 0 && maxWorkers[0] > 0 {
		workers = maxWorkers[0]
	}

	results := make([]starlark.Value, len(c.hosts))
	errors := make([]error, len(c.hosts))
	var wg sync.WaitGroup

	var sem chan struct{}
	if workers > 0 {
		sem = make(chan struct{}, workers)
	}

	for i, host := range c.hosts {
		wg.Add(1)
		if sem != nil {
			sem <- struct{}{}
		}
		go func(idx int, h string) {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			result, err := c.execOnHost(h, cmd)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = result
		}(i, host)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", c.hosts[i], err)
		}
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) execLinear(cmd string) (starlark.Value, error) {
	results := make([]starlark.Value, 0, len(c.hosts))

	for _, host := range c.hosts {
		result, err := c.execOnHost(host, cmd)
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", host, err)
		}
		results = append(results, result)
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) execOnHost(host, cmd string) (starlark.Value, error) {
	client, err := c.dialHostWithRetry(host)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	err = session.Run(cmd)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return nil, err
		}
	}

	return newSSHResult(host, cmd, stdout.String(), stderr.String(), exitCode, exitCode == 0, false), nil
}

func (c *SSHClient) execBatchConcurrent(commands []string, sudo bool, asUser, cwd string, env map[string]string, execOnError string, maxWorkers int) (starlark.Value, error) {
	results := make([]starlark.Value, len(c.hosts))
	errors := make([]error, len(c.hosts))
	var wg sync.WaitGroup

	var sem chan struct{}
	if maxWorkers > 0 {
		sem = make(chan struct{}, maxWorkers)
	}

	for i, host := range c.hosts {
		wg.Add(1)
		if sem != nil {
			sem <- struct{}{}
		}
		go func(idx int, h string) {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			result, err := c.execBatchOnHost(h, commands, sudo, asUser, cwd, env, execOnError)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = result
		}(i, host)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", c.hosts[i], err)
		}
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) execBatchLinear(commands []string, sudo bool, asUser, cwd string, env map[string]string, execOnError string) (starlark.Value, error) {
	results := make([]starlark.Value, 0, len(c.hosts))

	for _, host := range c.hosts {
		result, err := c.execBatchOnHost(host, commands, sudo, asUser, cwd, env, execOnError)
		if err != nil {
			return nil, fmt.Errorf("host %s: %w", host, err)
		}
		results = append(results, result)
	}

	return starlark.NewList(results), nil
}

func (c *SSHClient) execBatchOnHost(host string, commands []string, sudo bool, asUser, cwd string, env map[string]string, execOnError string) (starlark.Value, error) {
	steps := make([]starlark.Value, 0, len(commands))
	allOK := true
	stoppedEarly := false

	client, err := c.dialHostWithRetry(host)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	for _, cmd := range commands {
		finalCmd := buildSSHCommand(cmd, cwd, env, sudo, asUser)

		session, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr

		runErr := session.Run(finalCmd)
		session.Close()

		exitCode := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return nil, runErr
			}
		}

		stepOk := exitCode == 0
		steps = append(steps, newSSHResult(host, cmd, stdout.String(), stderr.String(), exitCode, stepOk, false))

		if !stepOk {
			allOK = false
			if execOnError == "stop" {
				stoppedEarly = true
				break
			}
		}
	}

	return newSSHBatchResult(host, allOK, stoppedEarly, false, steps), nil
}
