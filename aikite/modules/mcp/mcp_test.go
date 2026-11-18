package mcp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestLoad_RegistersBuiltins(t *testing.T) {
	m := New()
	globals, err := m.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	mcpMod, ok := globals["mcp"]
	if !ok {
		t.Fatalf("expected 'mcp' in globals")
	}
	for _, name := range []string{"serve", "connect"} {
		val, err := mcpMod.(starlark.HasAttrs).Attr(name)
		if err != nil || val == nil {
			t.Errorf("mcp.%s missing: err=%v val=%v", name, err, val)
		}
	}
}

func TestServe_RejectsPositional(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star", `mcp.serve("x")`, globals)
	if err == nil || !strings.Contains(err.Error(), "keyword") {
		t.Errorf("expected kwargs-only error, got %v", err)
	}
}

func TestServe_RequiresRuntime(t *testing.T) {
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}
	_, err := starlark.ExecFile(thread, "t.star", `mcp.serve()`, globals)
	if err == nil || !strings.Contains(err.Error(), "no runtime available in thread") {
		t.Errorf("expected no-runtime error, got %v", err)
	}
}

func TestMCPServe_TLSCertPermissionGate(t *testing.T) {
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"mcp.server"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}

	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test-mcp-tls-perm"}
	libkite.SetPermissions(thread, checker)

	_, err = starlark.ExecFile(thread, "t.star",
		`mcp.serve(name="srv", port=8443, tls_cert="/tmp/cert.pem", tls_key="/tmp/key.pem")`, globals)
	if err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "fs.read") {
		t.Errorf("expected fs.read PermissionError for tls_cert, got %v", err)
	}
}

func TestMCPConnect_PermissionGateBeforeExecAndEmptyCommandRejected(t *testing.T) {
	if _, _, err := inferTransport(starlark.NewList([]starlark.Value{starlark.String("")})); err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected empty executable rejection, got %v", err)
	}

	// mcp.client check must run before os.exec permission checks.
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   []string{"os.exec"},
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := New()
	globals, _ := m.Load(&libkite.ModuleConfig{Env: map[string]string{"PATH": t.TempDir(), "MCP_TEST": "1"}})
	thread := &starlark.Thread{Name: "test-mcp-connect-gate"}
	libkite.SetPermissions(thread, checker)

	_, err = starlark.ExecFile(thread, "t.star", `mcp.connect(["echo", "hi"])`, globals)
	if err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "mcp.client") {
		t.Errorf("expected mcp.client PermissionError before exec, got %v", err)
	}

	// Verify buildTransportWithConfig scrubs ambient host vars and applies ModuleConfig.Env.
	t.Setenv("HOST_CANARY_MCP", "secret_token")
	if runtime.GOOS != "windows" {
		binDir := t.TempDir()
		tool := filepath.Join(binDir, "fake-mcp")
		_ = os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0755)
		_, cmd, err := buildTransportWithConfig(thread, []string{"fake-mcp"}, "", &libkite.ModuleConfig{
			Env: map[string]string{"PATH": binDir, "MCP_ONLY": "yes"},
		})
		if err != nil {
			t.Fatalf("buildTransportWithConfig isolated failed: %v", err)
		}
		joined := strings.Join(cmd.Env, "\n")
		if strings.Contains(joined, "HOST_CANARY_MCP") || !strings.Contains(joined, "MCP_ONLY=yes") {
			t.Errorf("unexpected isolated cmd.Env: %v", cmd.Env)
		}

		// Even without ModuleConfig.Env, SanitizedBaselineEnv scrubs ambient host secrets.
		_, cmd2, err := buildTransportWithConfig(thread, []string{tool}, "", nil)
		if err != nil {
			t.Fatalf("buildTransportWithConfig baseline failed: %v", err)
		}
		if strings.Contains(strings.Join(cmd2.Env, "\n"), "HOST_CANARY_MCP") {
			t.Errorf("baseline cmd.Env leaked host secret: %v", cmd2.Env)
		}
	}
}

func TestMCPConnect_URLValidationAndRedirectGate(t *testing.T) {
	if _, _, err := parseTransportSpec(starlark.String("http://")); err == nil {
		t.Error("expected malformed URL without host to fail")
	}

	blockedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer blockedSrv.Close()

	allowedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blockedSrv.URL+"/mcp", http.StatusFound)
	}))
	defer allowedSrv.Close()

	allowedHost := strings.TrimPrefix(allowedSrv.URL, "http://")
	checker, err := libkite.NewPermissionChecker(&libkite.PermissionConfig{
		Allow:   libkite.SynthesizeNetworkRules([]string{allowedHost}),
		Default: libkite.DefaultDeny,
	})
	if err != nil {
		t.Fatal(err)
	}

	thread := &starlark.Thread{Name: "test-mcp-redirect"}
	libkite.SetPermissions(thread, checker)

	if err := checker.Check("mcp", "client", "connect", allowedSrv.URL+"/mcp"); err != nil {
		t.Fatalf("expected SynthesizeNetworkRules to allow %s: %v", allowedSrv.URL, err)
	}

	transport, _, err := buildTransportWithConfig(thread, nil, allowedSrv.URL+"/mcp", nil)
	if err != nil {
		t.Fatalf("buildTransportWithConfig failed: %v", err)
	}
	st := transport.(*mcpsdk.StreamableClientTransport)
	if st.HTTPClient == nil || st.HTTPClient.CheckRedirect == nil {
		t.Fatal("expected HTTPClient with CheckRedirect on StreamableClientTransport")
	}

	_, err = st.HTTPClient.Get(allowedSrv.URL + "/mcp")
	if err == nil || !strings.Contains(err.Error(), "permission denied") || !strings.Contains(err.Error(), "mcp.client") || !strings.Contains(err.Error(), blockedSrv.URL) {
		t.Errorf("expected CheckRedirect to block redirect to unpermitted host with PermissionError, got %v", err)
	}
}
