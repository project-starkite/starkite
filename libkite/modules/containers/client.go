package containers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/project-starkite/starkite/libkite"
	iomods "github.com/project-starkite/starkite/libkite/modules/io"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Client is the Starlark wrapper for EngineClient.
type Client struct {
	engine     *EngineClient
	socketPath string
}

var _ starlark.Value = (*Client)(nil)
var _ starlark.HasAttrs = (*Client)(nil)

func (c *Client) String() string {
	return fmt.Sprintf("<containers.Client endpoint=%q>", c.engine.Endpoint().String())
}
func (c *Client) Type() string          { return "containers.Client" }
func (c *Client) Freeze()               {}
func (c *Client) Truth() starlark.Bool  { return true }
func (c *Client) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: containers.Client") }

func (c *Client) AttrNames() []string {
	return []string{
		"build", "create", "delete", "endpoint", "exec", "get", "image_build",
		"image_inspect", "image_list", "image_pull", "image_remove", "images",
		"inspect", "list", "logs", "ping", "port", "prune", "pull", "remove",
		"restart", "rmi", "run", "socket", "start", "stop", "version", "wait",
	}
}

func (c *Client) Attr(name string) (starlark.Value, error) {
	if baseName, ok := strings.CutPrefix(name, "try_"); ok {
		val, err := c.Attr(baseName)
		if err != nil || val == nil {
			return nil, err
		}
		if b, ok := val.(*starlark.Builtin); ok {
			return libkite.TryWrap("containers.Client."+name, b), nil
		}
		return nil, nil
	}

	switch name {
	case "endpoint":
		return starlark.String(c.engine.Endpoint().String()), nil
	case "socket":
		return starlark.String(c.socketPath), nil
	case "ping":
		return starlark.NewBuiltin("Client.ping", c.pingFn), nil
	case "version":
		return starlark.NewBuiltin("Client.version", c.versionFn), nil
	case "create":
		return starlark.NewBuiltin("Client.create", c.createFn), nil
	case "run":
		return starlark.NewBuiltin("Client.run", c.runFn), nil
	case "get":
		return starlark.NewBuiltin("Client.get", c.getFn), nil
	case "list":
		return starlark.NewBuiltin("Client.list", c.listFn), nil
	case "start":
		return starlark.NewBuiltin("Client.start", c.startFn), nil
	case "stop":
		return starlark.NewBuiltin("Client.stop", c.stopFn), nil
	case "restart":
		return starlark.NewBuiltin("Client.restart", c.restartFn), nil
	case "delete":
		return starlark.NewBuiltin("Client.delete", c.deleteFn), nil
	case "remove":
		return starlark.NewBuiltin("Client.remove", c.deleteFn), nil
	case "wait":
		return starlark.NewBuiltin("Client.wait", c.waitFn), nil
	case "inspect":
		return starlark.NewBuiltin("Client.inspect", c.inspectFn), nil
	case "port":
		return starlark.NewBuiltin("Client.port", c.portFn), nil
	case "exec":
		return starlark.NewBuiltin("Client.exec", c.execFn), nil
	case "logs":
		return starlark.NewBuiltin("Client.logs", c.logsFn), nil
	case "image_list", "images":
		return starlark.NewBuiltin("Client."+name, c.imageListFn), nil
	case "image_pull", "pull":
		return starlark.NewBuiltin("Client."+name, c.imagePullFn), nil
	case "image_inspect":
		return starlark.NewBuiltin("Client.image_inspect", c.imageInspectFn), nil
	case "image_remove", "rmi":
		return starlark.NewBuiltin("Client."+name, c.imageRemoveFn), nil
	case "image_build", "build":
		return starlark.NewBuiltin("Client."+name, c.imageBuildFn), nil
	case "prune":
		return starlark.NewBuiltin("Client.prune", c.pruneFn), nil
	default:
		return nil, nil
	}
}

func (c *Client) getContext(thread *starlark.Thread) context.Context {
	ctx := context.Background()
	if rt := libkite.GetRuntime(thread); rt != nil {
		ctx = rt.Context()
	}
	return ctx
}

// pingFn implements client.ping() -> bool
func (c *Client) pingFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	ok, err := c.engine.Ping(ctx)
	if err != nil {
		return starlark.False, err
	}
	return starlark.Bool(ok), nil
}

