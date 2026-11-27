package mcp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

const (
	defaultDiscoveryTimeout = 30 * time.Second
	defaultToolCallTimeout  = 30 * time.Second
)

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
	closed  atomic.Bool

	toolsMu      sync.Mutex
	toolsLoaded  bool
	toolsByName  map[string]*cachedTool
	toolsInOrder []*cachedTool

	// The tools namespace is cached once at construction so repeated `.tools`
	// accesses return the same Starlark value.
	tools *MCPToolsNamespace
}

// cachedTool is the internal record of one discovered server tool.
// InputSchema is omitted for v1 — schemas are not exposed to scripts yet.
type cachedTool struct {
	name        string
	description string
	client      *MCPClient
}

var _ starlark.HasAttrs = (*MCPClient)(nil)

// newMCPClient constructs an MCPClient, wires its tools namespace, and
// installs a GC finalizer as a safety net in case the script forgets to
// call .close(). The finalizer runs only if the client is not already closed.
func newMCPClient(session *mcpsdk.ClientSession, _ *exec.Cmd) *MCPClient {
	c := &MCPClient{session: session}
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

// doClose closes the session cleanly. CommandTransport reaps any owned subprocess.
func (c *MCPClient) doClose() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	return c.session.Close()
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
func (c *MCPClient) callBuiltin(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("mcp.Client.call: expected 1 positional argument (tool name), got %d", len(args))
	}
	name, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("mcp.Client.call: tool name must be a string, got %s", args[0].Type())
	}
	if err := c.ensureDiscoveredWithThread(thread); err != nil {
		return nil, fmt.Errorf("mcp.Client.call: %w", err)
	}
	if _, ok := c.lookupTool(name); !ok {
		return nil, fmt.Errorf("mcp.Client.call: no such tool %q on server", name)
	}
	return c.invoke(thread, name, kwargs)
}

// invoke is the shared tool-call path used by client.tools.X() and client.call(...).
// Converts kwargs → map[string]any via startype, dispatches, and wraps the
// result as *MCPResult.
func (c *MCPClient) invoke(thread *starlark.Thread, name string, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.closed.Load() {
		return nil, errors.New("mcp client is closed")
	}

	if err := libkite.Check(thread, "mcp", "client", "tool_invoke", name); err != nil {
		return nil, err
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

	callCtx := libkite.ThreadContext(thread)
	var cancel context.CancelFunc
	if _, hasDeadline := callCtx.Deadline(); !hasDeadline {
		callCtx, cancel = context.WithTimeout(callCtx, defaultToolCallTimeout)
		defer cancel()
	}
	res, err := c.session.CallTool(callCtx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: argMap,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp tool %q: %w", name, err)
	}
	return newMCPResult(res), nil
}

func (c *MCPClient) ensureDiscovered() error {
	return c.ensureDiscoveredWithThread(nil)
}

func (c *MCPClient) ensureDiscoveredWithThread(thread *starlark.Thread) error {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()

	if c.toolsLoaded {
		return nil
	}
	if c.closed.Load() {
		return errors.New("mcp client is closed")
	}
	ctx, cancel := context.WithTimeout(libkite.ThreadContext(thread), defaultDiscoveryTimeout)
	defer cancel()
	list, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	toolsByName := make(map[string]*cachedTool, len(list.Tools))
	toolsInOrder := make([]*cachedTool, 0, len(list.Tools))
	for _, t := range list.Tools {
		ct := &cachedTool{
			name:        t.Name,
			description: t.Description,
			client:      c,
		}
		toolsByName[t.Name] = ct
		toolsInOrder = append(toolsInOrder, ct)
	}
	c.toolsByName = toolsByName
	c.toolsInOrder = toolsInOrder
	c.toolsLoaded = true
	return nil
}

func (c *MCPClient) lookupTool(name string) (*cachedTool, bool) {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()
	t, ok := c.toolsByName[name]
	return t, ok
}

func (c *MCPClient) listValidAttrNames() []string {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()
	out := make([]string, 0, len(c.toolsByName))
	for name := range c.toolsByName {
		if isValidIdentifier(name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

func (c *MCPClient) listToolsSnapshot() []*cachedTool {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()
	return slices.Clone(c.toolsInOrder)
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
	t, ok := n.client.lookupTool(name)
	if !ok {
		return nil, nil
	}
	return &MCPTool{cached: t}, nil
}

func (n *MCPToolsNamespace) AttrNames() []string {
	if err := n.client.ensureDiscovered(); err != nil {
		return nil
	}
	return n.client.listValidAttrNames()
}

func (n *MCPToolsNamespace) Iterate() starlark.Iterator {
	if err := n.client.ensureDiscovered(); err != nil {
		return &errorIterator{err: err}
	}
	return &toolsIterator{tools: n.client.listToolsSnapshot()}
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
// Starlark's iterator protocol has no error channel; this yields nothing and
// callers see an empty loop. The error still surfaces on subsequent
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

func (t *MCPTool) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, fmt.Errorf("mcp tool %q: takes only keyword arguments, got %d positional", t.cached.name, len(args))
	}
	return t.cached.client.invoke(thread, t.cached.name, kwargs)
}
