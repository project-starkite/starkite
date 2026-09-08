package containers_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
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

main()
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

func TestLifecyclePermissions(t *testing.T) {
	mux := http.NewServeMux()
	const cID = "c-perm-test"
	mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(containers.CreateContainerResponse{ID: cID})
	})
	mux.HandleFunc("/v1.45/containers/"+cID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+cID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	// 1. allow-local permits create, start, remove
	rtLocal, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.AllowLocalPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rtLocal.Close()

	script := fmt.Sprintf(`
load("containers", "containers")
def main():
    c = containers.config(host=%q)
    box = c.create(image="alpine")
    box.start()
    box.remove(force=True)

main()
`, "unix://"+sockPath)

	if err := rtLocal.Execute(context.Background(), script); err != nil {
		t.Fatalf("expected lifecycle to succeed under allow-local, got: %v", err)
	}

	// 2. Denying containers.manage blocks remove
	rtDenyManage, err := libkite.New(&libkite.Config{
		Registry: loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: &libkite.PermissionConfig{
			Allow:   []string{"containers.connect", "containers.write", "containers.read"},
			Deny:    []string{"containers.manage"},
			Default: libkite.DefaultDeny,
		},
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rtDenyManage.Close()

	if err := rtDenyManage.Execute(context.Background(), script); err == nil {
		t.Fatal("expected permission denial for containers.manage, got nil")
	}
}

func TestSchemas_Parsing(t *testing.T) {
	// Memory
	m, err := containers.ParseMemoryBytes("512m")
	if err != nil || m != 512*1024*1024 {
		t.Fatalf("ParseMemoryBytes(512m) = %d, %v; want %d", m, err, 512*1024*1024)
	}
	m, err = containers.ParseMemoryBytes("1gb")
	if err != nil || m != 1024*1024*1024 {
		t.Fatalf("ParseMemoryBytes(1gb) = %d, %v; want %d", m, err, 1024*1024*1024)
	}
	m, err = containers.ParseMemoryBytes(1024)
	if err != nil || m != 1024 {
		t.Fatalf("ParseMemoryBytes(1024) = %d, %v; want 1024", m, err)
	}

	// CPU
	c, err := containers.ParseNanoCPUs(0.5)
	if err != nil || c != 500_000_000 {
		t.Fatalf("ParseNanoCPUs(0.5) = %d, %v; want 500000000", c, err)
	}
	c, err = containers.ParseNanoCPUs("1.5")
	if err != nil || c != 1_500_000_000 {
		t.Fatalf("ParseNanoCPUs(1.5) = %d, %v; want 1500000000", c, err)
	}

	// Port normalization
	if p := containers.NormalizePortKey("80"); p != "80/tcp" {
		t.Fatalf("NormalizePortKey(80) = %q, want 80/tcp", p)
	}
	if p := containers.NormalizePortKey("53/udp"); p != "53/udp" {
		t.Fatalf("NormalizePortKey(53/udp) = %q, want 53/udp", p)
	}
}

func TestEngineClient_Lifecycle(t *testing.T) {
	mux := http.NewServeMux()

	const containerID = "c12345678901234567890"

	mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var cfg containers.ContainerConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if cfg.Image != "redis:alpine" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(containers.CreateContainerResponse{
			ID: containerID,
		})
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "10" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/restart", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/wait", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ContainerWaitResponse{
			StatusCode: 0,
		})
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := containers.ContainerInspectResponse{
			ID:   containerID,
			Name: "/test-redis",
		}
		resp.Config.Image = "redis:alpine"
		resp.State.Status = "running"
		resp.State.Running = true
		resp.NetworkSettings.Ports = map[string][]containers.PortBinding{
			"6379/tcp": {
				{HostIP: "0.0.0.0", HostPort: "32768"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]containers.ContainerSummary{
			{
				ID:    containerID,
				Names: []string{"/test-redis"},
				Image: "redis:alpine",
				State: "running",
			},
		})
	})

	mux.HandleFunc("/v1.45/containers/"+containerID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("force") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

	// 1. Create
	createResp, err := client.CreateContainer(ctx, "test-redis", &containers.ContainerConfig{
		Image: "redis:alpine",
		Cmd:   []string{"redis-server"},
	})
	if err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}
	if createResp.ID != containerID {
		t.Fatalf("CreateContainer ID = %q, want %q", createResp.ID, containerID)
	}

	// 2. Start
	if err := client.StartContainer(ctx, containerID); err != nil {
		t.Fatalf("StartContainer: %v", err)
	}

	// 3. Inspect
	inspectResp, err := client.InspectContainer(ctx, containerID)
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	name, _ := inspectResp["Name"].(string)
	state, _ := inspectResp["State"].(map[string]any)
	status, _ := state["Status"].(string)
	if name != "/test-redis" || status != "running" {
		t.Fatalf("InspectContainer unexpected: %+v", inspectResp)
	}

	// 4. List
	listResp, err := client.ListContainers(ctx, true)
	if err != nil || len(listResp) != 1 {
		t.Fatalf("ListContainers: %v, len=%d", err, len(listResp))
	}

	// 5. Restart
	if err := client.RestartContainer(ctx, containerID, 5); err != nil {
		t.Fatalf("RestartContainer: %v", err)
	}

	// 6. Stop
	if err := client.StopContainer(ctx, containerID, 10); err != nil {
		t.Fatalf("StopContainer: %v", err)
	}

	// 7. Wait
	code, err := client.WaitContainer(ctx, containerID, "not-running")
	if err != nil || code != 0 {
		t.Fatalf("WaitContainer = %d, %v; want 0, nil", code, err)
	}

	// 8. Remove
	if err := client.RemoveContainer(ctx, containerID, true, false); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}
}

