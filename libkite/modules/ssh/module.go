// Package ssh provides SSH remote execution for starkite.
// This is a factory module: ssh.config() returns an SSH client.
package ssh

import (
	"fmt"
	"os"
	"os/user"
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
			"keyscan": starlark.NewBuiltin("ssh.keyscan", m.sshKeyscan),
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

func defaultUser() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "root"
}

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

// sshExec executes a command or pipeline in a single one-shot call across explicit target hosts.
// Usage: ssh.exec("uptime", hosts=["192.168.1.10"], user="root", key="~/.ssh/id_ed25519", use_agent=True)
func (m *Module) sshExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var execArgs starlark.Tuple
	var rawCmd starlark.Value
	var rawCommands starlark.Value
	var rawHosts starlark.Value
	var defaultEnv *starlark.Dict

	if len(args) > 0 {
		rawCmd = args[0]
		if len(args) > 1 {
			execArgs = args[1:]
		}
	}

	var p struct {
		User         string `name:"user"`
		Key          string `name:"key"`
		Passphrase   string `name:"passphrase"`
		Password     string `name:"password"`
		UseAgent     bool   `name:"use_agent"`
		Prompt       bool   `name:"prompt"`
		Port         int    `name:"port"`
		Timeout      string `name:"timeout"`
		Sudo         bool   `name:"sudo"`
		AsUser       string `name:"as_user"`
		Cwd          string `name:"cwd"`
		DryRun       bool   `name:"dry_run"`
		HostKeyCheck bool   `name:"host_key_check"`
		ExecOnError  string `name:"exec_on_error"`
	}
	p.HostKeyCheck = true

	var remainingKwargs []starlark.Tuple
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "cmd":
			rawCmd = kv[1]
		case "commands":
			rawCommands = kv[1]
		case "hosts":
			rawHosts = kv[1]
		case "fleet":
			return nil, fmt.Errorf("ssh.exec: 'fleet' parameter is not supported in module-scope functions; use ssh.config(fleet=...) instead")
		case "jump", "jump_host", "jump_user", "jump_key", "jump_password", "jump_key_passphrase", "jump_port":
			return nil, fmt.Errorf("ssh.exec: bastion/jump host routing is not supported in module-scope functions; use ssh.config(jump={...}) instead")
		case "env":
			if d, ok := kv[1].(*starlark.Dict); ok {
				defaultEnv = d
			}
		default:
			remainingKwargs = append(remainingKwargs, kv)
		}
	}

	if err := startype.Args(nil, remainingKwargs).Go(&p); err != nil {
		return nil, err
	}

	if rawHosts == nil || rawHosts == starlark.None {
		return nil, fmt.Errorf("ssh.exec: missing required parameter 'hosts'")
	}

	var hosts []string
	switch h := rawHosts.(type) {
	case *starlark.List:
		for i := 0; i < h.Len(); i++ {
			if s, ok := starlark.AsString(h.Index(i)); ok {
				hosts = append(hosts, s)
			}
		}
	case starlark.Tuple:
		for _, elem := range h {
			if s, ok := starlark.AsString(elem); ok {
				hosts = append(hosts, s)
			}
		}
	case starlark.String:
		hosts = append(hosts, string(h))
	default:
		return nil, fmt.Errorf("ssh.exec: 'hosts' must be a string or list of strings, got %s", rawHosts.Type())
	}

	if len(hosts) == 0 {
		return nil, fmt.Errorf("ssh.exec: 'hosts' cannot be empty")
	}

	user := p.User
	if user == "" {
		user = defaultUser()
	}
	port := p.Port
	if port <= 0 {
		port = 22
	}
	timeout := 30 * time.Second
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("ssh.exec: invalid timeout %q: %w", p.Timeout, err)
		}
		timeout = d
	}

	client := &SSHClient{
		thread:            thread,
		dryRun:            p.DryRun || (m.config != nil && m.config.DryRun),
		debug:             m.config != nil && m.config.Debug,
		hosts:             hosts,
		user:              user,
		keyFile:           p.Key,
		keyPassphrase:     p.Passphrase,
		password:          p.Password,
		useAgent:          p.UseAgent,
		prompt:            p.Prompt,
		port:              port,
		timeout:           timeout,
		maxRetries:        3,
		execPolicy:        "concurrent",
		execOnError:       p.ExecOnError,
		hostKeyCheck:      p.HostKeyCheck,
		keepAliveInterval: 30 * time.Second,
		keepAliveMax:      3,
		defaultSudo:       p.Sudo,
		defaultAsUser:     p.AsUser,
		defaultCwd:        p.Cwd,
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

	var callArgs starlark.Tuple
	var callKwargs []starlark.Tuple
	if rawCmd != nil {
		callArgs = append(starlark.Tuple{rawCmd}, execArgs...)
	}
	if rawCommands != nil {
		callKwargs = append(callKwargs, starlark.Tuple{starlark.String("commands"), rawCommands})
	}

	return client.exec(thread, fn, callArgs, callKwargs)
}

