package containers

import (
	"context"
	"fmt"

	"github.com/project-starkite/starkite/libkite"
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
	return []string{"endpoint", "ping", "socket", "version"}
}

func (c *Client) Attr(name string) (starlark.Value, error) {
	switch name {
	case "endpoint":
		return starlark.String(c.engine.Endpoint().String()), nil
	case "socket":
		return starlark.String(c.socketPath), nil
	case "ping":
		return starlark.NewBuiltin("Client.ping", c.pingFn), nil
	case "version":
		return starlark.NewBuiltin("Client.version", c.versionFn), nil
	default:
		return nil, nil
	}
}

// pingFn implements client.ping() -> bool
func (c *Client) pingFn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs); err != nil {
		return nil, err
	}

	ctx := context.Background()
	if rt := libkite.GetRuntime(thread); rt != nil {
		ctx = rt.Context()
	}

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

	ctx := context.Background()
	if rt := libkite.GetRuntime(thread); rt != nil {
		ctx = rt.Context()
	}

	vData, err := c.engine.Version(ctx)
	if err != nil {
		return nil, err
	}

	return startype.Go[any](vData).ToStarlarkValue()
}