func TestStarlark_ContainerLifecycle(t *testing.T) {
	mux := http.NewServeMux()
	const containerID = "c9876543210987654321"

	mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(containers.CreateContainerResponse{
			ID: containerID,
		})
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/restart", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/wait", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ContainerWaitResponse{StatusCode: 0})
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := containers.ContainerInspectResponse{
			ID:   containerID,
			Name: "/starlark-test",
		}
		resp.Config.Image = "postgres:16-alpine"
		resp.State.Status = "running"
		resp.NetworkSettings.Ports = map[string][]containers.PortBinding{
			"5432/tcp": {
				{HostIP: "0.0.0.0", HostPort: "32769"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/v1.45/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]containers.ContainerSummary{
			{ID: containerID, Names: []string{"/starlark-test"}, Image: "postgres:16-alpine", State: "running"},
		})
	})
	mux.HandleFunc("/v1.45/containers/"+containerID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	starScript := fmt.Sprintf(`
load("containers", "containers")

def main():
    client = containers.config(host=%q)

    # 1. client.create()
    box = client.create(
        image = "postgres:16-alpine",
        name = "starlark-test",
        ports = {"5432/tcp": 0},
        volumes = {"/tmp": "/tmp"},
        env = {"POSTGRES_PASSWORD": "secret"},
        cpu = 0.5,
        memory = "512m",
    )
    if box.id != %q:
        fail("expected id " + %q + ", got " + box.id)
    if box.status != "created":
        fail("expected created, got " + box.status)

    # 2. box.start()
    box.start()
    if box.status != "running":
        fail("expected running, got " + box.status)

    # 3. box.port()
    port = box.port("5432/tcp")
    if port != 32769:
        fail("expected port 32769, got " + str(port))

    # 4. box.inspect()
    info = box.inspect()
    if info["Id"] != %q:
        fail("inspect returned wrong id")

    # 5. box.restart()
    box.restart(timeout=5)

    # 6. box.stop()
    box.stop(timeout=5)

    # 7. box.wait()
    exit_code = box.wait()
    if exit_code != 0:
        fail("expected exit_code 0, got " + str(exit_code))

    # 8. box.remove()
    box.remove(force=True)

    # 9. client.run()
    runner = client.run(image="postgres:16-alpine", name="starlark-test", detach=True)
    if runner.id != %q:
        fail("run failed to return container")

    # 10. client.get()
    fetched = client.get(%q)
    if fetched.id != %q:
        fail("get failed to return container")

    # 11. client.list()
    all_boxes = client.list(all=True)
    if len(all_boxes) != 1:
        fail("expected 1 container in list, got " + str(len(all_boxes)))

main()
`, "unix://"+sockPath, containerID, containerID, containerID, containerID, containerID, containerID)

	rt, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.AllowAllPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rt.Close()

	if err := rt.Execute(context.Background(), starScript); err != nil {
		t.Fatalf("Starlark execution failed: %v", err)
	}
}

