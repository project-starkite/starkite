package containers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Container represents an instantiated container.
type Container struct {
	engine *EngineClient
	id     string
	name   string
	image  string
	status string
}

var _ starlark.Value = (*Container)(nil)
var _ starlark.HasAttrs = (*Container)(nil)

// NewContainer creates a new Starlark Container handle.
func NewContainer(engine *EngineClient, id, name, image, status string) *Container {
	return &Container{
		engine: engine,
		id:     id,
		name:   strings.TrimPrefix(name, "/"),
		image:  image,
		status: status,
	}
}

func (c *Container) ID() string     { return c.id }
func (c *Container) Name() string   { return c.name }
func (c *Container) Image() string  { return c.image }
func (c *Container) Status() string { return c.status }

func (c *Container) String() string {
	shortID := c.id
	if len(shortID) > 12 {
		shortID = shortID[:12]
	}
	return fmt.Sprintf("<containers.Container id=%q name=%q image=%q status=%q>", shortID, c.name, c.image, c.status)
}

func (c *Container) Type() string         { return "containers.Container" }
func (c *Container) Freeze()              {}
func (c *Container) Truth() starlark.Bool { return true }
func (c *Container) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: containers.Container")
}

func (c *Container) AttrNames() []string {
	return []string{"id", "image", "inspect", "name", "port", "restart", "remove", "start", "status", "stop", "wait"}
}

func (c *Container) Attr(name string) (starlark.Value, error) {
	if baseName, ok := strings.CutPrefix(name, "try_"); ok {
		val, err := c.Attr(baseName)
		if err != nil || val == nil {
			return nil, err
		}
		if b, ok := val.(*starlark.Builtin); ok {
			return libkite.TryWrap("containers.Container."+name, b), nil
		}
		return nil, nil
	}

	switch name {
	case "id":
		return starlark.String(c.id), nil
	case "name":
		return starlark.String(c.name), nil
	case "image":
		return starlark.String(c.image), nil
	case "status":
		return starlark.String(c.status), nil
	case "start":
		return starlark.NewBuiltin("Container.start", c.startFn), nil
	case "stop":
		return starlark.NewBuiltin("Container.stop", c.stopFn), nil
	case "restart":
		return starlark.NewBuiltin("Container.restart", c.restartFn), nil
	case "remove":
		return starlark.NewBuiltin("Container.remove", c.removeFn), nil
	case "wait":
		return starlark.NewBuiltin("Container.wait", c.waitFn), nil
	case "inspect":
		return starlark.NewBuiltin("Container.inspect", c.inspectFn), nil
	case "port":
		return starlark.NewBuiltin("Container.port", c.portFn), nil
	default:
		return nil, nil
	}
}

func (c *Container) getContext(thread *starlark.Thread) context.Context {
	ctx := context.Background()
	if rt := libkite.GetRuntime(thread); rt != nil {
		ctx = rt.Context()
	}
	return ctx
}

// startFn implements container.start() -> None
func (c *Container) startFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "start", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.StartContainer(ctx, c.id); err != nil {
		return nil, err
	}
	c.status = "running"
	return starlark.None, nil
}

// stopFn implements container.stop(timeout=10) -> None
func (c *Container) stopFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var timeout = 10
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "timeout?", &timeout); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "stop", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.StopContainer(ctx, c.id, timeout); err != nil {
		return nil, err
	}
	c.status = "exited"
	return starlark.None, nil
}

// restartFn implements container.restart(timeout=10) -> None
func (c *Container) restartFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var timeout = 10
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "timeout?", &timeout); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "restart", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.RestartContainer(ctx, c.id, timeout); err != nil {
		return nil, err
	}
	c.status = "running"
	return starlark.None, nil
}

// removeFn implements container.remove(force=False, volumes=False) -> None
func (c *Container) removeFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var force bool
	var volumes bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "force?", &force, "volumes?", &volumes); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "manage", "remove", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.RemoveContainer(ctx, c.id, force, volumes); err != nil {
		return nil, err
	}
	c.status = "removed"
	return starlark.None, nil
}

// waitFn implements container.wait(condition="not-running") -> int
func (c *Container) waitFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var condition = "not-running"
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "condition?", &condition); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "wait", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	code, err := c.engine.WaitContainer(ctx, c.id, condition)
	if err != nil {
		return starlark.MakeInt(code), err
	}
	return starlark.MakeInt(code), nil
}

// inspectFn implements container.inspect() -> dict
func (c *Container) inspectFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "inspect", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	inspectData, err := c.engine.InspectContainer(ctx, c.id)
	if err != nil {
		return nil, err
	}

	if state, ok := inspectData["State"].(map[string]any); ok {
		if status, ok := state["Status"].(string); ok {
			c.status = status
		}
	}

	return startype.Go[any](inspectData).ToStarlarkValue()
}

// portFn implements container.port(container_port) -> int
func (c *Container) portFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var portSpec string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "port", &portSpec); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "port", c.id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	inspectData, err := c.engine.InspectContainer(ctx, c.id)
	if err != nil {
		return nil, err
	}

	normKey := NormalizePortKey(portSpec)
	netSettings, _ := inspectData["NetworkSettings"].(map[string]any)
	if netSettings == nil {
		return nil, fmt.Errorf("containers: no NetworkSettings found for container %s", c.id)
	}
	ports, _ := netSettings["Ports"].(map[string]any)
	if ports == nil {
		return nil, fmt.Errorf("containers: no Ports found for container %s", c.id)
	}

	rawBindings, ok := ports[normKey]
	if !ok || rawBindings == nil {
		return nil, fmt.Errorf("containers: port %q not exposed or bound on container %s", portSpec, c.id)
	}

	bindings, ok := rawBindings.([]any)
	if !ok || len(bindings) == 0 {
		return nil, fmt.Errorf("containers: port %q not bound on container %s", portSpec, c.id)
	}

	bindingMap, ok := bindings[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("containers: unexpected binding format for port %q", portSpec)
	}

	hostPortStr, _ := bindingMap["HostPort"].(string)
	hostPort, err := strconv.Atoi(hostPortStr)
	if err != nil {
		return nil, fmt.Errorf("containers: invalid host port %q: %w", hostPortStr, err)
	}

	return starlark.MakeInt(hostPort), nil
}
