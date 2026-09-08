package containers

import (
	"fmt"
	"sync"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

const ModuleName libkite.ModuleName = "containers"

// Module implements the containers standard library module.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "containers provides container engine management: config() returns a client for daemon operations"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		cfgBuiltin := starlark.NewBuiltin("containers.config", m.configConstructor)
		m.module = libkite.NewTryModule(string(ModuleName), starlark.StringDict{
			"config": cfgBuiltin,
			"client": cfgBuiltin, // alias for backwards compatibility
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "config" }

// configConstructor creates a new containers.Client instance.
// Signature: containers.config(host=None, timeout="30s")
func (m *Module) configConstructor(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Host    string `name:"host"`
		Timeout string `name:"timeout"`
	}
	p.Timeout = "30s"

	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("containers.config: %w", err)
	}

	ep, err := DiscoverEndpoint(p.Host)
	if err != nil {
		return nil, fmt.Errorf("containers.config: %w", err)
	}

	// Permissions check: gates connecting to daemon socket
	if err := libkite.Check(thread, "containers", "connect", "config", ep.Address); err != nil {
		return nil, err
	}

	timeoutDur, err := time.ParseDuration(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("containers.config: invalid timeout %q: %w", p.Timeout, err)
	}

	eng, err := NewEngineClient(ep, timeoutDur)
	if err != nil {
		return nil, fmt.Errorf("containers.config: %w", err)
	}

	return &Client{
		engine:     eng,
		socketPath: ep.Address,
	}, nil
}
