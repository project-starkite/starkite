package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

// startInMemoryServer spins up an MCP server with the given tools registered
// via AddTool and an in-memory client transport connected to it. Returns the
// live MCPClient Starlark value and a cleanup func.
//
// Each tool handler ignores args and returns TextContent("called:<name>")
// unless overridden via the fakeResults map (keyed by tool name).
func startInMemoryServer(t *testing.T, tools []string, fakeResults map[string]*mcpsdk.CallToolResult) (*MCPClient, func()) {
	t.Helper()

	ct, st := mcpsdk.NewInMemoryTransports()
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-srv", Version: "0.1.0"}, nil)
	for _, name := range tools {
		nameCopy := name // capture
		server.AddTool(&mcpsdk.Tool{
			Name:        nameCopy,
			Description: "test tool " + nameCopy,
			InputSchema: &jsonschema.Schema{Type: "object"},
		}, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			if res, ok := fakeResults[nameCopy]; ok {
				return res, nil
			}
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "called:" + nameCopy}},
			}, nil
		})
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(context.Background(), st)
	}()

	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := sdkClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	client := newMCPClient(session, nil)
	cleanup := func() {
		_ = client.doClose()
		// Server.Run exits when the transport closes; wait briefly so we
		// don't leak goroutines across tests.
		select {
		case <-serverDone:
		case <-time.After(2 * time.Second):
		}
	}
	return client, cleanup
}

// execOnClient sets up a Starlark script environment with `client` pre-bound
// to the given *MCPClient and runs src. Returns the resulting globals.
func execOnClient(t *testing.T, client *MCPClient, src string) starlark.StringDict {
	t.Helper()
	globals := starlark.StringDict{"client": client}
	thread := &starlark.Thread{Name: "test"}
	got, err := starlark.ExecFile(thread, "t.star", src, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	return got
}

// --- Transport inference (unit tests, no network) ---

func TestInferTransport_List_ReturnsCommandTransport(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("echo"),
		starlark.String("hi"),
	})
	transport, cmd, err := inferTransport(list)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := transport.(*mcpsdk.CommandTransport); !ok {
		t.Errorf("transport = %T, want *CommandTransport", transport)
	}
	if cmd == nil || cmd.Path == "" {
		t.Errorf("cmd not populated: %+v", cmd)
	}
}