type authConfig struct {
	User       string
	Key        string
	Passphrase string
	Password   string
	UseAgent   bool
	Prompt     bool
}

type jumpConfig struct {
	Host       string
	Port       int
	User       string
	Key        string
	Passphrase string
	Password   string
	UseAgent   bool
	Prompt     bool
}

func parseAuthDict(d *starlark.Dict) (authConfig, error) {
	cfg := authConfig{
		User: defaultUser(),
	}
	if d == nil {
		return cfg, nil
	}
	for _, item := range d.Items() {
		k, ok := starlark.AsString(item[0])
		if !ok {
			return cfg, fmt.Errorf("auth: key must be a string, got %s", item[0].Type())
		}
		switch k {
		case "user":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.User = s
			} else {
				return cfg, fmt.Errorf("auth.user must be a string, got %s", item[1].Type())
			}
		case "key":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Key = s
			} else {
				return cfg, fmt.Errorf("auth.key must be a string, got %s", item[1].Type())
			}
		case "passphrase":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Passphrase = s
			} else {
				return cfg, fmt.Errorf("auth.passphrase must be a string, got %s", item[1].Type())
			}
		case "password":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Password = s
			} else {
				return cfg, fmt.Errorf("auth.password must be a string, got %s", item[1].Type())
			}
		case "use_agent":
			if b, ok := item[1].(starlark.Bool); ok {
				cfg.UseAgent = bool(b)
			} else {
				return cfg, fmt.Errorf("auth.use_agent must be a bool, got %s", item[1].Type())
			}
		case "prompt":
			if b, ok := item[1].(starlark.Bool); ok {
				cfg.Prompt = bool(b)
			} else {
				return cfg, fmt.Errorf("auth.prompt must be a bool, got %s", item[1].Type())
			}
		default:
			return cfg, fmt.Errorf("auth: unexpected field %q (allowed: user, key, passphrase, password, use_agent, prompt)", k)
		}
	}
	return cfg, nil
}

