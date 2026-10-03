package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestContainerDriver_Registration(t *testing.T) {
	podman, err := Get(DriverPodman)
	if err != nil {
		t.Fatalf("Get(DriverPodman) error: %v", err)
	}
	if podman.Name() != DriverPodman {
		t.Errorf("podman.Name() = %s, want %s", podman.Name(), DriverPodman)
	}

	docker, err := Get(DriverDocker)
	if err != nil {
		t.Fatalf("Get(DriverDocker) error: %v", err)
	}
	if docker.Name() != DriverDocker {
		t.Errorf("docker.Name() = %s, want %s", docker.Name(), DriverDocker)
	}

	nerdctl, err := Get(DriverNerdctl)
	if err != nil {
		t.Fatalf("Get(DriverNerdctl) error: %v", err)
	}
	if nerdctl.Name() != DriverNerdctl {
		t.Errorf("nerdctl.Name() = %s, want %s", nerdctl.Name(), DriverNerdctl)
	}
}

func TestContainerDriver_BuildArgs(t *testing.T) {
	d := NewContainerDriver("podman", "/usr/bin/podman")

	spec := &ExecutionSpec{
		Command:     []string{"go", "build", "-o", "app"},
		Cwd:         "/workspace",
		Env:         []string{"GOOS=linux", "CGO_ENABLED=0"},
		Network:     NetworkNone,
		MaxMemoryMB: 512,
		MaxCPUs:     2.0,
		MaxPIDs:     100,
		Image:       "golang:1.24-alpine",
		Runtime:     "runsc",
		Mounts: []Mount{
			{
				Source:      "/host/src",
				Destination: "/workspace",
				Type:        MountBind,
				Mode:        MountRW,
			},
			{
				Source:      "/host/cache",
				Destination: "/root/.cache",
				Type:        MountBind,
				Mode:        MountRO,
			},
			{
				Destination: "/tmp",
				Type:        MountTmpfs,
			},
		},
	}

	args := d.BuildArgs(spec)
	joined := strings.Join(args, " ")

	expectedTokens := []string{
		"run --rm -i",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--ipc=private",
		"--uts=private",
		"--read-only",
		"--network=none",
		"--workdir /workspace",
		"-e GOOS=linux",
		"-e CGO_ENABLED=0",
		"--memory=512m",
		"--cpus=2.000000",
		"--pids-limit=100",
		"-v /host/src:/workspace:rw",
		"-v /host/cache:/root/.cache:ro",
		"--tmpfs=/tmp:rw,noexec,nosuid,nodev",
		"--runtime=runsc",
		"golang:1.24-alpine",
		"go build -o app",
	}

	for _, token := range expectedTokens {
		if !strings.Contains(joined, token) {
			t.Errorf("BuildArgs() output missing token %q:\nFull args: %s", token, joined)
		}
	}
}

func TestContainerDriver_DefaultImageAndNetworkHost(t *testing.T) {
	d := NewContainerDriver("docker", "/usr/bin/docker")

	spec := &ExecutionSpec{
		Command: []string{"ls", "-la"},
		Network: NetworkHost,
	}

	args := d.BuildArgs(spec)
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--network=host") {
		t.Errorf("expected --network=host, got: %s", joined)
	}
	if !strings.Contains(joined, DefaultContainerImage) {
		t.Errorf("expected default image %q, got: %s", DefaultContainerImage, joined)
	}
}

func TestContainerDriver_StarkiteImageEntrypoint(t *testing.T) {
	d := NewContainerDriver("podman", "/usr/bin/podman")

	// When using the default starkite image, kite binary prefix should be stripped
	// since the OCI entrypoint is already /ko-app/kite.
	spec := &ExecutionSpec{
		Command: []string{"/path/to/kite", "run", "deploy.star", "--permissions=allow-net"},
	}

	args := d.BuildArgs(spec)
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "/path/to/kite") || strings.Contains(joined, "/usr/local/bin/kite") {
		t.Errorf("expected kite binary to be stripped, got args: %s", joined)
	}
	expectedTail := "ghcr.io/project-starkite/starkite:latest run deploy.star --permissions=allow-net"
	if !strings.HasSuffix(joined, expectedTail) {
		t.Errorf("expected args to end with %q, got: %s", expectedTail, joined)
	}
}

func TestContainerDriver_HardeningContract(t *testing.T) {
	d := NewContainerDriver("docker", "/usr/bin/docker")

	// 1. Verify default pids-limit=256 and default /tmp tmpfs when unconstrained
	spec := &ExecutionSpec{
		Command: []string{"test.star"},
		Cwd:     "/workspace",
	}

	args := d.BuildArgs(spec)
	joined := strings.Join(args, " ")

	hardeningTokens := []string{
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges:true",
		"--ipc=private",
		"--uts=private",
		"--read-only",
		"--pids-limit=256",
		"--tmpfs=/tmp:rw,noexec,nosuid,nodev",
	}

	for _, token := range hardeningTokens {
		if !strings.Contains(joined, token) {
			t.Errorf("HardeningContract missing token %q in: %s", token, joined)
		}
	}

	// 2. On Linux, verify --user=<uid>:<gid> is injected
	if runtime.GOOS == "linux" {
		expectedUser := fmt.Sprintf("--user=%d:%d", os.Getuid(), os.Getgid())
		if !strings.Contains(joined, expectedUser) {
			t.Errorf("expected Linux user flag %q in: %s", expectedUser, joined)
		}
	}
}