// versionFn implements client.version() -> dict
func (c *Client) versionFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	vData, err := c.engine.Version(ctx)
	if err != nil {
		return nil, err
	}

	return startype.Go[any](vData).ToStarlarkValue()
}

// createFn implements client.create(image, name="", command=None, ports=None, volumes=None, env=None, network=None, cpu=None, memory=None, auto_remove=False) -> AttrDict
func (c *Client) createFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		image      string
		name       string
		commandVal starlark.Value
		portsVal   starlark.Value
		volumesVal starlark.Value
		envVal     starlark.Value
		network    string
		cpuVal     starlark.Value
		memoryVal  starlark.Value
		autoRemove bool
	)

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"image", &image,
		"name?", &name,
		"command?", &commandVal,
		"ports?", &portsVal,
		"volumes?", &volumesVal,
		"env?", &envVal,
		"network?", &network,
		"cpu?", &cpuVal,
		"memory?", &memoryVal,
		"auto_remove?", &autoRemove,
	); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "create", image); err != nil {
		return nil, err
	}

	cfg, err := c.buildContainerConfig(image, commandVal, portsVal, volumesVal, envVal, network, cpuVal, memoryVal, autoRemove)
	if err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	resp, err := c.engine.CreateContainer(ctx, name, cfg)
	if err != nil {
		return nil, err
	}

	return newContainerAttrDict(resp.ID, name, image, "created", nil), nil
}

// runFn implements client.run(image, name="", command=None, ports=None, volumes=None, env=None, detach=True, network=None, cpu=None, memory=None, auto_remove=False) -> AttrDict
func (c *Client) runFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		image      string
		name       string
		commandVal starlark.Value
		portsVal   starlark.Value
		volumesVal starlark.Value
		envVal     starlark.Value
		detach     = true
		network    string
		cpuVal     starlark.Value
		memoryVal  starlark.Value
		autoRemove bool
	)

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"image", &image,
		"name?", &name,
		"command?", &commandVal,
		"ports?", &portsVal,
		"volumes?", &volumesVal,
		"env?", &envVal,
		"detach?", &detach,
		"network?", &network,
		"cpu?", &cpuVal,
		"memory?", &memoryVal,
		"auto_remove?", &autoRemove,
	); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "run", image); err != nil {
		return nil, err
	}

	cfg, err := c.buildContainerConfig(image, commandVal, portsVal, volumesVal, envVal, network, cpuVal, memoryVal, autoRemove)
	if err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	resp, err := c.engine.CreateContainer(ctx, name, cfg)
	if err != nil {
		return nil, err
	}

	if err := c.engine.StartContainer(ctx, resp.ID); err != nil {
		return nil, fmt.Errorf("containers: failed to start container %s: %w", resp.ID, err)
	}
	status := "running"

	if !detach {
		exitCode, err := c.engine.WaitContainer(ctx, resp.ID, "not-running")
		if err != nil {
			return nil, err
		}
		if exitCode == 0 {
			status = "exited"
		} else {
			status = fmt.Sprintf("exited (%d)", exitCode)
		}
	}

	var ports map[string]any
	inspectData, err := c.engine.InspectContainer(ctx, resp.ID)
	if err == nil {
		if netSettings, ok := inspectData["NetworkSettings"].(map[string]any); ok {
			ports, _ = netSettings["Ports"].(map[string]any)
		}
		if state, ok := inspectData["State"].(map[string]any); ok {
			if st, ok := state["Status"].(string); ok && st != "" {
				status = st
			}
		}
		if n, ok := inspectData["Name"].(string); ok && n != "" {
			name = strings.TrimPrefix(n, "/")
		}
	}

	return newContainerAttrDict(resp.ID, name, image, status, ports), nil
}

