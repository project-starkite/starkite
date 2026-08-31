package sandbox

import (
	"context"
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
		"--network=none",
		"--workdir /workspace",
		"-e GOOS=linux",
		"-e CGO_ENABLED=0",
		"--memory=512m",
		"--cpus=2.000000",
		"--pids-limit=100",
		"-v /host/src:/workspace:rw",
		"-v /host/cache:/root/.cache:ro",
		"--tmpfs=/tmp:rw,noexec,nosuid",
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

			res, err := d.Exec(context.Background(), spec)
			if err != nil {
				if strings.Contains(err.Error(), "no matching manifest for windows") || strings.Contains(res.Stderr, "no matching manifest for windows") {
					t.Skipf("container engine %s cannot run Linux images on this host: %v", engineName, err)
				}
				t.Fatalf("Live %s container execution failed: %v, stderr: %s", engineName, err, res.Stderr)
			}

			if res.ExitCode != 0 {
				if strings.Contains(res.Stderr, "no matching manifest for windows") || (runtime.GOOS == "windows" && res.ExitCode == 125) {
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
