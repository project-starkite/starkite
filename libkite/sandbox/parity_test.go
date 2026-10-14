package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFormatPermissionError_Attribution(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name       string
		err        error
		targetPath string
		spec       *ExecutionSpec
		wantSubstr string
	}{
		{
			name:       "nil error returns nil",
			err:        nil,
			targetPath: "/tmp/test",
			spec:       &ExecutionSpec{},
			wantSubstr: "",
		},
		{
			name:       "no_exec policy without path",
			err:        syscall.EPERM,
			targetPath: "",
			spec:       &ExecutionSpec{NoExec: true},
			wantSubstr: "denied by no_exec policy",
		},
		{
			name:       "no_exec policy with path",
			err:        syscall.EACCES,
			targetPath: "/usr/bin/curl",
			spec:       &ExecutionSpec{NoExec: true},
			wantSubstr: "denied by no_exec policy",
		},
		{
			name:       "W^X on /tmp scratch path",
			err:        syscall.EACCES,
			targetPath: "/tmp/payload.sh",
			spec:       &ExecutionSpec{},
			wantSubstr: "W^X noexec policy",
		},
		{
			name:       "W^X on writable bind mount",
			err:        syscall.EACCES,
			targetPath: "/workspace/out/run.sh",
			spec: &ExecutionSpec{
				Mounts: []Mount{
					{Source: "/workspace/out", Destination: "/workspace/out", Type: MountBind, Mode: MountRW},
				},
			},
			wantSubstr: "W^X noexec policy",
		},
		{
			name:       "shared memory /dev/shm isolation",
			err:        syscall.EACCES,
			targetPath: "/dev/shm/secret_segment",
			spec:       &ExecutionSpec{},
			wantSubstr: "shared memory (/dev/shm) isolation",
		},
		{
			name:       "allowed_exec_paths policy",
			err:        syscall.EACCES,
			targetPath: "/usr/bin/python3",
			spec: &ExecutionSpec{
				AllowedExecPaths: []string{"/usr/bin/git"},
			},
			wantSubstr: "allowed_exec_paths policy",
		},
	}

	if home != "" {
		tests = append(tests, struct {
			name       string
			err        error
			targetPath string
			spec       *ExecutionSpec
			wantSubstr string
		}{
			name:       "home directory container-parity isolation",
			err:        syscall.EACCES,
			targetPath: filepath.Join(home, ".ssh", "id_rsa"),
			spec:       &ExecutionSpec{Cwd: home},
			wantSubstr: "home directory isolation",
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPermissionError(tt.err, tt.targetPath, tt.spec)
			if tt.err == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil error")
			}
			if !strings.Contains(got.Error(), tt.wantSubstr) {
				t.Errorf("FormatPermissionError() = %q, want substring %q", got.Error(), tt.wantSubstr)
			}
		})
	}
}

// isContainerDaemonUnavailable reports whether a container driver failure is due to
// an inactive or unreachable local container daemon rather than a sandbox policy failure.
func isContainerDaemonUnavailable(errMsg string, exitCode int) bool {
	lower := strings.ToLower(errMsg)
	return strings.Contains(lower, "no matching manifest for windows") ||
		strings.Contains(lower, "not supported for windows containers") ||
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
		strings.Contains(lower, "unknown or invalid runtime name: runsc") ||
		strings.Contains(lower, "unknown flag: --runtime") ||
		strings.Contains(lower, "runtime \"runsc\" not found") ||
		strings.Contains(lower, "oci runtime error") ||
		(runtime.GOOS == "windows" && exitCode != 0)
}

