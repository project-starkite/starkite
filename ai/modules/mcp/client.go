package mcp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// defaultDiscoveryTimeout bounds the initial ListTools call made on first
// access to client.tools. Conservative default; users experiencing timeouts
// can add a call with their own context later.
const defaultDiscoveryTimeout = 30 * time.Second

// MCPClient is the Starlark-visible handle returned by mcp.connect().
//
// Attribute surface:
//
//	.tools — MCPToolsNamespace; iterable + dynamic attribute lookup
//	.call  — builtin: .call(name, **kwargs) → MCPResult
//	.close — builtin: .close() → None; idempotent
//
// Truthiness is False after close.
type MCPClient struct {
	session *mcpsdk.ClientSession
	// cmd is non-nil when the transport spawned a subprocess. We keep the
	// reference so close() can terminate the process regardless of what the
	// session's Close() does with the pipe.
	cmd    *exec.Cmd
	closed atomic.Bool

	// Lazy-discovery state, guarded by toolsOnce. toolsErr is non-nil if
	// discovery failed — retained so repeated accesses get consistent errors.
	toolsOnce    sync.Once
	toolsErr     error
	toolsByName  map[string]*cachedTool
	toolsInOrder []*cachedTool

	// The tools namespace is cached once at construction so repeated `.tools`
	// accesses return the same Starlark value.
	tools *MCPToolsNamespace
}

// cachedTool is the internal record of one discovered server tool.
// InputSchema is omitted for v1 — we don't expose schemas to scripts yet.
type cachedTool struct {
	name        string
	description string
	client      *MCPClient
}

var _ starlark.HasAttrs = (*MCPClient)(nil)

// newMCPClient constructs an MCPClient, wires its tools namespace, and
// installs a GC finalizer as a safety net in case the script forgets to
// call .close(). The finalizer runs only if the client is not already closed.
func newMCPClient(session *mcpsdk.ClientSession, cmd *exec.Cmd) *MCPClient {
	c := &MCPClient{session: session, cmd: cmd}
	c.tools = &MCPToolsNamespace{client: c}
	runtime.SetFinalizer(c, (*MCPClient).finalize)
	return c
}

func (c *MCPClient) String() string { return "<mcp.Client>" }
func (c *MCPClient) Type() string   { return "mcp.Client" }
func (c *MCPClient) Freeze()        {}
func (c *MCPClient) Truth() starlark.Bool {
	return starlark.Bool(!c.closed.Load())
}
func (c *MCPClient) Hash() (uint32, error) {
	return 0, fmt.Errorf("mcp.Client is unhashable")
}

func (c *MCPClient) Attr(name string) (starlark.Value, error) {
	switch name {
	case "tools":
		return c.tools, nil
	case "call":
		return starlark.NewBuiltin("mcp.Client.call", c.callBuiltin), nil
	case "close":
		return starlark.NewBuiltin("mcp.Client.close", c.closeBuiltin), nil
	}
	return nil, nil
}

func (c *MCPClient) AttrNames() []string { return []string{"tools", "call", "close"} }

// doClose closes the session and kills the subprocess (if any). Idempotent via
// CompareAndSwap; safe to call from both .close() and the finalizer.
func (c *MCPClient) doClose() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	var firstErr error
	if err := c.session.Close(); err != nil {
		firstErr = err
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	return firstErr
}

// finalize is the runtime.SetFinalizer callback. Exists purely as a safety net
// for scripts that forget to call .close(). Explicit close is still recommended.
func (c *MCPClient) finalize() {
	_ = c.doClose()
}

func (c *MCPClient) closeBuiltin(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := c.doClose(); err != nil {
		return nil, fmt.Errorf("mcp.Client.close: %w", err)
	}
	return starlark.None, nil
}

// callBuiltin implements client.call(name, **kwargs). We validate that the
// named tool exists (via cached discovery) so the user gets a clean
// "no such tool" error instead of a server-side MethodNotFound.
func (c *MCPClient) callBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("mcp.Client.call: expected 1 positional argument (tool name), got %d", len(args))
	}
	name, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("mcp.Client.call: tool name must be a string, got %s", args[0].Type())
	}
	if err := c.ensureDiscovered(); err != nil {
		return nil, fmt.Errorf("mcp.Client.call: %w", err)
	}
	if _, ok := c.toolsByName[name]; !ok {
		return nil, fmt.Errorf("mcp.Client.call: no such tool %q on server", name)
	}
	return c.invoke(name, kwargs)
}

// invoke is the shared tool-call path used by client.tools.X() and client.call(...).
// Converts kwargs → map[string]any via startype, dispatches, and wraps the
// result as *MCPResult.
func (c *MCPClient) invoke(name string, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.closed.Load() {
		return nil, errors.New("mcp client is closed")
	}

	argMap := make(map[string]any, len(kwargs))
	for _, kv := range kwargs {
		key, ok := kv[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("tool kwargs must have string keys, got %s", kv[0].Type())
		}
		v, err := startype.Starlark(kv[1]).ToGoValue()
		if err != nil {
			return nil, fmt.Errorf("arg %q: %w", string(key), err)
		}
		argMap[string(key)] = v
	}

	res, err := c.session.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: argMap,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp tool %q: %w", name, err)
	}
	return newMCPResult(res), nil
}

