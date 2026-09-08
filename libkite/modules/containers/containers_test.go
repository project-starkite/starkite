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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
	"github.com/project-starkite/starkite/libkite/modules/containers"
)

// setupMockDaemon spins up a local HTTP server for testing.
// On Windows (or systems where AF_UNIX is unavailable), it uses a TCP listener (127.0.0.1:0).
// On Unix, it creates a temporary Unix domain socket.
func setupMockDaemon(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()

	if runtime.GOOS == "windows" {
		return setupMockDaemonTCP(t, handler)
	}

	sockPath := fmt.Sprintf("/tmp/dk-%d.sock", time.Now().UnixNano()%10000000)
	_ = os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return setupMockDaemonTCP(t, handler)
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

	return "unix://" + sockPath, cleanup
}

func setupMockDaemonTCP(t *testing.T, handler http.Handler) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen(tcp, 127.0.0.1:0): %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(listener)
	}()
	cleanup := func() {
		_ = server.Close()
		_ = listener.Close()
	}
	return "tcp://" + listener.Addr().String(), cleanup
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

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint(host)
	if err != nil {
		t.Fatalf("ParseEndpoint: %v", err)
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

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint(host)
	if err != nil {
		t.Fatalf("ParseEndpoint: %v", err)
	}

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

main()
`, host, ep.String(), ep.String(), ep.Address, ep.Address)

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

	host, cleanup := setupMockDaemon(t, mux)
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
    c.start(box)
    c.delete(box, force=True)

main()
`, host)

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

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint(host)
	if err != nil {
		t.Fatalf("ParseEndpoint: %v", err)
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

	host, cleanup := setupMockDaemon(t, mux)
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

    # 2. client.start()
    client.start(box)
    if box.status != "running":
        fail("expected running, got " + box.status)

    # 3. client.port()
    port = client.port(box, "5432/tcp")
    if port != 32769:
        fail("expected port 32769, got " + str(port))

    # 4. client.inspect()
    info = client.inspect(box)
    if info["Id"] != %q:
        fail("inspect returned wrong id")

    # 5. client.restart()
    client.restart(box, timeout=5)

    # 6. client.stop()
    client.stop(box, timeout=5)

    # 7. client.wait()
    exit_code = client.wait(box)
    if exit_code != 0:
        fail("expected exit_code 0, got " + str(exit_code))

    # 8. client.delete() / client.remove()
    client.delete(box, force=True)

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
`, host, containerID, containerID, containerID, containerID, containerID, containerID)

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

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint(host)
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

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	starScript := fmt.Sprintf(`
def main():
    client = containers.config(host=%q)
    box = client.get(%q)

    # 1. Exec
    res = client.exec(box, ["echo", "hello"], env={"MY_VAR": "test"})
    if not res.ok:
        fail("expected res.ok == True")
    if res.exit_code != 0:
        fail("expected exit_code 0")
    if res.stdout != "hello from exec\n":
        fail("expected stdout, got " + res.stdout)
    if res.stderr != "some stderr log\n":
        fail("expected stderr, got " + res.stderr)

    # 2. Logs
    logs = client.logs(box)
    content = logs.text()
    if content != "log line 1\nlog err 2\n":
        fail("expected logs text, got " + content)

main()
`, host, containerID)

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

	host, cleanup := setupMockDaemon(t, mux)
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
`, host, containerID)

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
`, host, containerID)

	if err := rtDenyRead.Execute(context.Background(), scriptLogs); err == nil {
		t.Fatal("expected box.logs() to fail when containers.read is denied")
	}
}

func TestEngineClient_ImageAndPrune(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1.45/images/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]containers.ImageSummary{
			{
				ID:          "sha256:112233445566",
				RepoTags:    []string{"alpine:latest", "alpine:3.19"},
				RepoDigests: []string{"alpine@sha256:1234"},
				Created:     1700000000,
				Size:        7340032,
				Labels:      map[string]string{"env": "test"},
			},
		})
	})

	mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		img := r.URL.Query().Get("fromImage")
		if img == "error:bad" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"Pulling fs layer"}` + "\n"))
			_, _ = w.Write([]byte(`{"errorDetail":{"message":"manifest unknown"},"error":"manifest unknown"}` + "\n"))
			return
		}
		authHeader := r.Header.Get("X-Registry-Auth")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"Pulling from library/alpine","id":"latest"}` + "\n"))
		if authHeader != "" {
			_, _ = w.Write([]byte(`{"status":"Authenticated with registry"}` + "\n"))
		}
		_, _ = w.Write([]byte(`{"status":"Download complete","id":"layer1"}` + "\n"))
	})

	mux.HandleFunc("/v1.45/images/alpine:3.19", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/v1.45/containers/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ContainersPruneReport{
			ContainersDeleted: []string{"c_old1", "c_old2"},
			SpaceReclaimed:    2048,
		})
	})

	mux.HandleFunc("/v1.45/volumes/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.VolumesPruneReport{
			VolumesDeleted: []string{"vol_old1"},
			SpaceReclaimed: 4096,
		})
	})

	mux.HandleFunc("/v1.45/images/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ImagesPruneReport{
			ImagesDeleted: []containers.ImageDeletedItem{
				{Deleted: "sha256:oldimage1"},
			},
			SpaceReclaimed: 8192,
		})
	})

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint(host)
	if err != nil {
		t.Fatalf("ParseEndpoint: %v", err)
	}

	client, err := containers.NewEngineClient(ep, 5*time.Second)
	if err != nil {
		t.Fatalf("NewEngineClient: %v", err)
	}
	ctx := context.Background()

	// 1. ListImages
	images, err := client.ListImages(ctx, true)
	if err != nil || len(images) != 1 {
		t.Fatalf("ListImages failed: %v, len=%d", err, len(images))
	}
	if images[0].ID != "sha256:112233445566" || len(images[0].RepoTags) != 2 {
		t.Errorf("unexpected image: %+v", images[0])
	}

	// 2. PullImage success
	if err := client.PullImage(ctx, "alpine:latest", "base64auth"); err != nil {
		t.Fatalf("PullImage failed: %v", err)
	}

	// 3. PullImage failure via stream error
	if err := client.PullImage(ctx, "error:bad", ""); err == nil {
		t.Fatal("expected PullImage with error:bad to fail")
	}

	// 4. RemoveImage
	if err := client.RemoveImage(ctx, "alpine:3.19", true); err != nil {
		t.Fatalf("RemoveImage failed: %v", err)
	}

	// 5. Prune
	pruneRes, err := client.Prune(ctx, true, true, true)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if len(pruneRes.ContainersDeleted) != 2 {
		t.Errorf("unexpected ContainersDeleted: %v", pruneRes.ContainersDeleted)
	}
	if len(pruneRes.VolumesDeleted) != 1 {
		t.Errorf("unexpected VolumesDeleted: %v", pruneRes.VolumesDeleted)
	}
	if len(pruneRes.ImagesDeleted) != 1 {
		t.Errorf("unexpected ImagesDeleted: %v", pruneRes.ImagesDeleted)
	}
	expectedReclaimed := int64(2048 + 4096 + 8192)
	if pruneRes.SpaceReclaimed != expectedReclaimed {
		t.Errorf("SpaceReclaimed = %d; want %d", pruneRes.SpaceReclaimed, expectedReclaimed)
	}
}

func TestStarlark_ImageAndPrune(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1.45/images/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]containers.ImageSummary{
			{
				ID:          "sha256:abcdef123456",
				RepoTags:    []string{"alpine:latest"},
				RepoDigests: []string{"alpine@sha256:abc"},
				Created:     1700000000,
				Size:        5242880,
				Labels:      map[string]string{"type": "base"},
			},
		})
	})

	mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"Pull complete"}` + "\n"))
	})

	mux.HandleFunc("/v1.45/containers/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ContainersPruneReport{
			ContainersDeleted: []string{"dead_box"},
			SpaceReclaimed:    1024,
		})
	})

	mux.HandleFunc("/v1.45/volumes/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.VolumesPruneReport{
			VolumesDeleted: []string{"dead_vol"},
			SpaceReclaimed: 2048,
		})
	})

	mux.HandleFunc("/v1.45/images/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ImagesPruneReport{
			ImagesDeleted: []containers.ImageDeletedItem{
				{Deleted: "sha256:dead_img"},
			},
			SpaceReclaimed: 4096,
		})
	})

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	starScript := fmt.Sprintf(`
load("containers", "containers")

def main():
    c = containers.config(host=%q)

    # 1. client.images()
    imgs = c.images()
    if len(imgs) != 1:
        fail("expected 1 image, got %%d" %% len(imgs))
    img = imgs[0]
    if img["id"] != "sha256:abcdef123456":
        fail("unexpected image id: %%s" %% img["id"])
    if img["Id"] != "sha256:abcdef123456":
        fail("unexpected image Id: %%s" %% img["Id"])
    if img["repo_tags"] != ["alpine:latest"]:
        fail("unexpected repo_tags: %%s" %% str(img["repo_tags"]))
    if img["size"] != 5242880:
        fail("unexpected size: %%d" %% img["size"])
    if img["labels"]["type"] != "base":
        fail("unexpected label: %%s" %% img["labels"]["type"])

    # 2. client.pull()
    c.pull("alpine:latest", auth={"username": "user", "password": "pw"})

    # 3. client.prune()
    rep = c.prune(containers=True, volumes=True, images=True)
    if rep["containers_deleted"] != ["dead_box"]:
        fail("unexpected containers_deleted: %%s" %% str(rep["containers_deleted"]))
    if rep["volumes_deleted"] != ["dead_vol"]:
        fail("unexpected volumes_deleted: %%s" %% str(rep["volumes_deleted"]))
    if rep["images_deleted"] != ["sha256:dead_img"]:
        fail("unexpected images_deleted: %%s" %% str(rep["images_deleted"]))
    if rep["space_reclaimed"] != 7168:
        fail("unexpected space_reclaimed: %%d" %% rep["space_reclaimed"])

main()
`, host)

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

