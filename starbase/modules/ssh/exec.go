package ssh

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/crypto/ssh"

	"github.com/vladimirvivien/starkite/starbase"
)

// exec executes a command on remote hosts.
func (c *SSHClient) exec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Extract env dict manually (startype can't handle it)
	var envDict *starlark.Dict
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "env" {
			if d, ok := kv[1].(*starlark.Dict); ok {
				envDict = d
			}
		} else {
			filteredKwargs = append(filteredKwargs, kv)
		}
	}

	var p struct {
		Cmd    string `name:"cmd" position:"0" required:"true"`
		Sudo   bool   `name:"sudo"`
		AsUser string `name:"as_user"`
		Cwd    string `name:"cwd"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	// Permission check for SSH exec - check each host
	for _, host := range c.hosts {
		if err := starbase.Check(thread, "ssh", "exec", host+":"+p.Cmd); err != nil {
			return nil, err
		}
	}

	// Apply defaults
	sudo := p.Sudo
	if !sudo {
		sudo = c.defaultSudo
	}
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
	for k, v := range c.defaultEnv {
		env[k] = v
	}
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
	finalCmd := p.Cmd
	if cwd != "" {
		finalCmd = fmt.Sprintf("cd %s && %s", cwd, p.Cmd)
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

	if c.dryRun {
		return c.dryRunExecResults(finalCmd), nil
	}

	if len(c.hosts) == 0 {
		return starlark.NewList(nil), nil
	}

	// Execute on all hosts
	if c.execPolicy == "concurrent" {
		return c.execConcurrent(finalCmd)
	}
	return c.execLinear(finalCmd)
}

func (c *SSHClient) dryRunExecResults(cmd string) starlark.Value {
	results := make([]starlark.Value, len(c.hosts))
	for i, host := range c.hosts {
		results[i] = starlarkstruct.FromStringDict(starlark.String("SSHResult"), starlark.StringDict{
			"host":    starlark.String(host),
			"stdout":  starlark.String(fmt.Sprintf("[DRY RUN] Would execute on %s: %s", host, cmd)),
			"stderr":  starlark.String(""),
			"code":    starlark.MakeInt(0),
			"ok":      starlark.True,
			"dry_run": starlark.True,
		})
	}
	return starlark.NewList(results)
}

func (c *SSHClient) execConcurrent(cmd string) (starlark.Value, error) {
	results := make([]starlark.Value, len(c.hosts))
	errors := make([]error, len(c.hosts))
	var wg sync.WaitGroup

	for i, host := range c.hosts {
		wg.Add(1)
		go func(idx int, h string) {
			defer wg.Done()
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

	return starlarkstruct.FromStringDict(starlark.String("SSHResult"), starlark.StringDict{
		"host":   starlark.String(host),
		"stdout": starlark.String(stdout.String()),
		"stderr": starlark.String(stderr.String()),
		"code":   starlark.MakeInt(exitCode),
		"ok":     starlark.Bool(exitCode == 0),
	}), nil
}