func makeDockerFrame(streamType byte, payload []byte) []byte {
	header := make([]byte, 8)
	header[0] = streamType
	binary.BigEndian.PutUint32(header[4:8], uint32(len(payload)))
	return append(header, payload...)
}

func TestDemuxStream(t *testing.T) {
	var raw bytes.Buffer
	raw.Write(makeDockerFrame(1, []byte("stdout line 1\n")))
	raw.Write(makeDockerFrame(2, []byte("stderr warning\n")))
	raw.Write(makeDockerFrame(1, []byte("stdout line 2\n")))

	var stdoutBuf, stderrBuf bytes.Buffer
	if err := containers.DemuxStream(&raw, &stdoutBuf, &stderrBuf); err != nil {
		t.Fatalf("DemuxStream failed: %v", err)
	}

	if got := stdoutBuf.String(); got != "stdout line 1\nstdout line 2\n" {
		t.Errorf("stdout = %q, want %q", got, "stdout line 1\nstdout line 2\n")
	}
	if got := stderrBuf.String(); got != "stderr warning\n" {
		t.Errorf("stderr = %q, want %q", got, "stderr warning\n")
	}

	// Raw unmultiplexed stream test
	rawUnmux := bytes.NewBufferString("plain text without framing")
	stdoutBuf.Reset()
	stderrBuf.Reset()
	if err := containers.DemuxStream(rawUnmux, &stdoutBuf, &stderrBuf); err != nil {
		t.Fatalf("DemuxStream raw failed: %v", err)
	}
	if got := stdoutBuf.String(); got != "plain text without framing" {
		t.Errorf("stdout = %q, want %q", got, "plain text without framing")
	}
}