func TestImageAndPrunePermissions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.45/images/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]containers.ImageSummary{})
	})
	mux.HandleFunc("/v1.45/images/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"complete"}` + "\n"))
	})
	mux.HandleFunc("/v1.45/containers/prune", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(containers.ContainersPruneReport{})
	})

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	// 1. Deny containers.read -> c.images() fails
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

	scriptImages := fmt.Sprintf(`
load("containers", "containers")
def main():
    c = containers.config(host=%q)
    c.images()
main()
`, host)

	if err := rtDenyRead.Execute(context.Background(), scriptImages); err == nil {
		t.Fatal("expected c.images() to fail when containers.read is denied")
	}

	// 2. Deny containers.write -> c.pull() fails
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

	scriptPull := fmt.Sprintf(`
load("containers", "containers")
def main():
    c = containers.config(host=%q)
    c.pull("alpine:latest")
main()
`, host)

	if err := rtDenyWrite.Execute(context.Background(), scriptPull); err == nil {
		t.Fatal("expected c.pull() to fail when containers.write is denied")
	}

	// 3. Deny containers.write -> c.prune() fails
	scriptPrune := fmt.Sprintf(`
load("containers", "containers")
def main():
    c = containers.config(host=%q)
    c.prune()
main()
`, host)

	if err := rtDenyWrite.Execute(context.Background(), scriptPrune); err == nil {
		t.Fatal("expected c.prune() to fail when containers.write is denied")
	}
}