// ensureDiscovered lazily populates the tool cache. Both client.tools.X and
// client.call() funnel through this; the sync.Once makes it safe across
// concurrent accesses (though the module isn't goroutine-hardened overall).
func (c *MCPClient) ensureDiscovered() error {
	c.toolsOnce.Do(func() {
		if c.closed.Load() {
			c.toolsErr = errors.New("mcp client is closed")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), defaultDiscoveryTimeout)
		defer cancel()
		list, err := c.session.ListTools(ctx, nil)
		if err != nil {
			c.toolsErr = fmt.Errorf("list tools: %w", err)
			return
		}
		c.toolsByName = make(map[string]*cachedTool, len(list.Tools))
		c.toolsInOrder = make([]*cachedTool, 0, len(list.Tools))
		for _, t := range list.Tools {
			ct := &cachedTool{
				name:        t.Name,
				description: t.Description,
				client:      c,
			}
			c.toolsByName[t.Name] = ct
			c.toolsInOrder = append(c.toolsInOrder, ct)
		}
	})
	return c.toolsErr
}

// MCPToolsNamespace is the .tools value on an MCPClient. Implements both
// attribute lookup (for `client.tools.foo(...)` shortcuts) and iteration (for
// `for t in client.tools: ...` discovery loops).
type MCPToolsNamespace struct {
	client *MCPClient
}

var (
	_ starlark.HasAttrs = (*MCPToolsNamespace)(nil)
	_ starlark.Iterable = (*MCPToolsNamespace)(nil)
)

func (n *MCPToolsNamespace) String() string        { return "<mcp.Tools>" }
func (n *MCPToolsNamespace) Type() string          { return "mcp.Tools" }
func (n *MCPToolsNamespace) Freeze()               {}
func (n *MCPToolsNamespace) Truth() starlark.Bool  { return starlark.True }
func (n *MCPToolsNamespace) Hash() (uint32, error) { return 0, fmt.Errorf("mcp.Tools is unhashable") }

// Attr returns a bound *MCPTool when name matches a discovered tool AND is a
// valid Starlark identifier. Non-identifier names (hyphens, dots, etc.) are
// only reachable via client.call().
func (n *MCPToolsNamespace) Attr(name string) (starlark.Value, error) {
	if !isValidIdentifier(name) {
		return nil, nil
	}
	if err := n.client.ensureDiscovered(); err != nil {
		return nil, err
	}
	t, ok := n.client.toolsByName[name]
	if !ok {
		return nil, nil
	}
	return &MCPTool{cached: t}, nil
}

func (n *MCPToolsNamespace) AttrNames() []string {
	if err := n.client.ensureDiscovered(); err != nil {
		return nil
	}
	out := make([]string, 0, len(n.client.toolsByName))
	for name := range n.client.toolsByName {
		if isValidIdentifier(name) {
			out = append(out, name)
		}
	}
	return out
}

func (n *MCPToolsNamespace) Iterate() starlark.Iterator {
	if err := n.client.ensureDiscovered(); err != nil {
		return &errorIterator{err: err}
	}
	return &toolsIterator{tools: n.client.toolsInOrder}
}

// toolsIterator yields MCPTool values in the order returned by ListTools.
type toolsIterator struct {
	tools []*cachedTool
	i     int
}

func (it *toolsIterator) Next(p *starlark.Value) bool {
	if it.i >= len(it.tools) {
		return false
	}
	*p = &MCPTool{cached: it.tools[it.i]}
	it.i++
	return true
}

func (it *toolsIterator) Done() {}

// errorIterator is an empty iterator used when discovery fails inside Iterate().
// Starlark's iterator protocol has no error channel, so we yield nothing and
// callers see an empty loop. The error is still surfaced on subsequent
// client.tools.X accesses because ensureDiscovered is a sync.Once.
type errorIterator struct {
	err error
}

func (it *errorIterator) Next(*starlark.Value) bool { return false }
func (it *errorIterator) Done()                     {}

// MCPTool is the callable value returned by `client.tools.<name>` and yielded
// during iteration. Calling it invokes the underlying MCP tool; attributes
// expose metadata.
type MCPTool struct {
	cached *cachedTool
}

var (
	_ starlark.HasAttrs = (*MCPTool)(nil)
	_ starlark.Callable = (*MCPTool)(nil)
)

func (t *MCPTool) String() string { return fmt.Sprintf("<mcp.Tool name=%q>", t.cached.name) }
func (t *MCPTool) Type() string   { return "mcp.Tool" }
func (t *MCPTool) Freeze()        {}
func (t *MCPTool) Truth() starlark.Bool {
	return starlark.Bool(t.cached != nil && t.cached.client != nil)
}
func (t *MCPTool) Hash() (uint32, error) { return 0, fmt.Errorf("mcp.Tool is unhashable") }

// Name is part of the starlark.Callable interface — used for stack frames
// and "argument to non-function" error messages.
func (t *MCPTool) Name() string { return t.cached.name }

func (t *MCPTool) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(t.cached.name), nil
	case "description":
		return starlark.String(t.cached.description), nil
	}
	return nil, nil
}

func (t *MCPTool) AttrNames() []string { return []string{"name", "description"} }

func (t *MCPTool) CallInternal(_ *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("mcp tool %q: takes only keyword arguments, got %d positional", t.cached.name, len(args))
	}
	return t.cached.client.invoke(t.cached.name, kwargs)
}
