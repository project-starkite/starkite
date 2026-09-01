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
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/fleet"
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
			"config":  starlark.NewBuiltin("ssh.config", m.sshConfig),
			"copy_id": starlark.NewBuiltin("ssh.copy_id", m.sshCopyId),
			"exec":    starlark.NewBuiltin("ssh.exec", m.sshExec),
			"keygen":  starlark.NewBuiltin("ssh.keygen", m.sshKeygen),
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

func newSSHKeyPair(kp *KeyPair) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("SSHKeyPair"), starlark.StringDict{
		"public_key":  starlark.String(kp.PublicKey),
		"private_key": starlark.String(kp.PrivateKey),
		"fingerprint": starlark.String(kp.Fingerprint),
		"type":        starlark.String(kp.Type),
		"comment":     starlark.String(kp.Comment),
		"path":        starlark.String(kp.Path),
		"pub_path":    starlark.String(kp.PubPath),
	})
}

// sshKeygen generates an in-memory or on-disk SSH keypair.
func (m *Module) sshKeygen(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Type       string `name:"type" position:"0"`
		Bits       int    `name:"bits"`
		Comment    string `name:"comment"`
		Passphrase string `name:"passphrase"`
		Path       string `name:"path"`
		Overwrite  bool   `name:"overwrite"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	kp, err := GenerateKeyPairWithOptions(KeyGenOptions{
		Type:       p.Type,
		Bits:       p.Bits,
		Comment:    p.Comment,
		Passphrase: p.Passphrase,
		Path:       p.Path,
		Overwrite:  p.Overwrite,
	})
	if err != nil {
		return nil, fmt.Errorf("ssh.keygen: %w", err)
	}

	return newSSHKeyPair(kp), nil
}

// sshExec executes a command or pipeline across a fleet or host list in a single one-shot call.
// Signatures:
//   - ssh.exec("uptime", fleet=web_fleet, user="deploy")
//   - ssh.exec("git", ["pull", "origin", "main"], hosts=["192.168.1.10"], user="root")
//   - ssh.exec(cmd="uptime", hosts="192.168.1.10", user="root")
func (m *Module) sshExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var execArgs starlark.Tuple
	var execKwargs []starlark.Tuple
	var configKwargs []starlark.Tuple

	if len(args) > 0 {
		execArgs = args
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "cmd":
			if len(execArgs) == 0 {
				execArgs = starlark.Tuple{kv[1]}
			} else {
				execKwargs = append(execKwargs, kv)
			}
		case "commands", "sudo", "as_user", "cwd", "env", "exec_on_error":
			execKwargs = append(execKwargs, kv)
		default:
			// config parameters (fleet, hosts, user, key, port, timeout, exec_policy, exec_max_workers, etc.)
			configKwargs = append(configKwargs, kv)
		}
	}

	clientVal, err := m.sshConfig(thread, fn, nil, configKwargs)
	if err != nil {
		return nil, err
	}
	client, ok := clientVal.(*SSHClient)
	if !ok {
		return nil, fmt.Errorf("ssh.exec: failed to create SSH client")
	}

	return client.exec(thread, fn, execArgs, execKwargs)
}

// sshConfig creates a configured SSH client.
// Usage: ssh.config(hosts=["host1", "host2"], user="root", key="/path/to/key", ...)
func (m *Module) sshConfig(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Permission check for creating SSH client
	if err := libkite.Check(thread, "ssh", "connect", "config", ""); err != nil {
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

	// Extract fleet, hosts shortcut, and env dict manually
	var rawFleet starlark.Value
	var rawHosts starlark.Value
	var defaultEnv *starlark.Dict
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "fleet":
			rawFleet = kv[1]
		case "hosts":
			rawHosts = kv[1]
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
		ExecMaxWorkers    int    `name:"exec_max_workers"`
		MaxWorkers        int    `name:"max_workers"`
		ExecOnError       string `name:"exec_on_error"`
		JumpHost          string `name:"jump_host"`
		JumpUser          string `name:"jump_user"`
		JumpKey           string `name:"jump_key"`
		JumpPassword      string `name:"jump_password"`
		JumpPort          int    `name:"jump_port"`
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

	// Apply fleet if provided
	if rawFleet != nil && rawFleet != starlark.None {
		if fl, ok := rawFleet.(*fleet.Fleet); ok {
			client.fleet = fl
			for _, r := range fl.Resources() {
				if r.Address != "" {
					client.hosts = append(client.hosts, r.Address)
				}
			}
		} else {
			fl, err := fleet.FromSource(thread, rawFleet)
			if err != nil {
				return nil, fmt.Errorf("ssh.config: invalid fleet argument: %w", err)
			}
			client.fleet = fl
			for _, r := range fl.Resources() {
				if r.Address != "" {
					client.hosts = append(client.hosts, r.Address)
				}
			}
		}
	}

	// Apply hosts shortcut if provided
	if rawHosts != nil && rawHosts != starlark.None {
		switch h := rawHosts.(type) {
		case *starlark.List:
			for i := 0; i < h.Len(); i++ {
				if s, ok := starlark.AsString(h.Index(i)); ok {
					client.hosts = append(client.hosts, s)
				}
			}
		case starlark.Tuple:
			for _, elem := range h {
				if s, ok := starlark.AsString(elem); ok {
					client.hosts = append(client.hosts, s)
				}
			}
		case starlark.String:
			client.hosts = append(client.hosts, string(h))
		}
	}

	// Synthesize fleet from hosts if only hosts shortcut was provided
	if client.fleet == nil && len(client.hosts) > 0 {
		var resources []fleet.Resource
		for _, h := range client.hosts {
			resources = append(resources, fleet.Resource{
				ID:      h,
				Name:    h,
				Kind:    "host",
				Address: h,
				Labels:  make(map[string]string),
				Data:    make(map[string]any),
			})
		}
		client.fleet = fleet.New(resources)
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
	if p.ExecMaxWorkers > 0 {
		client.execMaxWorkers = p.ExecMaxWorkers
	} else if p.MaxWorkers > 0 {
		client.execMaxWorkers = p.MaxWorkers
	}
	if p.ExecOnError != "" {
		if p.ExecOnError != "stop" && p.ExecOnError != "continue" {
			return nil, fmt.Errorf("ssh.config: invalid exec_on_error %q (must be \"stop\" or \"continue\")", p.ExecOnError)
		}
		client.execOnError = p.ExecOnError
	} else {
		client.execOnError = "stop"
	}
	if p.JumpHost != "" {
		client.jumpHost = p.JumpHost
	}
	if p.JumpUser != "" {
		client.jumpUser = p.JumpUser
	}
	if p.JumpKey != "" {
		client.jumpKeyFile = p.JumpKey
	}
	if p.JumpPassword != "" {
		client.jumpPassword = p.JumpPassword
	}
	if p.JumpPort > 0 {
		client.jumpPort = p.JumpPort
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
	fleet             *fleet.Fleet
	hosts             []string
	user              string
	keyFile           string
	keyPassphrase     string
	password          string
	port              int
	timeout           time.Duration
	maxRetries        int
	execPolicy        string // "concurrent" or "linear"
	execMaxWorkers    int
	execOnError       string // "stop" or "continue"
	jumpHost          string
	jumpUser          string
	jumpKeyFile       string
	jumpPassword      string
	jumpPort          int
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
		case "copy_id":
			return libkite.TryWrap("ssh.client."+name, starlark.NewBuiltin("ssh.client.copy_id", c.copyId)), nil
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
	case "copy_id":
		return starlark.NewBuiltin("ssh.client.copy_id", c.copyId), nil
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
	case "fleet":
		if c.fleet != nil {
			return c.fleet, nil
		}
		return starlark.None, nil
	case "exec_max_workers":
		return starlark.MakeInt(c.execMaxWorkers), nil
	case "exec_on_error":
		return starlark.String(c.execOnError), nil
	case "exec_policy":
		return starlark.String(c.execPolicy), nil
	case "jump_host":
		return starlark.String(c.jumpHost), nil
	case "jump_user":
		return starlark.String(c.jumpUser), nil
	case "jump_port":
		return starlark.MakeInt(c.jumpPort), nil
	default:
		return nil, nil
	}
}

func (c *SSHClient) AttrNames() []string {
	return []string{"copy_id", "download", "exec", "exec_max_workers", "exec_on_error", "exec_policy", "fleet", "hosts", "jump_host", "jump_port", "jump_user", "try_copy_id", "try_download", "try_exec", "try_upload", "upload"}
}
