package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// defaultConnectTimeout caps the initial session handshake (Connect). After
// Connect returns, subsequent calls use their own untimed contexts unless the
// user's script layers one on.
const defaultConnectTimeout = 30 * time.Second

// connectBuiltin is the `mcp.connect(arg, timeout=?)` Starlark builtin.
//
//	arg is a list  → stdio subprocess via CommandTransport
//	arg is a URL   → HTTP via StreamableClientTransport
//	anything else  → error
//
// Returns an *MCPClient on success.
func (m *Module) connectBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("mcp.connect: expected 1 positional argument (command list or URL), got %d", len(args))
	}

	var p struct {
		Timeout string `name:"timeout"`
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("mcp.connect: %w", err)
	}
	timeout := defaultConnectTimeout
	if p.Timeout != "" {
		d, err := time.ParseDuration(p.Timeout)
		if err != nil {
			return nil, fmt.Errorf("mcp.connect: timeout: %w", err)
		}
		timeout = d
	}

	transport, subproc, err := inferTransport(args[0])
	if err != nil {
		return nil, fmt.Errorf("mcp.connect: %w", err)
	}

	if err := libkite.Check(thread, "mcp", "client", "connect", describeTransport(args[0])); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "starkite", Version: "0.1.0"}, nil)
	session, err := sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		// Connect may have failed after spawning the subprocess; kill it
		// so we don't leak a process.
		if subproc != nil && subproc.Process != nil {
			_ = subproc.Process.Kill()
			_ = subproc.Wait()
		}
		return nil, fmt.Errorf("mcp.connect: %w", err)
	}
	return newMCPClient(session, subproc), nil
}

// inferTransport picks an MCP transport based on the Starlark arg's type.
// Returns the transport, the spawned *exec.Cmd (nil for non-stdio), and an
// error for unsupported shapes.
func inferTransport(v starlark.Value) (mcpsdk.Transport, *exec.Cmd, error) {
	switch vv := v.(type) {
	case *starlark.List:
		argv, err := coerceCommandList(vv)
		if err != nil {
			return nil, nil, err
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		return &mcpsdk.CommandTransport{Command: cmd}, cmd, nil
	case starlark.Tuple:
		argv, err := coerceCommandList(vv)
		if err != nil {
			return nil, nil, err
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		return &mcpsdk.CommandTransport{Command: cmd}, cmd, nil
	case starlark.String:
		url := string(vv)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return nil, nil, fmt.Errorf("URL must start with http:// or https://, got %q", url)
		}
		return &mcpsdk.StreamableClientTransport{Endpoint: url}, nil, nil
	default:
		return nil, nil, fmt.Errorf("expected a command list or http(s) URL, got %s", v.Type())
	}
}

// coerceCommandList walks a Starlark iterable of strings into []string.
// Rejects empty lists and non-string elements.
func coerceCommandList(v starlark.Value) ([]string, error) {
	iter, ok := v.(starlark.Iterable)
	if !ok {
		return nil, fmt.Errorf("command list: expected iterable, got %s", v.Type())
	}
	it := iter.Iterate()
	defer it.Done()

	var out []string
	var elem starlark.Value
	for it.Next(&elem) {
		s, ok := starlark.AsString(elem)
		if !ok {
			return nil, fmt.Errorf("command list elements must be strings, got %s", elem.Type())
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("command list must not be empty")
	}
	return out, nil
}

// describeTransport returns a short string used as the permission "resource"
// for the mcp.connect check. For a list we show argv[0]; for a URL we show
// the URL itself. Gives operators something meaningful to match against in
// permission policies.
func describeTransport(v starlark.Value) string {
	switch vv := v.(type) {
	case *starlark.List:
		if vv.Len() > 0 {
			if s, ok := starlark.AsString(vv.Index(0)); ok {
				return s
			}
		}
		return "stdio"
	case starlark.Tuple:
		if len(vv) > 0 {
			if s, ok := starlark.AsString(vv[0]); ok {
				return s
			}
		}
		return "stdio"
	case starlark.String:
		return string(vv)
	}
	return ""
}