// getFn implements client.get(id) -> AttrDict
func (c *Client) getFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var idOrName string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "id", &idOrName); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "get", idOrName); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	inspectData, err := c.engine.InspectContainer(ctx, idOrName)
	if err != nil {
		return nil, err
	}

	id, _ := inspectData["Id"].(string)
	name, _ := inspectData["Name"].(string)
	image := ""
	if cfg, ok := inspectData["Config"].(map[string]any); ok {
		image, _ = cfg["Image"].(string)
	}
	status := ""
	if state, ok := inspectData["State"].(map[string]any); ok {
		status, _ = state["Status"].(string)
	}
	var ports map[string]any
	if netSettings, ok := inspectData["NetworkSettings"].(map[string]any); ok {
		ports, _ = netSettings["Ports"].(map[string]any)
	}

	return newContainerAttrDict(id, name, image, status, ports), nil
}

// listFn implements client.list(all=False) -> list[AttrDict]
func (c *Client) listFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var all bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "all?", &all); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "list", ""); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	summaries, err := c.engine.ListContainers(ctx, all)
	if err != nil {
		return nil, err
	}

	containersList := make([]starlark.Value, 0, len(summaries))
	for _, s := range summaries {
		name := ""
		if len(s.Names) > 0 {
			name = s.Names[0]
		}
		containersList = append(containersList, newContainerAttrDict(s.ID, name, s.Image, s.State, nil))
	}

	return starlark.NewList(containersList), nil
}

// startFn implements client.start(target) -> None
func (c *Client) startFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "start", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.StartContainer(ctx, id); err != nil {
		return nil, err
	}

	if ad, ok := target.(*AttrDict); ok {
		ad.mu.Lock()
		ad.data["status"] = "running"
		ad.data["Status"] = "running"
		ad.mu.Unlock()
	}

	return starlark.None, nil
}

// stopFn implements client.stop(target, timeout=10) -> None
func (c *Client) stopFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	var timeout = 10
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target, "timeout?", &timeout); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "stop", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.StopContainer(ctx, id, timeout); err != nil {
		return nil, err
	}

	if ad, ok := target.(*AttrDict); ok {
		ad.mu.Lock()
		ad.data["status"] = "exited"
		ad.data["Status"] = "exited"
		ad.mu.Unlock()
	}

	return starlark.None, nil
}

// restartFn implements client.restart(target, timeout=10) -> None
func (c *Client) restartFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	var timeout = 10
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target, "timeout?", &timeout); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "restart", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.RestartContainer(ctx, id, timeout); err != nil {
		return nil, err
	}

	if ad, ok := target.(*AttrDict); ok {
		ad.mu.Lock()
		ad.data["status"] = "running"
		ad.data["Status"] = "running"
		ad.mu.Unlock()
	}

	return starlark.None, nil
}

// deleteFn implements client.delete(target, force=False, volumes=False) -> None
// Also aliased as client.remove(...)
func (c *Client) deleteFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	var force bool
	var volumes bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target, "force?", &force, "volumes?", &volumes); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "manage", "remove", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.RemoveContainer(ctx, id, force, volumes); err != nil {
		return nil, err
	}

	if ad, ok := target.(*AttrDict); ok {
		ad.mu.Lock()
		ad.data["status"] = "removed"
		ad.data["Status"] = "removed"
		ad.mu.Unlock()
	}

	return starlark.None, nil
}

// waitFn implements client.wait(target, condition="not-running") -> int
func (c *Client) waitFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	var condition = "not-running"
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target, "condition?", &condition); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "wait", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	code, err := c.engine.WaitContainer(ctx, id, condition)
	if err != nil {
		return starlark.MakeInt(code), err
	}
	return starlark.MakeInt(code), nil
}

// inspectFn implements client.inspect(target) -> AttrDict
func (c *Client) inspectFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "inspect", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	inspectData, err := c.engine.InspectContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	if ad, ok := target.(*AttrDict); ok {
		if state, ok := inspectData["State"].(map[string]any); ok {
			if status, ok := state["Status"].(string); ok {
				ad.mu.Lock()
				ad.data["status"] = status
				ad.data["Status"] = status
				ad.mu.Unlock()
			}
		}
	}

	return NewAttrDict(inspectData), nil
}

