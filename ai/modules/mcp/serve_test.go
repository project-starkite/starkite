package mcp

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestServe_ToolsMustBeFuncOrTool(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", tools=[42])`, globals)
	if err == nil || !strings.Contains(err.Error(), "function or ai.Tool") {
		t.Errorf("expected type error, got %v", err)
	}
}

func TestBuildServer_RegistersTools(t *testing.T) {
	tools := buildToolFromSource(t, `
def hello(name = "world"):
    "Say hi."
    return "hi " + name
`, "hello")

	rt := newTestRuntime(t)
	server, err := buildServer("test-srv", "0.1.0", tools, nil, nil, rt)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
}

func TestBuildServer_NoTools_OK(t *testing.T) {
	rt := newTestRuntime(t)
	server, err := buildServer("empty-srv", "0.1.0", nil, nil, nil, rt)
	if err != nil {
		t.Fatalf("buildServer with no tools: %v", err)
	}
	if server == nil {
		t.Fatal("server is nil")
	}
}

func TestServe_ResourcesMustBeDict(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", resources=42)`, globals)
	if err == nil || !strings.Contains(err.Error(), "resources must be a dict") {
		t.Errorf("expected dict-required error, got %v", err)
	}
}

func TestServe_PromptsMustBeDict(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", prompts=42)`, globals)
	if err == nil || !strings.Contains(err.Error(), "prompts must be a dict") {
		t.Errorf("expected dict-required error, got %v", err)
	}
}

// --- HTTP-transport validation (Slice 2.4) ---

func TestServe_Port_RejectNegative(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", port=-1)`, globals)
	if err == nil || !strings.Contains(err.Error(), "port must be non-negative") {
		t.Errorf("expected negative-port error, got %v", err)
	}
}

func TestServe_TLS_CertWithoutKey_Errors(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", port=1, tls_cert="/tmp/cert.pem")`, globals)
	if err == nil || !strings.Contains(err.Error(), "tls_cert and tls_key must both be set") {
		t.Errorf("expected TLS pair error, got %v", err)
	}
}

func TestServe_TLS_KeyWithoutCert_Errors(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", port=1, tls_key="/tmp/key.pem")`, globals)
	if err == nil || !strings.Contains(err.Error(), "tls_cert and tls_key must both be set") {
		t.Errorf("expected TLS pair error, got %v", err)
	}
}

func TestServe_HTTPKwargs_WithoutPort_Error(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", host="0.0.0.0")`, globals)
	if err == nil || !strings.Contains(err.Error(), "require port") {
		t.Errorf("expected require-port error, got %v", err)
	}
}

// TestRunHTTPServerCtx_StartsAndShutsDown binds an ephemeral port, verifies the
// server comes up (HTTP 4xx for a GET with no MCP headers is fine — the
// handler responded, which proves it's listening), captures the startup log,
// and cancels cleanly via the context.
func TestRunHTTPServerCtx_StartsAndShutsDown(t *testing.T) {
	rt := newTestRuntime(t)
	server, err := buildServer("test", "0.1.0", nil, nil, nil, rt)
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}

	// Capture stderr around the call so we can inspect the startup log
	// without polluting test output.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use an OS-assigned free port: bind briefly, read the port, close,
	// then let the MCP server rebind. Small race window, acceptable for
	// a unit test.
	port, err := pickFreePort()
	if err != nil {
		t.Fatalf("pickFreePort: %v", err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runHTTPServerCtx(ctx, server, httpOpts{
			host: "127.0.0.1",
			port: port,
			path: "/",
		})
	}()

	// Poll until the server is accepting connections (up to ~2s).
	url := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	deadline := time.Now().Add(2 * time.Second)
	var reachable bool
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			reachable = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !reachable {
		cancel()
		<-serverDone
		w.Close()
		os.Stderr = origStderr
		t.Fatal("HTTP server never became reachable")
	}

	// Shut down and verify no error.
	cancel()
	select {
	case err := <-serverDone:
		if err != nil {
			t.Errorf("runHTTPServerCtx: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down within 3s")
	}

	// Inspect captured stderr.
	w.Close()
	os.Stderr = origStderr
	logBytes, _ := io.ReadAll(r)
	logLine := string(logBytes)
	if !strings.Contains(logLine, "mcp.serve: listening on http://127.0.0.1:") {
		t.Errorf("missing or malformed startup log: %q", logLine)
	}
	if !strings.Contains(logLine, "/") {
		t.Errorf("startup log missing path: %q", logLine)
	}
}

// pickFreePort asks the OS for a free TCP port on 127.0.0.1 by briefly binding
// and immediately closing a listener. Small race window between close and the
// real server rebinding, but acceptable for unit tests.
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