// TestCrossDriverParitySuite verifies that all available sandbox drivers
// (Landlock, Seatbelt, Podman, Docker, Nerdctl, gVisor) enforce identical
// security invariants for W^X, NoExec, AllowedExecPaths, and /dev/shm isolation.
func TestCrossDriverParitySuite(t *testing.T) {
	driverNames := []string{
		DriverLandlock,
		DriverSeatbelt,
		DriverPodman,
		DriverDocker,
		DriverNerdctl,
		DriverGVisor,
	}

	for _, drvName := range driverNames {
		t.Run(drvName, func(t *testing.T) {
			d, err := Get(drvName)
			if err != nil || !d.Available() {
				t.Skipf("driver %q not registered or available on %s", drvName, runtime.GOOS)
			}

			isContainer := drvName == DriverPodman || drvName == DriverDocker || drvName == DriverNerdctl || drvName == DriverGVisor
			img := ""
			if isContainer {
				img = "alpine:latest"
			}

			// 1. Parity Check: NoExec blocks subprocess execution
			t.Run("NoExec_BlocksExecution", func(t *testing.T) {
				spec := &ExecutionSpec{
					Command: []string{"/bin/echo", "noexec-should-block"},
					Image:   img,
					Network: NetworkHost,
					NoExec:  true,
					Timeout: 15 * time.Second,
				}
				res, err := d.Exec(context.Background(), spec)
				if err == nil && res != nil && res.ExitCode == 0 {
					t.Fatalf("[%s] NoExec parity violation: /bin/echo succeeded when NoExec=true (stdout=%q)", drvName, res.Stdout)
				}
				if res != nil && !strings.Contains(res.Stderr, "no_exec") && !strings.Contains(strings.ToLower(res.Stderr), "not permitted") {
					t.Errorf("[%s] expected no_exec diagnostic in stderr, got: %q", drvName, res.Stderr)
				}
			})

			// 2. Parity Check: AllowedExecPaths permits allowlisted binaries and blocks non-allowlisted binaries
			t.Run("AllowedExecPaths_Enforced", func(t *testing.T) {
				allowedSpec := &ExecutionSpec{
					Command:          []string{"/bin/echo", "parity-allowed-ok"},
					Image:            img,
					Network:          NetworkHost,
					AllowedExecPaths: []string{"/bin/echo", "/usr/bin/echo"},
					Timeout:          15 * time.Second,
				}
				resAllow, errAllow := d.Exec(context.Background(), allowedSpec)
				if errAllow != nil {
					if isContainer && isContainerDaemonUnavailable(errAllow.Error(), 125) {
						t.Logf("[%s] container daemon unavailable for positive exec check: %v", drvName, errAllow)
					} else {
						t.Errorf("[%s] expected allowlisted /bin/echo to succeed, got error: %v", drvName, errAllow)
					}
				} else if resAllow != nil && resAllow.ExitCode != 0 {
					if isContainer && isContainerDaemonUnavailable(resAllow.Stderr, resAllow.ExitCode) {
						t.Logf("[%s] container daemon unavailable for positive exec check: %s", drvName, resAllow.Stderr)
					} else {
						t.Errorf("[%s] expected allowlisted /bin/echo to exit 0, got exit=%d stderr=%q", drvName, resAllow.ExitCode, resAllow.Stderr)
					}
				}

				deniedSpec := &ExecutionSpec{
					Command:          []string{"/usr/bin/id"},
					Image:            img,
					Network:          NetworkHost,
					AllowedExecPaths: []string{"/bin/echo", "/usr/bin/echo"},
					Timeout:          15 * time.Second,
				}
				res, err := d.Exec(context.Background(), deniedSpec)
				if err == nil && res != nil && res.ExitCode == 0 {
					t.Fatalf("[%s] AllowedExecPaths parity violation: /usr/bin/id succeeded when not allowlisted (stdout=%q)", drvName, res.Stdout)
				}
				if res != nil && !strings.Contains(res.Stderr, "allowed_exec_paths") && !strings.Contains(strings.ToLower(res.Stderr), "not permitted") && !strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
					t.Errorf("[%s] expected allowed_exec_paths diagnostic in stderr, got: %q", drvName, res.Stderr)
				}
			})

			// 3. Parity Check: Write-XOR-Execute (W^X) blocks executing binaries from writable mounts
			t.Run("WX_BlocksWritableMountExecution", func(t *testing.T) {
				scratchDir := t.TempDir()
				payload := filepath.Join(scratchDir, "wx_probe.sh")
				if err := os.WriteFile(payload, []byte("#!/bin/sh\necho wx-escaped\n"), 0o755); err != nil {
					t.Fatal(err)
				}

				wxSpec := &ExecutionSpec{
					Command: []string{payload},
					Image:   img,
					Network: NetworkHost,
					Mounts: []Mount{
						{
							Source:      scratchDir,
							Destination: scratchDir,
							Type:        MountBind,
							Mode:        MountRW,
						},
					},
					Timeout: 15 * time.Second,
				}
				res, err := d.Exec(context.Background(), wxSpec)
				if err == nil && res != nil && res.ExitCode == 0 {
					t.Fatalf("[%s] W^X parity violation: executing %q from writable mount succeeded! stdout=%q", drvName, payload, res.Stdout)
				}
				if isContainer && res != nil && isContainerDaemonUnavailable(res.Stderr, res.ExitCode) {
					t.Logf("[%s] container daemon unavailable for W^X check: %s", drvName, res.Stderr)
					return
				}
				if isContainer && err != nil && isContainerDaemonUnavailable(err.Error(), 125) {
					t.Logf("[%s] container daemon unavailable for W^X check: %v", drvName, err)
					return
				}
				if res != nil && !strings.Contains(res.Stderr, "W^X") && !strings.Contains(strings.ToLower(res.Stderr), "permission denied") && !strings.Contains(strings.ToLower(res.Stderr), "not permitted") {
					t.Errorf("[%s] expected W^X diagnostic in stderr, got: %q", drvName, res.Stderr)
				}
			})

			// 4. Parity Check: $HOME container-parity isolation when Cwd == $HOME
			t.Run("HomeDirectory_ContainerParityIsolation", func(t *testing.T) {
				if realHome, err := os.UserHomeDir(); err == nil && realHome != "" {
					if os.Getenv("XDG_DATA_HOME") == "" {
						t.Setenv("XDG_DATA_HOME", filepath.Join(realHome, ".local", "share"))
					}
					if os.Getenv("XDG_CONFIG_HOME") == "" {
						t.Setenv("XDG_CONFIG_HOME", filepath.Join(realHome, ".config"))
					}
				}
				fakeHome := t.TempDir()
				t.Setenv("HOME", fakeHome)
				t.Setenv("USERPROFILE", fakeHome)

				secretPath := filepath.Join(fakeHome, "secret_cred.txt")
				if err := os.WriteFile(secretPath, []byte("top-secret-token"), 0o600); err != nil {
					t.Fatal(err)
				}
				scriptPath := filepath.Join(fakeHome, "app.star")
				if err := os.WriteFile(scriptPath, []byte("print('ok')"), 0o644); err != nil {
					t.Fatal(err)
				}

				homeSpec := &ExecutionSpec{
					Command:    []string{"/bin/cat", secretPath},
					Cwd:        fakeHome,
					ScriptFile: scriptPath,
					Image:      img,
					Network:    NetworkHost,
					Mounts: []Mount{
						{
							Source:      fakeHome,
							Destination: fakeHome,
							Type:        MountBind,
							Mode:        MountRW,
						},
					},
					Timeout: 15 * time.Second,
				}
				res, err := d.Exec(context.Background(), homeSpec)
				if err == nil && res != nil && res.ExitCode == 0 && strings.Contains(res.Stdout, "top-secret-token") {
					t.Fatalf("[%s] $HOME container-parity violation: sibling secret file in $HOME was readable when Cwd=$HOME!", drvName)
				}
			})

			// 5. Parity Check: /dev/shm mount stripping / isolation
			if isContainer {
				t.Run("DevShm_ContainerArgsIsolated", func(t *testing.T) {
					cd := NewContainerDriver(drvName, "/usr/bin/"+drvName)
					spec := &ExecutionSpec{
						Command: []string{"/bin/true"},
						Mounts: []Mount{
							{
								Source:      "/dev/shm",
								Destination: "/dev/shm",
								Type:        MountBind,
								Mode:        MountRW,
							},
						},
					}
					args := strings.Join(cd.BuildArgs(spec), " ")
					if strings.Contains(args, "/dev/shm:/dev/shm") {
						t.Errorf("[%s] /dev/shm bind mount must be stripped from container args: %s", drvName, args)
					}
					if !strings.Contains(args, "--ipc=private") {
						t.Errorf("[%s] expected --ipc=private in container args: %s", drvName, args)
					}
				})
			}
		})
	}
}