// portFn implements client.port(target, port) -> int
func (c *Client) portFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var target starlark.Value
	var portVal starlark.Value
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "target", &target, "port", &portVal); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	var portSpec string
	switch pv := portVal.(type) {
	case starlark.Int:
		portSpec = pv.String()
	case starlark.String:
		portSpec = string(pv)
	default:
		return nil, fmt.Errorf("containers: port must be an int or string, got %s", portVal.Type())
	}

	if err := libkite.Check(thread, "containers", "read", "port", id); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	inspectData, err := c.engine.InspectContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	normKey := NormalizePortKey(portSpec)
	netSettings, _ := inspectData["NetworkSettings"].(map[string]any)
	if netSettings == nil {
		return nil, fmt.Errorf("containers: no NetworkSettings found for container %s", id)
	}
	ports, _ := netSettings["Ports"].(map[string]any)
	if ports == nil {
		return nil, fmt.Errorf("containers: no Ports found for container %s", id)
	}

	rawBindings, ok := ports[normKey]
	if !ok || rawBindings == nil {
		return nil, fmt.Errorf("containers: port %q not exposed or bound on container %s", portSpec, id)
	}

	bindings, ok := rawBindings.([]any)
	if !ok || len(bindings) == 0 {
		return nil, fmt.Errorf("containers: port %q not bound on container %s", portSpec, id)
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

// execFn implements client.exec(target, command, env=None, user=None, workdir=None) -> ExecResult
func (c *Client) execFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		target     starlark.Value
		cmdVal     starlark.Value
		envVal     starlark.Value
		user       string
		workingDir string
	)

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"target", &target,
		"command", &cmdVal,
		"env?", &envVal,
		"user?", &user,
		"workdir?", &workingDir,
	); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "exec", id); err != nil {
		return nil, err
	}

	var cmd []string
	switch v := cmdVal.(type) {
	case starlark.String:
		cmd = []string{string(v)}
	case starlark.Indexable:
		cmd = make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			s, ok := starlark.AsString(v.Index(i))
			if !ok {
				return nil, fmt.Errorf("containers: exec command element %d must be string, got %s", i, v.Index(i).Type())
			}
			cmd[i] = s
		}
	default:
		return nil, fmt.Errorf("containers: exec command must be a string or list of strings, got %s", cmdVal.Type())
	}

	var env []string
	if envVal != nil && envVal != starlark.None {
		if envDict, ok := envVal.(*starlark.Dict); ok {
			for _, item := range envDict.Items() {
				k, ok1 := starlark.AsString(item[0])
				v, ok2 := starlark.AsString(item[1])
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("containers: exec env keys and values must be strings")
				}
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
		} else if envAttr, ok := envVal.(*AttrDict); ok {
			envAttr.mu.RLock()
			for k, v := range envAttr.data {
				env = append(env, fmt.Sprintf("%s=%v", k, v))
			}
			envAttr.mu.RUnlock()
		} else {
			return nil, fmt.Errorf("containers: exec env must be a dictionary, got %s", envVal.Type())
		}
	}

	cfg := ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
		Env:          env,
		User:         user,
		WorkingDir:   workingDir,
	}

	ctx := c.getContext(thread)
	return c.engine.Exec(ctx, id, cfg)
}

// logsFn implements client.logs(target, follow=False, tail="all", stdout=True, stderr=True) -> io.reader
func (c *Client) logsFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		target starlark.Value
		follow bool
		tail   = "all"
		stdout = true
		stderr = true
	)

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"target", &target,
		"follow?", &follow,
		"tail?", &tail,
		"stdout?", &stdout,
		"stderr?", &stderr,
	); err != nil {
		return nil, err
	}

	id, err := extractTargetID(target)
	if err != nil {
		return nil, err
	}

	name := extractTargetName(target, id)

	if err := libkite.Check(thread, "containers", "read", "logs", id); err != nil {
		return nil, err
	}

	opts := LogsOptions{
		Follow: follow,
		Tail:   tail,
		Stdout: stdout,
		Stderr: stderr,
	}

	ctx := c.getContext(thread)
	rc, err := c.engine.Logs(ctx, id, opts)
	if err != nil {
		return nil, err
	}

	return iomods.NewReaderWithCloser(rc, rc, fmt.Sprintf("%s.logs", name)), nil
}

