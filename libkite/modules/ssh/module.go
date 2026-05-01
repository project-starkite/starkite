// Package ssh provides SSH remote execution for starkite.
// This is a factory module: ssh.config() returns an SSH client.
package ssh

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "ssh"

// Module implements SSH remote execution.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "ssh provides SSH remote execution: config() returns a client for remote commands"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		members := starlark.StringDict{
			"config": starlark.NewBuiltin("ssh.config", m.sshConfig),
		}
		if config != nil && config.TestMode {
			members["test_server"] = starlark.NewBuiltin("ssh.test_server", m.testserverFactory)
			members["test_key"] = starlark.NewBuiltin("ssh.test_key", m.testKeyFactory)
		}
		m.module = libkite.NewTryModule(string(ModuleName), members)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }

func (m *Module) FactoryMethod() string { return "config" }

// sshConfig creates a configured SSH client.
// Usage: ssh.config(hosts=["host1", "host2"], user="root", key="/path/to/key", ...)
func (m *Module) sshConfig(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Permission check for creating SSH client
	if err := libkite.Check(thread, "ssh", "config", ""); err != nil {
		return nil, err
	}

	client := &SSHClient{
		thread:            thread,
		dryRun:            m.config != nil && m.config.DryRun,
		debug:             m.config != nil && m.config.Debug,
		port:              22,
		timeout:           30 * time.Second,
		maxRetries:        3,
		execPolicy:        "concurrent",
		hostKeyCheck:      true,
		keepAliveInterval: 30 * time.Second,
		keepAliveMax:      3,
	}

	// Extract hosts list and env dict manually (startype can't handle these)
	var hostList *starlark.List
	var defaultEnv *starlark.Dict
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "hosts":
			if l, ok := kv[1].(*starlark.List); ok {
				hostList = l
			}
		case "env":
			if d, ok := kv[1].(*starlark.Dict); ok {
				defaultEnv = d
			}
		default:
			filteredKwargs = append(filteredKwargs, kv)
		}
	}

	// Use startype for remaining simple parameters
	var p struct {
		User              string `name:"user"`
		Key               string `name:"key"`
		KeyPassphrase     string `name:"key_passphrase"`
		Password          string `name:"password"`
		Port              int    `name:"port"`
		Timeout           string `name:"timeout"`
		MaxRetries        int    `name:"max_retries"`
		ExecPolicy        string `name:"exec_policy"`
		JumpHost          string `name:"jump_host"`
		KnownHostsFile    string `name:"known_hosts_file"`
		HostKeyCheck      bool   `name:"host_key_check"`
		KeepAliveInterval string `name:"keep_alive_interval"`
		KeepAliveMax      int    `name:"keep_alive_max"`
		Sudo              bool   `name:"sudo"`
		AsUser            string `name:"as_user"`
		Cwd               string `name:"cwd"`
		DryRun            bool   `name:"dry_run"`
	}
	if err := startype.Args(args, filteredKwargs).Go(&p); err != nil {
		return nil, err
	}

	// Apply hosts list
	if hostList != nil {
		for i := 0; i < hostList.Len(); i++ {
			if h, ok := starlark.AsString(hostList.Index(i)); ok {
				client.hosts = append(client.hosts, h)
			}
		}
	}

	// Apply simple parameters
	if p.User != "" {
		client.user = p.User
	}
	if p.Key != "" {
		client.keyFile = p.Key
	}
	if p.KeyPassphrase != "" {
		client.keyPassphrase = p.KeyPassphrase
	}
	if p.Password != "" {
		client.password = p.Password
	}
	if p.Port > 0 {
		client.port = p.Port
	}
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("ssh.config: invalid timeout %q: %w", p.Timeout, err)
		}
		client.timeout = d
	}
	if p.MaxRetries > 0 {
		client.maxRetries = p.MaxRetries
	}
	if p.ExecPolicy != "" {
		client.execPolicy = p.ExecPolicy
	}
	if p.JumpHost != "" {
		client.jumpHost = p.JumpHost
	}
	if p.KnownHostsFile != "" {
		client.knownHostsFile = p.KnownHostsFile
	}
	client.hostKeyCheck = p.HostKeyCheck
	if p.KeepAliveInterval != "" {
		d, err := time.ParseDuration(p.KeepAliveInterval)
		if err != nil {
			return nil, fmt.Errorf("ssh.config: invalid keep_alive_interval %q: %w", p.KeepAliveInterval, err)
		}
		client.keepAliveInterval = d
	}
	if p.KeepAliveMax > 0 {
		client.keepAliveMax = p.KeepAliveMax
	}
	client.defaultSudo = p.Sudo
	if p.AsUser != "" {
		client.defaultAsUser = p.AsUser
	}
	if defaultEnv != nil {
		client.defaultEnv = make(map[string]string)
		for _, item := range defaultEnv.Items() {
			if k, ok := starlark.AsString(item[0]); ok {
				if v, ok := starlark.AsString(item[1]); ok {
					client.defaultEnv[k] = v
				}
			}
		}
	}
	if p.Cwd != "" {
		client.defaultCwd = p.Cwd
	}
	// dry_run from kwarg overrides module-level config
	if p.DryRun {
		client.dryRun = true
	}

	return client, nil
}

// SSHClient represents a configured SSH client for remote execution.
type SSHClient struct {
	thread            *starlark.Thread
	dryRun            bool
	debug             bool
	hosts             []string
	user              string
	keyFile           string
	keyPassphrase     string
	password          string
	port              int
	timeout           time.Duration
	maxRetries        int
	execPolicy        string // "concurrent" or "linear"
	jumpHost          string
	knownHostsFile    string
	hostKeyCheck      bool
	keepAliveInterval time.Duration
	keepAliveMax      int
	defaultSudo       bool
	defaultAsUser     string
	defaultEnv        map[string]string
	defaultCwd        string
}

func (c *SSHClient) String() string        { return fmt.Sprintf("<ssh.client hosts=%v>", c.hosts) }
func (c *SSHClient) Type() string          { return "ssh.client" }
func (c *SSHClient) Freeze()               {}
func (c *SSHClient) Truth() starlark.Bool  { return starlark.Bool(len(c.hosts) > 0) }
func (c *SSHClient) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: ssh.client") }

func (c *SSHClient) Attr(name string) (starlark.Value, error) {
	// try_ prefix dispatch
	if baseName, ok := strings.CutPrefix(name, "try_"); ok {
		switch baseName {
		case "exec":
			return libkite.TryWrap("ssh.client."+name, starlark.NewBuiltin("ssh.client.exec", c.exec)), nil
		case "upload":
			return libkite.TryWrap("ssh.client."+name, starlark.NewBuiltin("ssh.client.upload", c.upload)), nil
		case "download":
			return libkite.TryWrap("ssh.client."+name, starlark.NewBuiltin("ssh.client.download", c.download)), nil
		}
		return nil, nil
	}
	switch name {
	case "exec":
		return starlark.NewBuiltin("ssh.client.exec", c.exec), nil
	case "upload":
		return starlark.NewBuiltin("ssh.client.upload", c.upload), nil
	case "download":
		return starlark.NewBuiltin("ssh.client.download", c.download), nil
	case "hosts":
		elems := make([]starlark.Value, len(c.hosts))
		for i, h := range c.hosts {
			elems[i] = starlark.String(h)
		}
		return starlark.NewList(elems), nil
	default:
		return nil, nil
	}
}

func (c *SSHClient) AttrNames() []string {
	return []string{"download", "exec", "hosts", "try_download", "try_exec", "try_upload", "upload"}
}