func TestEngineClient_TCP(t *testing.T) {
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

	host, cleanup := setupMockDaemonTCP(t, mux)
	defer cleanup()

	ep, err := containers.ParseEndpoint(host)
	if err != nil {
		t.Fatalf("ParseEndpoint(%q): %v", host, err)
	}

	client, err := containers.NewEngineClient(ep, 5*time.Second)
	if err != nil {
		t.Fatalf("NewEngineClient: %v", err)
	}

	ok, err := client.Ping(context.Background())
	if err != nil || !ok {
		t.Fatalf("Ping over TCP = %v, %v; want true, nil", ok, err)
	}

	v, err := client.Version(context.Background())
	if err != nil || v["Version"] != "27.1.1" {
		t.Fatalf("Version over TCP = %v, %v; want 27.1.1", v, err)
	}
}

func TestStarlark_ModuleBasedHybrid_AttrDictSerialization(t *testing.T) {
	mux := http.NewServeMux()
	containerID := "c_attrdict_1234567890ab"

	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": containerID})
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Id":   containerID,
			"Name": "/app-db",
			"Config": map[string]any{
				"Image": "postgres:16-alpine",
			},
			"State": map[string]any{
				"Status":  "running",
				"Running": true,
			},
			"NetworkSettings": map[string]any{
				"Ports": map[string]any{
					"5432/tcp": []map[string]any{
						{"HostIp": "0.0.0.0", "HostPort": "32768"},
					},
				},
			},
		})
	})

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	starScript := fmt.Sprintf(`
load("containers", "containers")
load("json", "json")
load("yaml", "yaml")

def main():
    dockr = containers.config(host=%q)

    # 1. dockr.run() returns AttrDict
    box = dockr.run(
        image = "postgres:16-alpine",
        name = "app-db",
        ports = {"5432/tcp": 0},
        detach = True,
    )
    if type(box) != "AttrDict":
        fail("expected AttrDict, got " + type(box))
    if box.id != %q:
        fail("expected id " + %q + ", got " + box.id)
    if box.name != "app-db":
        fail("expected name app-db, got " + box.name)
    if box.status != "running":
        fail("expected status running, got " + box.status)

    # 2. Native serialization without method errors
    json_str = json.encode(box)
    decoded = json.decode(json_str)
    if decoded["id"] != %q:
        fail("json decode failed to preserve id")
    if decoded["name"] != "app-db":
        fail("json decode failed to preserve name")

    yaml_str = yaml.encode(box)
    if "app-db" not in yaml_str:
        fail("yaml encode missing app-db")

    # 3. Verbs accept AttrDict directly
    dockr.stop(box, timeout=5)
    if box.status != "exited":
        fail("expected status exited after stop")

    # 4. Verbs accept string identifier
    dockr.start(%q)
    dockr.stop(%q, timeout=2)
    dockr.delete(%q, force=True)

main()
`, host, containerID, containerID, containerID, containerID, containerID, containerID)

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

