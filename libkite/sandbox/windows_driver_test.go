//go:build windows

package sandbox

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWindowsDriver_RegistrationAndAvailability(t *testing.T) {
	d, err := Get(DriverWindows)
	if err != nil {
		t.Fatalf("Get(DriverWindows) failed: %v", err)
	}
	if d.Name() != DriverWindows {
		t.Errorf("d.Name() = %q, want %q", d.Name(), DriverWindows)
	}

	alias, err := Get(DriverAppContainer)
	if err != nil {
		t.Fatalf("Get(DriverAppContainer) failed: %v", err)
	}
	if alias.Name() != DriverWindows {
		t.Errorf("alias.Name() = %q, want %q", alias.Name(), DriverWindows)
	}

	if !d.Available() {
		t.Logf("WindowsDriver reports unavailable on this host: %v", checkWindowsOSBaseline())
	}
}

func TestWindowsDriver_BasicExec(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	spec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "echo", "hello-windows-sandbox"},
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (stderr: %q)", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello-windows-sandbox") {
		t.Errorf("Stdout = %q, want substring 'hello-windows-sandbox'", res.Stdout)
	}
}

func TestWindowsDriver_NoExecBlocksChildProcess(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	// cmd.exe is launched, but when it attempts to spawn whoami.exe,
	// PROC_THREAD_ATTRIBUTE_CHILD_PROCESS_POLICY (PROCESS_CREATION_CHILD_PROCESS_RESTRICTED)
	// causes the kernel to deny CreateProcess with Access is denied.
	spec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "whoami.exe"},
		NoExec:  true,
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	// Process may return non-zero exit code or error
	if err == nil && res.ExitCode == 0 {
		t.Errorf("expected child process creation to be blocked under NoExec=true, got exit 0 (stdout: %q)", res.Stdout)
	}
	combined := ""
	if res != nil {
		combined = res.Stdout + res.Stderr
	}
	t.Logf("NoExec child block result: exit=%v output=%q", res.ExitCode, combined)
}

func TestWindowsDriver_JobObjectTimeoutAndOrphanCleanup(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	spec := &ExecutionSpec{
		Command: []string{"cmd.exe", "/c", "ping.exe", "-n", "30", "127.0.0.1"},
		Timeout: 500 * time.Millisecond,
	}

	res, err := d.Exec(context.Background(), spec)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if res == nil || !res.TimedOut {
		t.Errorf("expected res.TimedOut == true, got %v", res)
	}
}

func TestWindowsDriver_JobObjectMemoryLimitOOM(t *testing.T) {
	d := NewWindowsDriver()
	if !d.Available() {
		t.Skip("skipping on Windows host that does not meet OS baseline")
	}

	// Request 16MB limit while PowerShell allocates 64MB array
	spec := &ExecutionSpec{
		Command:     []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$a = New-Object byte[] (64*1024*1024)"},
		MaxMemoryMB: 16,
		Timeout:     10 * time.Second,
	}

	res, _ := d.Exec(context.Background(), spec)
	if res != nil {
		t.Logf("OOM test result: exit=%d killedByOOM=%v stderr=%q", res.ExitCode, res.KilledByOOM, res.Stderr)
		if !res.KilledByOOM && res.ExitCode == 0 {
			t.Errorf("expected memory limit termination, but process exited with code 0")
		}
	}
}

func TestWindowsDriver_OSBaselineCheck(t *testing.T) {
	// With override env var set, baseline check always passes
	os.Setenv("STARKITE_WINDOWS_ALLOW_LEGACY_BUILD", "1")
	defer os.Unsetenv("STARKITE_WINDOWS_ALLOW_LEGACY_BUILD")

	if err := checkWindowsOSBaseline(); err != nil {
		t.Errorf("checkWindowsOSBaseline() with override failed: %v", err)
	}
}
