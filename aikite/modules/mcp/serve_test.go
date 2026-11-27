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

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestServe_ToolsMustBeFuncOrTool(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="x", tools=[42])`, globals)
	if err == nil || !strings.Contains(err.Error(), "function or mcp.Tool") {
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

	// Capture stderr around the call to inspect the startup log without
	// polluting test output.
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

func TestServe_HardeningAndStdioIsolation(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// Port > 65535 rejected.
	if _, err := starlark.ExecFile(thread, "t.star", `mcp.serve(name="x", port=70000)`, globals); err == nil || !strings.Contains(err.Error(), "65535") {
		t.Errorf("expected out-of-range port error, got %v", err)
	}

	// Non-slash path rejected before http.ServeMux registration.
	if _, err := starlark.ExecFile(thread, "t.star", `mcp.serve(name="x", port=8080, path="no-leading-slash")`, globals); err == nil || !strings.Contains(err.Error(), "must start with '/'") {
		t.Errorf("expected path prefix error, got %v", err)
	}

	// DryRun returns None without starting server or requiring runtime.
	mDry := New()
	dryGlobals, _ := mDry.Load(&libkite.ModuleConfig{DryRun: true})
	if _, err := starlark.ExecFile(thread, "t.star", `mcp.serve(name="x", port=8080)`, dryGlobals); err != nil {
		t.Errorf("DryRun serve failed: %v", err)
	}

	// Occupied port returns bind error synchronously without printing startup log.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	occupiedPort := ln.Addr().(*net.TCPAddr).Port

	origStderr := os.Stderr
	errR, errW, _ := os.Pipe()
	os.Stderr = errW
	rt := newTestRuntime(t)
	srv, err := buildServer("test", "0.1.0", nil, nil, nil, rt)
	if err != nil {
		os.Stderr = origStderr
		t.Fatal(err)
	}
	bindErr := runHTTPServerCtx(context.Background(), srv, httpOpts{host: "127.0.0.1", port: occupiedPort, path: "/"})
	errW.Close()
	os.Stderr = origStderr
	errLog, _ := io.ReadAll(errR)
	if bindErr == nil {
		t.Fatal("expected bind error on occupied port")
	}
	if strings.Contains(string(errLog), "listening on") {
		t.Errorf("startup log emitted despite bind failure: %q", string(errLog))
	}

	// Stdio mode diverts os.Stdout to os.Stderr and routes tool thread.Print to os.Stderr.
	tools := buildToolFromSource(t, `
def noisy():
    print("script print")
    return "ok"
`, "noisy")
	stdioSrv, err := buildServerWithOptions("test-stdio", "0.1.0", tools, nil, nil, rt, true)
	if err != nil {
		t.Fatal(err)
	}
	stdioSrv.AddTool(&mcpsdk.Tool{Name: "probe_stdout", InputSchema: map[string]any{"type": "object"}},
		func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			_, _ = os.Stdout.WriteString("leaked stdout\n")
			return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "probed"}}}, nil
		})

	c2sR, c2sW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	s2cR, s2cW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR2, errW2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	origStdin := os.Stdin
	origStdout := os.Stdout
	os.Stdin = c2sR
	os.Stdout = s2cW
	os.Stderr = errW2
	defer func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	srvCtx, srvCancel := context.WithCancel(context.Background())
	srvDone := make(chan error, 1)
	go func() {
		srvDone <- runStdioServer(srvCtx, stdioSrv)
	}()

	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-stdio-client"}, nil)
	connCtx, connCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer connCancel()
	sess, err := sdkClient.Connect(connCtx, &mcpsdk.IOTransport{Reader: s2cR, Writer: c2sW}, nil)
	if err != nil {
		srvCancel()
		_ = c2sW.Close()
		<-srvDone
		t.Fatalf("stdio client Connect: %v", err)
	}

	if res, err := sess.CallTool(connCtx, &mcpsdk.CallToolParams{Name: "noisy"}); err != nil || res.IsError {
		t.Fatalf("call noisy failed: res=%+v err=%v", res, err)
	}
	if res, err := sess.CallTool(connCtx, &mcpsdk.CallToolParams{Name: "probe_stdout"}); err != nil || res.IsError {
		t.Fatalf("call probe_stdout failed: res=%+v err=%v", res, err)
	}

	_ = sess.Close()
	srvCancel()
	_ = s2cW.Close()
	if err := <-srvDone; err != nil {
		t.Fatalf("runStdioServer: %v", err)
	}
	_ = errW2.Close()
	os.Stdin = origStdin
	os.Stdout = origStdout
	os.Stderr = origStderr

	capturedStderr, _ := io.ReadAll(errR2)
	if !strings.Contains(string(capturedStderr), "script print") {
		t.Errorf("expected stdio tool print on stderr, got: %q", string(capturedStderr))
	}
	if !strings.Contains(string(capturedStderr), "leaked stdout") {
		t.Errorf("expected diverted stdout write on stderr, got: %q", string(capturedStderr))
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