func parseJumpDict(d *starlark.Dict, targetAuth authConfig) (jumpConfig, error) {
	cfg := jumpConfig{
		Port:       22,
		User:       targetAuth.User,
		Key:        targetAuth.Key,
		Passphrase: targetAuth.Passphrase,
		Password:   targetAuth.Password,
		UseAgent:   targetAuth.UseAgent,
		Prompt:     targetAuth.Prompt,
	}
	if d == nil {
		return cfg, nil
	}
	for _, item := range d.Items() {
		k, ok := starlark.AsString(item[0])
		if !ok {
			return cfg, fmt.Errorf("jump: key must be a string, got %s", item[0].Type())
		}
		switch k {
		case "host":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Host = s
			} else {
				return cfg, fmt.Errorf("jump.host must be a string, got %s", item[1].Type())
			}
		case "port":
			var p int
			if err := startype.Starlark(item[1]).Go(&p); err == nil && p > 0 {
				cfg.Port = p
			} else {
				return cfg, fmt.Errorf("jump.port must be a positive integer, got %v", item[1])
			}
		case "user":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.User = s
			} else {
				return cfg, fmt.Errorf("jump.user must be a string, got %s", item[1].Type())
			}
		case "key":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Key = s
			} else {
				return cfg, fmt.Errorf("jump.key must be a string, got %s", item[1].Type())
			}
		case "passphrase":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Passphrase = s
			} else {
				return cfg, fmt.Errorf("jump.passphrase must be a string, got %s", item[1].Type())
			}
		case "password":
			if s, ok := starlark.AsString(item[1]); ok {
				cfg.Password = s
			} else {
				return cfg, fmt.Errorf("jump.password must be a string, got %s", item[1].Type())
			}
		case "use_agent":
			if b, ok := item[1].(starlark.Bool); ok {
				cfg.UseAgent = bool(b)
			} else {
				return cfg, fmt.Errorf("jump.use_agent must be a bool, got %s", item[1].Type())
			}
		case "prompt":
			if b, ok := item[1].(starlark.Bool); ok {
				cfg.Prompt = bool(b)
			} else {
				return cfg, fmt.Errorf("jump.prompt must be a bool, got %s", item[1].Type())
			}
		default:
			return cfg, fmt.Errorf("jump: unexpected field %q (allowed: host, port, user, key, passphrase, password, use_agent, prompt)", k)
		}
	}
	if cfg.Host == "" {
		return cfg, fmt.Errorf("jump: 'host' is required in jump configuration")
	}
	return cfg, nil
}