// imageListFn implements client.image_list(all=False) -> list[AttrDict]
func (c *Client) imageListFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var all bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "all?", &all); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "read", "image_list", ""); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	images, err := c.engine.ListImages(ctx, all)
	if err != nil {
		return nil, err
	}

	items := make([]starlark.Value, 0, len(images))
	for _, img := range images {
		d := starlark.NewDict(8)
		d.SetKey(starlark.String("id"), starlark.String(img.ID))
		d.SetKey(starlark.String("Id"), starlark.String(img.ID))
		d.SetKey(starlark.String("size"), starlark.MakeInt64(img.Size))
		d.SetKey(starlark.String("Size"), starlark.MakeInt64(img.Size))
		d.SetKey(starlark.String("created"), starlark.MakeInt64(img.Created))
		d.SetKey(starlark.String("Created"), starlark.MakeInt64(img.Created))

		tags := make([]starlark.Value, len(img.RepoTags))
		for i, t := range img.RepoTags {
			tags[i] = starlark.String(t)
		}
		tagsList := starlark.NewList(tags)
		d.SetKey(starlark.String("repo_tags"), tagsList)
		d.SetKey(starlark.String("RepoTags"), tagsList)

		digests := make([]starlark.Value, len(img.RepoDigests))
		for i, dg := range img.RepoDigests {
			digests[i] = starlark.String(dg)
		}
		digestsList := starlark.NewList(digests)
		d.SetKey(starlark.String("repo_digests"), digestsList)
		d.SetKey(starlark.String("RepoDigests"), digestsList)

		labelsDict := starlark.NewDict(len(img.Labels))
		for k, v := range img.Labels {
			labelsDict.SetKey(starlark.String(k), starlark.String(v))
		}
		d.SetKey(starlark.String("labels"), labelsDict)
		d.SetKey(starlark.String("Labels"), labelsDict)

		items = append(items, d)
	}

	return starlark.NewList(items), nil
}

// imagePullFn implements client.image_pull(image, auth=None) -> None
func (c *Client) imagePullFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		image   string
		authVal starlark.Value
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"image", &image,
		"auth?", &authVal,
	); err != nil {
		return nil, err
	}

	if strings.TrimSpace(image) == "" {
		return nil, fmt.Errorf("containers: image cannot be empty")
	}

	if err := libkite.Check(thread, "containers", "write", "image_pull", image); err != nil {
		return nil, err
	}

	var authEncoded string
	if authVal != nil && authVal != starlark.None {
		switch a := authVal.(type) {
		case starlark.String:
			authEncoded = a.GoString()
		case *starlark.Dict:
			m := make(map[string]any)
			for _, item := range a.Items() {
				k, _ := starlark.AsString(item[0])
				v, _ := starlark.AsString(item[1])
				m[k] = v
			}
			data, err := json.Marshal(m)
			if err != nil {
				return nil, fmt.Errorf("containers: marshal auth: %w", err)
			}
			authEncoded = base64.StdEncoding.EncodeToString(data)
		default:
			return nil, fmt.Errorf("containers: auth must be dict or string, got %s", authVal.Type())
		}
	}

	ctx := c.getContext(thread)
	if err := c.engine.PullImage(ctx, image, authEncoded); err != nil {
		return nil, err
	}

	return starlark.None, nil
}

// imageInspectFn implements client.image_inspect(image) -> AttrDict
func (c *Client) imageInspectFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var image string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "image", &image); err != nil {
		return nil, err
	}

	if strings.TrimSpace(image) == "" {
		return nil, fmt.Errorf("containers: image cannot be empty")
	}

	if err := libkite.Check(thread, "containers", "read", "image_inspect", image); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	data, err := c.engine.InspectImage(ctx, image)
	if err != nil {
		return nil, err
	}

	return NewAttrDict(data), nil
}

// imageRemoveFn implements client.image_remove(image, force=False) -> None
func (c *Client) imageRemoveFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		image string
		force bool
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "image", &image, "force?", &force); err != nil {
		return nil, err
	}

	if strings.TrimSpace(image) == "" {
		return nil, fmt.Errorf("containers: image cannot be empty")
	}

	if err := libkite.Check(thread, "containers", "delete", "image_remove", image); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	if err := c.engine.RemoveImage(ctx, image, force); err != nil {
		return nil, err
	}

	return starlark.None, nil
}

