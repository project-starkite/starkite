package k8s

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite/modules/ssh"
)

func TestResolveTargetAddr(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		wantErr  bool
		errMatch string
	}{
		{
			name:  "standard https with port",
			input: "https://k8s-controller:6443",
			want:  "k8s-controller:6443",
		},
		{
			name:  "localhost https with port",
			input: "https://127.0.0.1:6443",
			want:  "127.0.0.1:6443",
		},
		{
			name:  "https default port 443",
			input: "https://api.k8s.example.com",
			want:  "api.k8s.example.com:443",
		},
		{
			name:  "http default port 80",
			input: "http://internal-control-plane",
			want:  "internal-control-plane:80",
		},
		{
			name:  "host and port without scheme",
			input: "k8s-controller:6443",
			want:  "k8s-controller:6443",
		},
		{
			name:  "ip and port without scheme",
			input: "172.20.206.160:6443",
			want:  "172.20.206.160:6443",
		},
		{
			name:     "empty URL",
			input:    "",
			wantErr:  true,
			errMatch: "empty API server address",
		},
		{
			name:     "unsupported scheme without port",
			input:    "ftp://control-plane",
			wantErr:  true,
			errMatch: "cannot infer port for scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTargetAddr(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMatch) {
					t.Fatalf("error %q does not match %q", err.Error(), tt.errMatch)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveTargetAddr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func createDummyKubeconfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	content := `apiVersion: v1
clusters:
- cluster:
    server: https://127.0.0.1:6443
    insecure-skip-tls-verify: true
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
kind: Config
preferences: {}
users:
- name: default
  user:
    token: dummy-token
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write dummy kubeconfig: %v", err)
	}
	return path
}

func TestK8sConfigWithJump(t *testing.T) {
	ts, err := ssh.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.AddPassword("bastionuser", "bastionpass")
	if err := ts.Start(); err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	defer ts.Close()

	kubeconfigPath := createDummyKubeconfig(t)

	mod := New()
	thread := &starlark.Thread{Name: "test-k8s-jump"}

	jumpDict := starlark.NewDict(5)
	jumpDict.SetKey(starlark.String("host"), starlark.String("127.0.0.1"))
	jumpDict.SetKey(starlark.String("port"), starlark.MakeInt(ts.Port()))
	jumpDict.SetKey(starlark.String("user"), starlark.String("bastionuser"))
	jumpDict.SetKey(starlark.String("password"), starlark.String("bastionpass"))
	jumpDict.SetKey(starlark.String("host_key_check"), starlark.False)

	val, err := mod.configFactory(thread, starlark.NewBuiltin("k8s.config", nil), nil, []starlark.Tuple{
		{starlark.String("kubeconfig"), starlark.String(kubeconfigPath)},
		{starlark.String("server"), starlark.String("https://k8s-controller:6443")},
		{starlark.String("tls_server_name"), starlark.String("k8s-controller")},
		{starlark.String("jump"), jumpDict},
	})
	if err != nil {
		t.Fatalf("k8s.config failed: %v", err)
	}

	client, ok := val.(*K8sClient)
	if !ok {
		t.Fatalf("k8s.config returned %T, want *K8sClient", val)
	}

	// Verify server endpoint override
	serverVal, err := client.serverEndpoint(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("client.server() error: %v", err)
	}
	if s, ok := serverVal.(starlark.String); !ok || string(s) != "https://k8s-controller:6443" {
		t.Errorf("client.server() = %v, want 'https://k8s-controller:6443'", serverVal)
	}

	// Verify tls_server_name override
	tlsVal, err := client.tlsServerName(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("client.tls_server_name() error: %v", err)
	}
	if s, ok := tlsVal.(starlark.String); !ok || string(s) != "k8s-controller" {
		t.Errorf("client.tls_server_name() = %v, want 'k8s-controller'", tlsVal)
	}

	// Verify tunnel dialer was configured and points to k8s-controller:6443
	if client.Dialer() == nil {
		t.Fatal("expected non-nil Dialer() on K8sClient")
	}
	if client.Dialer().TargetAddr() != "k8s-controller:6443" {
		t.Errorf("Dialer().TargetAddr() = %q, want 'k8s-controller:6443'", client.Dialer().TargetAddr())
	}

	// Close client and verify clean teardown
	closeVal, err := client.closeClient(thread, nil, nil, nil)
	if err != nil {
		t.Fatalf("client.close() error: %v", err)
	}
	if closeVal != starlark.None {
		t.Errorf("client.close() = %v, want None", closeVal)
	}
}

func TestK8sConfigJumpValidation(t *testing.T) {
	kubeconfigPath := createDummyKubeconfig(t)
	mod := New()
	thread := &starlark.Thread{Name: "test-k8s-jump-validation"}

	// 1. Invalid jump type (not a dict)
	_, err := mod.configFactory(thread, starlark.NewBuiltin("k8s.config", nil), nil, []starlark.Tuple{
		{starlark.String("kubeconfig"), starlark.String(kubeconfigPath)},
		{starlark.String("jump"), starlark.String("invalid-string")},
	})
	if err == nil || !strings.Contains(err.Error(), "'jump' must be a dict") {
		t.Fatalf("expected 'jump' must be a dict error, got: %v", err)
	}

	// 2. Missing required host in jump dict
	invalidJump := starlark.NewDict(1)
	invalidJump.SetKey(starlark.String("port"), starlark.MakeInt(22))
	_, err = mod.configFactory(thread, starlark.NewBuiltin("k8s.config", nil), nil, []starlark.Tuple{
		{starlark.String("kubeconfig"), starlark.String(kubeconfigPath)},
		{starlark.String("jump"), invalidJump},
	})
	if err == nil || !strings.Contains(err.Error(), "'host' is required") {
		t.Fatalf("expected 'host' is required error, got: %v", err)
	}

	// 3. None jump is allowed (noop)
	val, err := mod.configFactory(thread, starlark.NewBuiltin("k8s.config", nil), nil, []starlark.Tuple{
		{starlark.String("kubeconfig"), starlark.String(kubeconfigPath)},
		{starlark.String("jump"), starlark.None},
	})
	if err != nil {
		t.Fatalf("k8s.config with jump=None failed: %v", err)
	}
	client := val.(*K8sClient)
	if client.Dialer() != nil {
		t.Fatalf("expected nil Dialer() when jump=None, got %v", client.Dialer())
	}
}