// sshConfig creates a configured SSH client.
// Usage: ssh.config(hosts=["host1", "host2"], auth={"user": "root", "key": "/path/to/key"}, jump={"host": "bastion"})
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

	// Extract fleet, hosts shortcut, auth dict, jump dict, and env dict manually
	var rawFleet starlark.Value
	var rawHosts starlark.Value
	var rawAuth *starlark.Dict
	var rawJump *starlark.Dict
	var defaultEnv *starlark.Dict
	filteredKwargs := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		switch key {
		case "fleet":
			rawFleet = kv[1]
		case "hosts":
			rawHosts = kv[1]
		case "auth":
			if d, ok := kv[1].(*starlark.Dict); ok {
				rawAuth = d
			} else {
				return nil, fmt.Errorf("ssh.config: 'auth' must be a dict, got %s", kv[1].Type())
			}
		case "jump":
			if d, ok := kv[1].(*starlark.Dict); ok {
				rawJump = d
			} else {
				return nil, fmt.Errorf("ssh.config: 'jump' must be a dict, got %s", kv[1].Type())
			}
		case "env":
			if d, ok := kv[1].(*starlark.Dict); ok {
				defaultEnv = d
			}
		case "user", "key", "password", "key_passphrase", "passphrase", "use_agent", "ask_passphrase", "prompt", "ask_password":
			return nil, fmt.Errorf("ssh.config: flat credential parameter %q is not supported; configure target credentials in auth={...}", key)
		case "jump_host", "jump_user", "jump_key", "jump_password", "jump_key_passphrase", "jump_port":
			return nil, fmt.Errorf("ssh.config: flat jump parameter %q is not supported; configure bastion routing in jump={...}", key)
		default:
			filteredKwargs = append(filteredKwargs, kv)
		}
	}

	// Use startype for remaining simple parameters
	var p struct {
		Port              int    `name:"port"`
		Timeout           string `name:"timeout"`
		MaxRetries        int    `name:"max_retries"`
		ExecPolicy        string `name:"exec_policy"`
		ExecMaxWorkers    int    `name:"exec_max_workers"`
		MaxWorkers        int    `name:"max_workers"`
		ExecOnError       string `name:"exec_on_error"`
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

	// Apply structured auth
	authCfg, err := parseAuthDict(rawAuth)
	if err != nil {
		return nil, fmt.Errorf("ssh.config: %w", err)
	}
	client.user = authCfg.User
	client.keyFile = authCfg.Key
	client.keyPassphrase = authCfg.Passphrase
	client.password = authCfg.Password
	client.useAgent = authCfg.UseAgent
	client.prompt = authCfg.Prompt

	// Apply structured jump
	if rawJump != nil {
		jumpCfg, err := parseJumpDict(rawJump, authCfg)
		if err != nil {
			return nil, fmt.Errorf("ssh.config: %w", err)
		}
		client.jumpHost = jumpCfg.Host
		client.jumpPort = jumpCfg.Port
		client.jumpUser = jumpCfg.User
		client.jumpKeyFile = jumpCfg.Key
		client.jumpKeyPassphrase = jumpCfg.Passphrase
		client.jumpPassword = jumpCfg.Password
		client.jumpUseAgent = jumpCfg.UseAgent
		client.jumpPrompt = jumpCfg.Prompt
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
	client.defaultAsUser = p.AsUser
	client.defaultCwd = p.Cwd
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
	client.dryRun = p.DryRun

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
	useAgent          bool
	prompt            bool
	jumpHost          string
	jumpPort          int
	jumpUser          string
	jumpKeyFile       string
	jumpKeyPassphrase string
	jumpPassword      string
	jumpUseAgent      bool
	jumpPrompt        bool
	port              int
	timeout           time.Duration
	maxRetries        int
	execPolicy        string // "concurrent" or "linear"
	execMaxWorkers    int
	execOnError       string // "stop" or "continue"
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
		case "keyscan":
			return libkite.TryWrap("ssh.client."+name, starlark.NewBuiltin("ssh.client.keyscan", c.keyscan)), nil
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
	case "keyscan":
		return starlark.NewBuiltin("ssh.client.keyscan", c.keyscan), nil
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
	case "auth":
		d := starlark.NewDict(6)
		d.SetKey(starlark.String("user"), starlark.String(c.user))
		d.SetKey(starlark.String("key"), starlark.String(c.keyFile))
		d.SetKey(starlark.String("passphrase"), starlark.String(c.keyPassphrase))
		d.SetKey(starlark.String("password"), starlark.String(c.password))
		d.SetKey(starlark.String("use_agent"), starlark.Bool(c.useAgent))
		d.SetKey(starlark.String("prompt"), starlark.Bool(c.prompt))
		return d, nil
	case "jump":
		if c.jumpHost == "" {
			return starlark.None, nil
		}
		d := starlark.NewDict(8)
		d.SetKey(starlark.String("host"), starlark.String(c.jumpHost))
		d.SetKey(starlark.String("port"), starlark.MakeInt(c.jumpPort))
		d.SetKey(starlark.String("user"), starlark.String(c.jumpUser))
		d.SetKey(starlark.String("key"), starlark.String(c.jumpKeyFile))
		d.SetKey(starlark.String("passphrase"), starlark.String(c.jumpKeyPassphrase))
		d.SetKey(starlark.String("password"), starlark.String(c.jumpPassword))
		d.SetKey(starlark.String("use_agent"), starlark.Bool(c.jumpUseAgent))
		d.SetKey(starlark.String("prompt"), starlark.Bool(c.jumpPrompt))
		return d, nil
	case "exec_max_workers":
		return starlark.MakeInt(c.execMaxWorkers), nil
	case "exec_on_error":
		return starlark.String(c.execOnError), nil
	case "exec_policy":
		return starlark.String(c.execPolicy), nil
	default:
		return nil, nil
	}
}

func (c *SSHClient) AttrNames() []string {
	return []string{"auth", "copy_id", "download", "exec", "exec_max_workers", "exec_on_error", "exec_policy", "fleet", "hosts", "jump", "keyscan", "try_copy_id", "try_download", "try_exec", "try_keyscan", "try_upload", "upload"}
}