func TestInferTransport_URL_ReturnsStreamableTransport(t *testing.T) {
	transport, cmd, err := inferTransport(starlark.String("http://example.com/mcp"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if cmd != nil {
		t.Errorf("cmd = %+v, want nil for HTTP", cmd)
	}
	st, ok := transport.(*mcpsdk.StreamableClientTransport)
	if !ok {
		t.Fatalf("transport = %T, want *StreamableClientTransport", transport)
	}
	if st.Endpoint != "http://example.com/mcp" {
		t.Errorf("endpoint = %q", st.Endpoint)
	}
}

func TestInferTransport_BadURLScheme(t *testing.T) {
	_, _, err := inferTransport(starlark.String("ftp://x/y"))
	if err == nil || !strings.Contains(err.Error(), "http://") {
		t.Errorf("expected scheme error, got %v", err)
	}
}

func TestInferTransport_EmptyCommandList(t *testing.T) {
	_, _, err := inferTransport(starlark.NewList(nil))
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected empty-list error, got %v", err)
	}
}

func TestInferTransport_UnsupportedType(t *testing.T) {
	_, _, err := inferTransport(starlark.MakeInt(42))
	if err == nil || !strings.Contains(err.Error(), "command list or http(s) URL") {
		t.Errorf("expected unsupported-type error, got %v", err)
	}
}

func TestConnect_PositionalRequired(t *testing.T) {
	m := New()
	globals, _ := m.Load(&starbase.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star", `mcp.connect()`, globals)
	if err == nil || !strings.Contains(err.Error(), "expected 1 positional") {
		t.Errorf("expected arg-count error, got %v", err)
	}
}

func TestConnect_InvalidArgType(t *testing.T) {
	m := New()
	globals, _ := m.Load(&starbase.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star", `mcp.connect(42)`, globals)
	if err == nil || !strings.Contains(err.Error(), "command list or http(s) URL") {
		t.Errorf("expected type error, got %v", err)
	}
}

func TestConnect_InvalidTimeout(t *testing.T) {
	m := New()
	globals, _ := m.Load(&starbase.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.connect("http://x", timeout="bogus")`, globals)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got %v", err)
	}
}

// --- In-memory end-to-end tests ---

func TestClient_Tools_Iteration(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"alpha", "beta", "gamma"}, nil)
	defer cleanup()

	got := execOnClient(t, client, `
names = [t.name for t in client.tools]
count = len(names)
`)
	countVal := got["count"].(starlark.Int)
	if n, _ := countVal.Int64(); n != 3 {
		t.Errorf("count = %d, want 3", n)
	}
	// Assert the names are present (iteration order preserved from server).
	namesList := got["names"].(*starlark.List)
	seen := map[string]bool{}
	for i := 0; i < namesList.Len(); i++ {
		s, _ := starlark.AsString(namesList.Index(i))
		seen[s] = true
	}
	for _, want := range []string{"alpha", "beta", "gamma"} {
		if !seen[want] {
			t.Errorf("missing %q in iteration", want)
		}
	}
}

func TestClient_ToolsAttr_ValidIdentifier(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"greet"}, nil)
	defer cleanup()

	got := execOnClient(t, client, `
resp = client.tools.greet(name="world")
text = resp.text
`)
	if s, _ := starlark.AsString(got["text"]); s != "called:greet" {
		t.Errorf("text = %q", s)
	}
}

func TestClient_ToolsAttr_NonIdentifier_ReturnsNil(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"foo-bar"}, nil)
	defer cleanup()

	// The attribute lookup itself returns nil for non-identifier names.
	// In Starlark, this surfaces as "has no field or method 'foo_bar'"
	// at the getattr level. We can test the Attr method directly.
	if err := client.ensureDiscovered(); err != nil {
		t.Fatalf("ensureDiscovered: %v", err)
	}
	v, err := client.tools.Attr("foo-bar")
	if err != nil {
		t.Fatalf("Attr: %v", err)
	}
	if v != nil {
		t.Errorf("Attr('foo-bar') = %v, want nil", v)
	}
}

func TestClient_Call_HyphenatedName(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"foo-bar"}, nil)
	defer cleanup()

	got := execOnClient(t, client, `
resp = client.call("foo-bar", x=1)
text = resp.text
`)
	if s, _ := starlark.AsString(got["text"]); s != "called:foo-bar" {
		t.Errorf("text = %q", s)
	}
}

func TestClient_Call_MissingTool(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"known"}, nil)
	defer cleanup()

	_, err := starlark.ExecFile(
		&starlark.Thread{Name: "test"},
		"t.star",
		`client.call("nope")`,
		starlark.StringDict{"client": client},
	)
	if err == nil || !strings.Contains(err.Error(), "no such tool") {
		t.Errorf("expected no-such-tool error, got %v", err)
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	client, cleanup := startInMemoryServer(t, nil, nil)
	defer cleanup()

	if err := client.doClose(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := client.doClose(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

func TestClient_Truth_AfterClose(t *testing.T) {
	client, cleanup := startInMemoryServer(t, nil, nil)
	defer cleanup()

	if !bool(client.Truth()) {
		t.Errorf("Truth = False before close")
	}
	_ = client.doClose()
	if bool(client.Truth()) {
		t.Errorf("Truth = True after close")
	}
}

func TestClient_ToolResult_IsError(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"bad"}, map[string]*mcpsdk.CallToolResult{
		"bad": {
			IsError: true,
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "oops"}},
		},
	})
	defer cleanup()

	got := execOnClient(t, client, `
resp = client.tools.bad()
is_err = resp.is_error
err_text = resp.text
`)
	if b, ok := got["is_err"].(starlark.Bool); !ok || !bool(b) {
		t.Errorf("is_err = %v, want True", got["is_err"])
	}
	if s, _ := starlark.AsString(got["err_text"]); s != "oops" {
		t.Errorf("err_text = %q", s)
	}
}

func TestClient_Invoke_AfterClose_Errors(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"x"}, nil)
	defer cleanup()

	// Discover first (populates cache) to isolate the closed-check.
	if err := client.ensureDiscovered(); err != nil {
		t.Fatalf("ensureDiscovered: %v", err)
	}
	_ = client.doClose()

	_, err := client.invoke("x", nil)
	if err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected closed error, got %v", err)
	}
}

func TestClient_Discovery_LazyAndCached(t *testing.T) {
	client, cleanup := startInMemoryServer(t, []string{"a", "b"}, nil)
	defer cleanup()

	// First call triggers ListTools.
	if err := client.ensureDiscovered(); err != nil {
		t.Fatalf("first discovery: %v", err)
	}
	if len(client.toolsByName) != 2 {
		t.Errorf("toolsByName size = %d, want 2", len(client.toolsByName))
	}
	// Second call uses cached state — can't observe easily, but at
	// minimum must not error.
	if err := client.ensureDiscovered(); err != nil {
		t.Errorf("second discovery: %v", err)
	}
}

// Compile-time assertion that errorIterator satisfies starlark.Iterator.
var _ starlark.Iterator = (*errorIterator)(nil)

// silence "errors unused" if imports shift later
var _ = errors.New