func TestEngineClient_ExecAndLogs(t *testing.T) {
	mux := http.NewServeMux()
	const containerID = "c_exec_test"
	const execID = "e_12345"

	mux.HandleFunc("/v1.45/containers/"+containerID+"/exec", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(containers.ExecCreateResponse{ID: execID})
	})

	mux.HandleFunc("/v1.45/exec/"+execID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeDockerFrame(1, []byte("hello from exec\n")))
		_, _ = w.Write(makeDockerFrame(2, []byte("some stderr log\n")))
	})

	mux.HandleFunc("/v1.45/exec/"+execID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ExecInspectResponse{
			ID:       execID,
			Running:  false,
			ExitCode: 0,
		})
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeDockerFrame(1, []byte("log line 1\n")))
		_, _ = w.Write(makeDockerFrame(2, []byte("log err 2\n")))
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint("unix://" + sockPath)
	if err != nil {
		t.Fatalf("ParseEndpoint: %v", err)
	}
	client, err := containers.NewEngineClient(ep, 10*time.Second)
	if err != nil {
		t.Fatalf("NewEngineClient: %v", err)
	}

	ctx := context.Background()
	res, err := client.Exec(ctx, containerID, containers.ExecConfig{
		Cmd: []string{"echo", "hi"},
	})
	if err != nil {
		t.Fatalf("client.Exec: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if res.Stdout != "hello from exec\n" {
		t.Errorf("Stdout = %q, want %q", res.Stdout, "hello from exec\n")
	}
	if res.Stderr != "some stderr log\n" {
		t.Errorf("Stderr = %q, want %q", res.Stderr, "some stderr log\n")
	}

	// Logs
	rc, err := client.Logs(ctx, containerID, containers.LogsOptions{
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		t.Fatalf("client.Logs: %v", err)
	}
	defer rc.Close()
	logData, _ := io.ReadAll(rc)
	if string(logData) != "log line 1\nlog err 2\n" {
		t.Errorf("Logs = %q, want %q", string(logData), "log line 1\nlog err 2\n")
	}
}

func TestStarlark_ExecAndLogs(t *testing.T) {
	mux := http.NewServeMux()
	const containerID = "c_star_exec"
	const execID = "e_star_123"

	mux.HandleFunc("/v1.45/containers/"+containerID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id":   containerID,
			"Name": "/test-app",
			"Config": map[string]any{
				"Image": "alpine:latest",
			},
			"State": map[string]any{
				"Status":  "running",
				"Running": true,
			},
		})
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/exec", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(containers.ExecCreateResponse{ID: execID})
	})

	mux.HandleFunc("/v1.45/exec/"+execID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeDockerFrame(1, []byte("hello from exec\n")))
		_, _ = w.Write(makeDockerFrame(2, []byte("some stderr log\n")))
	})

	mux.HandleFunc("/v1.45/exec/"+execID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ExecInspectResponse{
			ID:       execID,
			Running:  false,
			ExitCode: 0,
		})
	})

	mux.HandleFunc("/v1.45/containers/"+containerID+"/logs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeDockerFrame(1, []byte("log line 1\n")))
		_, _ = w.Write(makeDockerFrame(2, []byte("log err 2\n")))
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	starScript := fmt.Sprintf(`
def main():
    client = containers.config(host=%q)
    box = client.get(%q)

    # 1. Exec
    res = box.exec(["echo", "hello"], env={"MY_VAR": "test"})
    if not res.ok:
        fail("expected res.ok == True")
    if res.exit_code != 0:
        fail("expected exit_code 0")
    if res.stdout != "hello from exec\n":
        fail("expected stdout, got " + res.stdout)
    if res.stderr != "some stderr log\n":
        fail("expected stderr, got " + res.stderr)

    # 2. Logs
    logs = box.logs()
    content = logs.text()
    if content != "log line 1\nlog err 2\n":
        fail("expected logs text, got " + content)

main()
`, "unix://"+sockPath, containerID)

	rt, err := libkite.New(&libkite.Config{
		Registry:    loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: libkite.AllowAllPermissions(),
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rt.Close()

	if err := rt.Execute(context.Background(), starScript); err != nil {
		t.Fatalf("Starlark execution failed: %v", err)
	}
}

func TestExecAndLogsPermissions(t *testing.T) {
	mux := http.NewServeMux()
	const containerID = "c_perm_stream"

	mux.HandleFunc("/v1.45/containers/"+containerID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id":   containerID,
			"Name": "/perm-box",
			"State": map[string]any{
				"Status": "running",
			},
		})
	})

	sockPath, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	// 1. Denying containers.write blocks exec
	rtDenyWrite, err := libkite.New(&libkite.Config{
		Registry: loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: &libkite.PermissionConfig{
			Allow:   []string{"containers.connect", "containers.read"},
			Deny:    []string{"containers.write"},
			Default: libkite.DefaultDeny,
		},
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rtDenyWrite.Close()

	scriptExec := fmt.Sprintf(`
load("containers", "containers")
def main():
    c = containers.config(host=%q)
    box = c.get(%q)
    box.exec(["echo", "blocked"])
main()
`, "unix://"+sockPath, containerID)

	if err := rtDenyWrite.Execute(context.Background(), scriptExec); err == nil {
		t.Fatal("expected box.exec() to fail when containers.write is denied")
	}

	// 2. Denying containers.read blocks logs
	rtDenyRead, err := libkite.New(&libkite.Config{
		Registry: loader.NewDefaultRegistry(&libkite.ModuleConfig{}),
		Permissions: &libkite.PermissionConfig{
			Allow:   []string{"containers.connect", "containers.write"},
			Deny:    []string{"containers.read"},
			Default: libkite.DefaultDeny,
		},
	})
	if err != nil {
		t.Fatalf("libkite.New: %v", err)
	}
	defer rtDenyRead.Close()

	scriptLogs := fmt.Sprintf(`
load("containers", "containers")
def main():
    c = containers.config(host=%q)
    box = c.get(%q)
    box.logs()
main()
`, "unix://"+sockPath, containerID)

	if err := rtDenyRead.Execute(context.Background(), scriptLogs); err == nil {
		t.Fatal("expected box.logs() to fail when containers.read is denied")
	}
}