func resolvePathString(val starlark.Value) (string, error) {
	if s, ok := starlark.AsString(val); ok {
		if strings.HasPrefix(s, "path(\"") && strings.HasSuffix(s, "\")") {
			s = strings.TrimSuffix(strings.TrimPrefix(s, "path(\""), "\")")
		}
		return s, nil
	}
	if ha, ok := val.(starlark.HasAttrs); ok {
		if attr, err := ha.Attr("string"); err == nil && attr != nil {
			if s, ok := starlark.AsString(attr); ok {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("expected string or fs.path, got %s", val.Type())
}

// imageBuildFn implements client.image_build(path, tag=None, dockerfile="Dockerfile") -> str
func (c *Client) imageBuildFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		pathVal    starlark.Value
		tag        string
		dockerfile = "Dockerfile"
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &pathVal, "tag?", &tag, "dockerfile?", &dockerfile); err != nil {
		return nil, err
	}

	dirPath, err := resolvePathString(pathVal)
	if err != nil {
		return nil, fmt.Errorf("containers: %w", err)
	}

	if strings.TrimSpace(dirPath) == "" {
		return nil, fmt.Errorf("containers: build path cannot be empty")
	}

	if err := libkite.Check(thread, "containers", "write", "image_build", dirPath); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	logs, err := c.engine.BuildImage(ctx, dirPath, tag, dockerfile)
	if err != nil {
		return nil, err
	}

	return starlark.String(logs), nil
}

// pruneFn implements client.prune(containers=True, volumes=False, images=False) -> dict
func (c *Client) pruneFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	pruneContainers := true
	pruneVolumes := false
	pruneImages := false

	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"containers?", &pruneContainers,
		"volumes?", &pruneVolumes,
		"images?", &pruneImages,
	); err != nil {
		return nil, err
	}

	if err := libkite.Check(thread, "containers", "write", "prune", ""); err != nil {
		return nil, err
	}

	ctx := c.getContext(thread)
	rep, err := c.engine.Prune(ctx, pruneContainers, pruneVolumes, pruneImages)
	if err != nil {
		return nil, err
	}

	d := starlark.NewDict(4)

	cDeleted := make([]starlark.Value, len(rep.ContainersDeleted))
	for i, id := range rep.ContainersDeleted {
		cDeleted[i] = starlark.String(id)
	}
	d.SetKey(starlark.String("containers_deleted"), starlark.NewList(cDeleted))

	vDeleted := make([]starlark.Value, len(rep.VolumesDeleted))
	for i, v := range rep.VolumesDeleted {
		vDeleted[i] = starlark.String(v)
	}
	d.SetKey(starlark.String("volumes_deleted"), starlark.NewList(vDeleted))

	iDeleted := make([]starlark.Value, len(rep.ImagesDeleted))
	for i, im := range rep.ImagesDeleted {
		iDeleted[i] = starlark.String(im)
	}
	d.SetKey(starlark.String("images_deleted"), starlark.NewList(iDeleted))

	d.SetKey(starlark.String("space_reclaimed"), starlark.MakeInt64(rep.SpaceReclaimed))

	return d, nil
}

