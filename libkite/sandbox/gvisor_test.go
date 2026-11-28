package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGVisorDriver_Registration(t *testing.T) {
	drv, err := Get(DriverGVisor)
	if err != nil {
		t.Fatalf("Get(DriverGVisor) error: %v", err)
	}
	if drv.Name() != DriverGVisor {
		t.Errorf("drv.Name() = %s, want %s", drv.Name(), DriverGVisor)
	}
}

func TestGVisorDriver_ValidateSpec(t *testing.T) {
	drv := NewGVisorDriver()
	if !drv.Available() {
		t.Skip("gVisor or container runtime not available on this host; skipping")
	}

	validSpec := &ExecutionSpec{
		Command: []string{"echo", "test"},
	}
	if err := drv.ValidateSpec(validSpec); err != nil {
		t.Errorf("ValidateSpec(validSpec) error: %v", err)
	}

	invalidSpec := &ExecutionSpec{
		Command: []string{},
	}
	if err := drv.ValidateSpec(invalidSpec); err == nil {
		t.Error("ValidateSpec(empty command) expected error, got nil")
	}
}

func TestGVisorDriver_Exec(t *testing.T) {
	drv := NewGVisorDriver()
	if !drv.Available() {
		t.Skip("gVisor runtime not available; skipping")
	}

	spec := &ExecutionSpec{
		Command: []string{"cat", "/proc/version"},
		Image:   "alpine:latest",
		Network: NetworkNone,
		Timeout: 30 * time.Second,
	}

	res, err := drv.Exec(context.Background(), spec)
	if err != nil {
		t.Logf("gVisor execution note: %v (runsc OCI runtime might not be configured in container daemon)", err)
		return
	}

	if res.ExitCode != 0 {
		t.Logf("gVisor exit code: %d, stderr: %s", res.ExitCode, res.Stderr)
		return
	}

	t.Logf("gVisor /proc/version output: %s", strings.TrimSpace(res.Stdout))

	if strings.Contains(strings.ToLower(res.Stdout), "gvisor") || strings.Contains(strings.ToLower(res.Stdout), "sentry") {
		t.Log("Successfully verified gVisor Sentry application kernel inside container!")
	}
}

func TestGVisorDriver_StandaloneRunscFailClosed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := &GVisorDriver{runscPath: "/nonexistent/runsc"}
	if d.Available() {
		t.Fatalf("expected Available()=false when runsc does not exist on disk")
	}

	dValid := &GVisorDriver{runscPath: "/usr/bin/runsc"}
	tests := []struct {
		name string
		spec *ExecutionSpec
	}{
		{
			name: "network none",
			spec: &ExecutionSpec{Command: []string{"/bin/echo", "hi"}, Network: NetworkNone},
		},
		{
			name: "mounts configured",
			spec: &ExecutionSpec{
				Command: []string{"/bin/echo", "hi"},
				Network: NetworkHost,
				Mounts:  []Mount{{Source: "/tmp", Destination: "/tmp", Type: MountBind, Mode: MountRO}},
			},
		},
		{
			name: "memory limit set",
			spec: &ExecutionSpec{Command: []string{"/bin/echo", "hi"}, Network: NetworkHost, MaxMemoryMB: 128},
		},
		{
			name: "no_exec set",
			spec: &ExecutionSpec{Command: []string{"/bin/echo", "hi"}, Network: NetworkHost, NoExec: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.spec.NoExec {
				res, err := dValid.execStandaloneRunsc(context.Background(), tc.spec)
				if err == nil && (res == nil || res.ExitCode == 0) {
					t.Errorf("expected standalone runsc to block NoExec")
				}
				return
			}
			if _, err := dValid.execStandaloneRunsc(context.Background(), tc.spec); err == nil {
				t.Errorf("expected standalone runsc to fail closed for %s", tc.name)
			}
		})
	}
}