func TestContainerDriver_ContainerParity(t *testing.T) {
	d := NewContainerDriver("docker", "/usr/bin/docker")

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		t.Skip("skipping container parity test: cannot resolve UserHomeDir")
	}

	// Case 1: Cwd == $HOME -> omit $HOME bind mount, mount script file read-only
	scriptPath := filepath.Join(homeDir, "script.star")
	specHome := &ExecutionSpec{
		Command:    []string{"script.star"},
		Cwd:        homeDir,
		ScriptFile: scriptPath,
		Mounts: []Mount{
			{
				Source:      homeDir,
				Destination: homeDir,
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
	}

	argsHome := d.BuildArgs(specHome)
	joinedHome := strings.Join(argsHome, " ")

	// Ensure the full host $HOME bind mount is NOT in args
	badMount := fmt.Sprintf("-v %s:%s:rw", homeDir, homeDir)
	if strings.Contains(joinedHome, badMount) {
		t.Errorf("container parity violation: host $HOME mount %q must be omitted when Cwd is $HOME", badMount)
	}

	// Ensure the script file is mounted read-only
	expectedScriptMount := fmt.Sprintf("-v %s:%s:ro", scriptPath, scriptPath)
	if !strings.Contains(joinedHome, expectedScriptMount) {
		t.Errorf("container parity: expected script mount %q in args: %s", expectedScriptMount, joinedHome)
	}

	// Case 2: Cwd == "/" -> omit root mount, mount script file read-only
	specRoot := &ExecutionSpec{
		Command:    []string{"root.star"},
		Cwd:        "/",
		ScriptFile: "/root.star",
		Mounts: []Mount{
			{
				Source:      "/",
				Destination: "/",
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
	}

	argsRoot := d.BuildArgs(specRoot)
	joinedRoot := strings.Join(argsRoot, " ")

	if strings.Contains(joinedRoot, "-v /:/:rw") {
		t.Errorf("container parity violation: root mount -v /:/:rw must be omitted when Cwd is /")
	}
	if !strings.Contains(joinedRoot, "-v /root.star:/root.star:ro") {
		t.Errorf("container parity: expected -v /root.star:/root.star:ro in args: %s", joinedRoot)
	}

	// Case 3: Cwd == "/workspace" (normal dir) -> keep normal mounts
	specNormal := &ExecutionSpec{
		Command: []string{"work.star"},
		Cwd:     "/workspace",
		Mounts: []Mount{
			{
				Source:      "/workspace",
				Destination: "/workspace",
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
	}

	argsNormal := d.BuildArgs(specNormal)
	joinedNormal := strings.Join(argsNormal, " ")

	if !strings.Contains(joinedNormal, "-v /workspace:/workspace:rw") {
		t.Errorf("expected normal workspace mount to be preserved in args: %s", joinedNormal)
	}
}

func TestContainerDriver_LiveExec(t *testing.T) {
	engines := []string{DriverPodman, DriverDocker, DriverNerdctl}
	var anyRan bool

	for _, engineName := range engines {
		t.Run(engineName, func(t *testing.T) {
			d, err := Get(engineName)
			if err != nil {
				t.Fatalf("Get(%s) error: %v", engineName, err)
			}
			if !d.Available() {
				t.Skipf("%s is not installed or available on this system; skipping", engineName)
			}

			anyRan = true
			t.Logf("Running live container test using %s", engineName)

			spec := &ExecutionSpec{
				Command: []string{"echo", "hello-from-" + engineName},
				Image:   "alpine:latest",
				Network: NetworkNone,
				Timeout: 30 * time.Second,
			}

			isDaemonUnavailable := func(errMsg string, exitCode int) bool {
				lower := strings.ToLower(errMsg)
				return strings.Contains(lower, "no matching manifest for windows") ||
					strings.Contains(lower, "docker_engine") ||
					strings.Contains(lower, "daemon is running") ||
					strings.Contains(lower, "cannot connect to the docker daemon") ||
					strings.Contains(lower, "failed to connect to the docker api") ||
					strings.Contains(lower, "connection refused") ||
					strings.Contains(lower, "cannot connect to the podman service") ||
					strings.Contains(lower, "cannot connect to podman") ||
					strings.Contains(lower, "unable to connect to podman") ||
					strings.Contains(lower, "is the docker daemon running") ||
					strings.Contains(lower, "pipe/docker_engine") ||
					(runtime.GOOS == "windows" && exitCode != 0)
			}

			res, err := d.Exec(context.Background(), spec)
			if err != nil {
				exitCode := 0
				stderr := ""
				if res != nil {
					exitCode = res.ExitCode
					stderr = res.Stderr
				}
				if isDaemonUnavailable(err.Error(), exitCode) || isDaemonUnavailable(stderr, exitCode) {
					t.Skipf("container engine %s cannot run Linux images on this host: %v", engineName, err)
				}
				t.Fatalf("Live %s container execution failed: %v, stderr: %s", engineName, err, stderr)
			}

			if res.ExitCode != 0 {
				if isDaemonUnavailable(res.Stderr, res.ExitCode) {
					t.Skipf("container engine %s cannot run Linux images on this host: %s", engineName, res.Stderr)
				}
				t.Errorf("ExitCode = %d, want 0; stderr: %s", res.ExitCode, res.Stderr)
			}

			if !strings.Contains(res.Stdout, "hello-from-"+engineName) {
				t.Errorf("Stdout = %q, want 'hello-from-%s'", res.Stdout, engineName)
			}
		})
	}

	if !anyRan {
		t.Log("No container engines (podman/docker/nerdctl) available on this host for live execution")
	}
}
