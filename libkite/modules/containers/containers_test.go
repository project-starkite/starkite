package containers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
	"github.com/project-starkite/starkite/libkite/modules/containers"
)

// setupMockDaemon spins up a local Unix domain socket HTTP server for testing.
func setupMockDaemon(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()
	sockPath := fmt.Sprintf("/tmp/dk-%d.sock", time.Now().UnixNano()%10000000)
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen(unix, %s): %v", sockPath, err)
	}

	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()

	cleanup := func() {
		_ = server.Close()
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}

	return sockPath, cleanup
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		input       string
		wantScheme  string
		wantAddress string
		wantURL     string
		wantErr     bool
	}{
		{
			input:       "unix:///var/run/docker.sock",
			wantScheme:  "unix",
			wantAddress: "/var/run/docker.sock",
			wantURL:     "http://localhost",
		},
		{
			input:       "/custom/path/podman.sock",
			wantScheme:  "unix",
			wantAddress: "/custom/path/podman.sock",
			wantURL:     "http://localhost",
		},
		{
			input:       "tcp://127.0.0.1:2375",
			wantScheme:  "tcp",
			wantAddress: "127.0.0.1:2375",
			wantURL:     "http://127.0.0.1:2375",
		},
		{
			input:       "http://10.0.0.1:2375",
			wantScheme:  "tcp",
			wantAddress: "10.0.0.1:2375",
			wantURL:     "http://10.0.0.1:2375",
		},
		{
			input:       "npipe:////./pipe/docker_engine",
			wantScheme:  "npipe",
			wantAddress: "//./pipe/docker_engine",
			wantURL:     "http://localhost",
		},
		{
			input:   "",
			wantErr: true,
		},
		{
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			ep, err := containers.ParseEndpoint(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseEndpoint(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseEndpoint(%q) unexpected error: %v", tt.input, err)
			}
			if ep.Scheme != tt.wantScheme {
				t.Errorf("Scheme = %q, want %q", ep.Scheme, tt.wantScheme)
			}
			if ep.Address != tt.wantAddress {
				t.Errorf("Address = %q, want %q", ep.Address, tt.wantAddress)
			}
			if ep.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", ep.URL, tt.wantURL)
			}
		})
	}
}

func TestDiscoverEndpoint(t *testing.T) {
	// 1. Explicit override
	ep, err := containers.DiscoverEndpoint("unix:///tmp/custom.sock")
	if err != nil || ep.Address != "/tmp/custom.sock" {
		t.Fatalf("DiscoverEndpoint with override failed: %v, %+v", err, ep)
	}

	// 2. DOCKER_HOST env var
	t.Setenv("DOCKER_HOST", "tcp://192.168.1.100:2376")
	ep, err = containers.DiscoverEndpoint("")
	if err != nil || ep.Address != "192.168.1.100:2376" || ep.Scheme != "tcp" {
		t.Fatalf("DiscoverEndpoint with DOCKER_HOST failed: %v, %+v", err, ep)
	}

	// 3. Fallback when env is empty
	t.Setenv("DOCKER_HOST", "")
	ep, err = containers.DiscoverEndpoint("")
	if err != nil || ep.Address == "" {
		t.Fatalf("DiscoverEndpoint fallback failed: %v, %+v", err, ep)
	}
}

func TestCandidateSockets_ExcludesColimaAndOrbStack(t *testing.T) {
	candidates := containers.CandidateSockets()
	for _, c := range candidates {
		if strings.Contains(c, "colima") {
			t.Errorf("CandidateSockets() should not contain colima path: %s", c)
		}
		if strings.Contains(c, "orbstack") {
			t.Errorf("CandidateSockets() should not contain orbstack path: %s", c)
		}
	}
}

func TestEngineClient_PingAndVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/v1.45/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Version":    "27.1.1",
			"ApiVersion": "1.45",
			"Os":         "linux",
			"Arch":       "amd64",
		})
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep := containers.Endpoint{
		Scheme:  "unix",
		Address: sockPath,
		URL:     "http://localhost",
	}

	client, err := containers.NewEngineClient(ep, 5*time.Second)
	if err != nil {
		t.Fatalf("NewEngineClient: %v", err)
	}

	ctx := context.Background()

	// Test Ping
	ok, err := client.Ping(ctx)
	if err != nil || !ok {
		t.Fatalf("client.Ping() = %v, %v; want true, nil", ok, err)
	}

	// Test Version
	vData, err := client.Version(ctx)
	if err != nil {
		t.Fatalf("client.Version(): %v", err)
	}
	if vData["Version"] != "27.1.1" {
		t.Errorf("Version = %v, want 27.1.1", vData["Version"])
	}
	if vData["ApiVersion"] != "1.45" {
		t.Errorf("ApiVersion = %v, want 1.45", vData["ApiVersion"])
	}
}

func TestModuleFactoryMethod(t *testing.T) {
	m := containers.New()
	if m.FactoryMethod() != "config" {
		t.Errorf("FactoryMethod() = %q, want %q", m.FactoryMethod(), "config")
	}
}

func TestStarlarkClient_PingAndVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/v1.45/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Version":    "27.1.1",
			"ApiVersion": "1.45",
		})
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	starScript := fmt.Sprintf(`
load("containers", "containers")

def main():
    # Canonical constructor: containers.config()
    client = containers.config(host=%q)
    if not client.ping():
        fail("expected ping to return True")

    ver = client.version()
    if ver["Version"] != "27.1.1":
        fail("expected Version 27.1.1, got " + str(ver.get("Version")))
    if ver["ApiVersion"] != "1.45":
        fail("expected ApiVersion 1.45, got " + str(ver.get("ApiVersion")))

    if client.endpoint != %q:
        fail("expected endpoint " + %q + ", got " + client.endpoint)

    if client.socket != %q:
        fail("expected socket " + %q + ", got " + client.socket)

    # Alias constructor: containers.client()
    alias_client = containers.client(host=%q)
    if not alias_client.ping():
        fail("expected alias_client.ping() to return True")
`, "unix://"+sockPath, "unix://"+sockPath, "unix://"+sockPath, sockPath, sockPath, "unix://"+sockPath)

	rt, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.AllowAllPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rt.Close()

	if err := rt.Execute(context.Background(), starScript); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

func TestContainersPermissions(t *testing.T) {
	starScript := `
load("containers", "containers")
client = containers.config(host="unix:///tmp/fake.sock")
alias = containers.client(host="unix:///tmp/fake.sock")
`
	// 1. deny-all blocks containers.config()
	rtDeny, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.DenyAllPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New(deny-all): %v", err)
	}
	defer rtDeny.Close()

	if err := rtDeny.Execute(context.Background(), starScript); err == nil {
		t.Fatal("expected permission denial under deny-all, got nil")
	}

	// 2. allow-fs blocks containers.config()
	rtFS, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.AllowFSPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New(allow-fs): %v", err)
	}
	defer rtFS.Close()

	if err := rtFS.Execute(context.Background(), starScript); err == nil {
		t.Fatal("expected permission denial under allow-fs, got nil")
	}

	// 3. allow-local allows containers.config()
	rtLocal, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.AllowLocalPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New(allow-local): %v", err)
	}
	defer rtLocal.Close()

	if err := rtLocal.Execute(context.Background(), starScript); err != nil {
		t.Fatalf("expected containers.config to be allowed under allow-local, got: %v", err)
	}
}