func (c *Client) buildContainerConfig(
	image string,
	commandVal starlark.Value,
	portsVal starlark.Value,
	volumesVal starlark.Value,
	envVal starlark.Value,
	network string,
	cpuVal starlark.Value,
	memoryVal starlark.Value,
	autoRemove bool,
) (*ContainerConfig, error) {
	if strings.TrimSpace(image) == "" {
		return nil, fmt.Errorf("containers: image cannot be empty")
	}

	cfg := &ContainerConfig{
		Image: image,
		HostConfig: HostConfig{
			NetworkMode: network,
			AutoRemove:  autoRemove,
		},
	}

	// Command
	if commandVal != nil && commandVal != starlark.None {
		switch cmd := commandVal.(type) {
		case *starlark.List:
			for i := 0; i < cmd.Len(); i++ {
				s, ok := starlark.AsString(cmd.Index(i))
				if !ok {
					return nil, fmt.Errorf("containers: command list items must be strings")
				}
				cfg.Cmd = append(cfg.Cmd, s)
			}
		case starlark.Tuple:
			for _, item := range cmd {
				s, ok := starlark.AsString(item)
				if !ok {
					return nil, fmt.Errorf("containers: command tuple items must be strings")
				}
				cfg.Cmd = append(cfg.Cmd, s)
			}
		case starlark.String:
			cfg.Cmd = strings.Fields(cmd.GoString())
		default:
			return nil, fmt.Errorf("containers: command must be list, tuple, or string, got %s", commandVal.Type())
		}
	}

	// Env
	if envVal != nil && envVal != starlark.None {
		switch env := envVal.(type) {
		case *starlark.Dict:
			for _, item := range env.Items() {
				k, _ := starlark.AsString(item[0])
				v, _ := starlark.AsString(item[1])
				if k != "" {
					cfg.Env = append(cfg.Env, fmt.Sprintf("%s=%s", k, v))
				}
			}
		case *starlark.List:
			for i := 0; i < env.Len(); i++ {
				s, ok := starlark.AsString(env.Index(i))
				if !ok {
					return nil, fmt.Errorf("containers: env list items must be strings")
				}
				cfg.Env = append(cfg.Env, s)
			}
		default:
			return nil, fmt.Errorf("containers: env must be dict or list, got %s", envVal.Type())
		}
	}

	// Ports
	if portsVal != nil && portsVal != starlark.None {
		dict, ok := portsVal.(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("containers: ports must be a dict, got %s", portsVal.Type())
		}
		cfg.ExposedPorts = make(map[string]struct{})
		cfg.HostConfig.PortBindings = make(map[string][]PortBinding)

		for _, item := range dict.Items() {
			cPortStr, ok := starlark.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("containers: port key must be string (e.g. \"5432/tcp\"), got %s", item[0].Type())
			}
			normKey := NormalizePortKey(cPortStr)
			cfg.ExposedPorts[normKey] = struct{}{}

			var hostPortStr string
			if i, ok := item[1].(starlark.Int); ok {
				val, _ := i.Int64()
				if val > 0 {
					hostPortStr = strconv.FormatInt(val, 10)
				} else {
					hostPortStr = "" // 0 or empty means ephemeral port in Docker API
				}
			} else if s, ok := starlark.AsString(item[1]); ok {
				hostPortStr = s
			} else {
				return nil, fmt.Errorf("containers: port value must be int or string, got %s", item[1].Type())
			}

			cfg.HostConfig.PortBindings[normKey] = []PortBinding{
				{HostPort: hostPortStr},
			}
		}
	}

	// Volumes
	if volumesVal != nil && volumesVal != starlark.None {
		dict, ok := volumesVal.(*starlark.Dict)
		if !ok {
			return nil, fmt.Errorf("containers: volumes must be a dict, got %s", volumesVal.Type())
		}
		for _, item := range dict.Items() {
			hostPath, ok1 := starlark.AsString(item[0])
			containerPath, ok2 := starlark.AsString(item[1])
			if !ok1 || !ok2 {
				return nil, fmt.Errorf("containers: volume keys and values must be string paths")
			}
			if !strings.Contains(containerPath, ":") {
				containerPath += ":rw"
			}
			cfg.HostConfig.Binds = append(cfg.HostConfig.Binds, fmt.Sprintf("%s:%s", hostPath, containerPath))
		}
	}

	// CPU
	if cpuVal != nil && cpuVal != starlark.None {
		var cpuArg any
		if err := startype.Starlark(cpuVal).Go(&cpuArg); err != nil {
			return nil, fmt.Errorf("containers: parse cpu: %w", err)
		}
		nanoCPUs, err := ParseNanoCPUs(cpuArg)
		if err != nil {
			return nil, fmt.Errorf("containers: parse cpu: %w", err)
		}
		cfg.HostConfig.NanoCPUs = nanoCPUs
	}

	// Memory
	if memoryVal != nil && memoryVal != starlark.None {
		var memArg any
		if err := startype.Starlark(memoryVal).Go(&memArg); err != nil {
			return nil, fmt.Errorf("containers: parse memory: %w", err)
		}
		memBytes, err := ParseMemoryBytes(memArg)
		if err != nil {
			return nil, fmt.Errorf("containers: parse memory: %w", err)
		}
		cfg.HostConfig.Memory = memBytes
	}

	return cfg, nil
}
