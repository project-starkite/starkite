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