func TestStarlark_ModuleShortcuts(t *testing.T) {
	mux := http.NewServeMux()
	containerID := "c_shortcut_9876543210ab"
	execID := "exec_shortcut_123"

	mux.HandleFunc("/_ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/v1.45/containers/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": containerID})
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/stop", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/v1.45/containers/"+containerID+"/exec", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": execID})
	})
	mux.HandleFunc("/v1.45/exec/"+execID+"/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeDockerFrame(1, []byte("shortcut output\n")))
	})
	mux.HandleFunc("/v1.45/exec/"+execID+"/json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"Running":  false,
			"ExitCode": 0,
		})
	})

	host, cleanup := setupMockDaemon(t, mux)
	defer cleanup()

	t.Setenv("DOCKER_HOST", host)

	starScript := fmt.Sprintf(`
load("containers", "containers")

def main():
    # 1. containers.run() module shortcut
    box = containers.run("alpine:latest", name="shortcut-app", detach=True)
    if type(box) != "AttrDict":
        fail("expected AttrDict from containers.run, got " + type(box))
    if box.id != %q:
        fail("expected id " + %q + ", got " + box.id)

    # 2. containers.exec() module shortcut
    res = containers.exec(box, ["echo", "hello"])
    if not res.ok:
        fail("expected res.ok == True")
    if "shortcut output" not in res.stdout:
        fail("expected stdout with shortcut output")

    # 3. containers.stop() module shortcut with string target
    containers.stop(%q, timeout=2)

    # 4. containers.delete() module shortcut with AttrDict target
    containers.delete(box, force=True)

main()
`, containerID, containerID, containerID)

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
