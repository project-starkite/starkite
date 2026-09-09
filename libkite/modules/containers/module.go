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
	once         sync.Once
	module       starlark.Value
	config       *libkite.ModuleConfig
	mu           sync.Mutex
	cachedClient *Client
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
		members := starlark.StringDict{
			"config":        starlark.NewBuiltin("containers.config", m.configConstructor),
			"run":           starlark.NewBuiltin("containers.run", m.shortcutRun),
			"exec":          starlark.NewBuiltin("containers.exec", m.shortcutExec),
			"stop":          starlark.NewBuiltin("containers.stop", m.shortcutStop),
			"delete":        starlark.NewBuiltin("containers.delete", m.shortcutDelete),
			"remove":        starlark.NewBuiltin("containers.remove", m.shortcutDelete),
			"image_pull":    starlark.NewBuiltin("containers.image_pull", m.shortcutImagePull),
			"pull":          starlark.NewBuiltin("containers.pull", m.shortcutImagePull),
			"image_build":   starlark.NewBuiltin("containers.image_build", m.shortcutImageBuild),
			"build":         starlark.NewBuiltin("containers.build", m.shortcutImageBuild),
			"image_list":    starlark.NewBuiltin("containers.image_list", m.shortcutImageList),
			"images":        starlark.NewBuiltin("containers.images", m.shortcutImageList),
			"image_inspect": starlark.NewBuiltin("containers.image_inspect", m.shortcutImageInspect),
			"image_remove":  starlark.NewBuiltin("containers.image_remove", m.shortcutImageRemove),
			"rmi":           starlark.NewBuiltin("containers.rmi", m.shortcutImageRemove),
		}
		m.module = libkite.NewTryModule(string(ModuleName), members)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "config" }

func (m *Module) ensureDefaultClient(thread *starlark.Thread) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cachedClient != nil {
		return m.cachedClient, nil
	}

	ep, err := DiscoverEndpoint("")
	if err != nil {
		return nil, fmt.Errorf("containers: %w", err)
	}

	if err := libkite.Check(thread, "containers", "connect", "default", ep.Address); err != nil {
		return nil, err
	}

	eng, err := NewEngineClient(ep, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("containers: %w", err)
	}

	m.cachedClient = &Client{
		engine:     eng,
		socketPath: ep.Address,
	}
	return m.cachedClient, nil
}

func (m *Module) shortcutRun(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.runFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.execFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutStop(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.stopFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutDelete(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.deleteFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutImagePull(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.imagePullFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutImageBuild(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.imageBuildFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutImageList(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.imageListFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutImageInspect(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.imageInspectFn(thread, fn, args, kwargs)
}

func (m *Module) shortcutImageRemove(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	client, err := m.ensureDefaultClient(thread)
	if err != nil {
		return nil, err
	}
	return client.imageRemoveFn(thread, fn, args, kwargs)
}

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
