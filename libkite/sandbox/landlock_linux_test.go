//go:build linux

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLandlockDriver_RegistrationAndAvailability(t *testing.T) {
	d, err := Get(DriverLandlock)
	if err != nil {
		t.Fatalf("Get(DriverLandlock) failed: %v", err)
	}

	if d.Name() != DriverLandlock {
		t.Errorf("d.Name() = %s, want %s", d.Name(), DriverLandlock)
	}

	if !d.Available() {
		t.Logf("Landlock is not enabled on this kernel")
	}
}

func TestLandlockDriver_ExecBasic(t *testing.T) {
	d, err := Resolve(DriverDefault)
	if err != nil {
		t.Fatalf("Resolve(DriverDefault) error: %v", err)
	}

	spec := &ExecutionSpec{
		Command: []string{"/bin/echo", "hello-landlock"},
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("Exec failed: %v, stderr=%s", err, res.Stderr)
	}

	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}

	if !strings.Contains(res.Stdout, "hello-landlock") {
		t.Errorf("Stdout = %q, want 'hello-landlock'", res.Stdout)
	}
}

func TestLandlockDriver_FilesystemIsolation(t *testing.T) {
	tempDir := t.TempDir()
	allowedFile := filepath.Join(tempDir, "allowed.txt")
	if err := os.WriteFile(allowedFile, []byte("allowed-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Get(DriverLandlock)
	if err != nil {
		t.Fatalf("Get(DriverLandlock) error: %v", err)
	}

	// Test reading allowed file
	readSpec := &ExecutionSpec{
		Command: []string{"/bin/cat", allowedFile},
		Mounts: []Mount{
			{
				Source:      tempDir,
				Destination: tempDir,
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), readSpec)
	if err != nil {
		t.Fatalf("reading allowed file failed: %v", err)
	}
	if !strings.Contains(res.Stdout, "allowed-data") {
		t.Errorf("expected allowed data in stdout, got %q", res.Stdout)
	}
}

func TestLandlockDriver_NetworkIsolationFailClosed(t *testing.T) {
	d, err := Get(DriverLandlock)
	if err != nil {
		t.Fatalf("Get(DriverLandlock) failed: %v", err)
	}

	spec := &ExecutionSpec{
		Command: []string{"/bin/echo", "isolated"},
		Network: NetworkNone,
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		// If host kernel restricts unprivileged user namespaces, must fail closed with descriptive message
		if strings.Contains(err.Error(), `network isolation (network="none") failed`) {
			t.Logf("Deterministic fail-closed on restricted userns: %v", err)
			return
		}
		t.Fatalf("unexpected Exec error: %v", err)
	}

	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "isolated") {
		t.Errorf("Stdout = %q, want 'isolated'", res.Stdout)
	}
}

func TestLandlockDriver_ValidateSpec_ContainerParity(t *testing.T) {
	d := NewLandlockDriver()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("skipping test: os.UserHomeDir() unavailable")
	}

	// 1. Cwd == $HOME -> should succeed (isolated to script file only under container parity)
	specHome := &ExecutionSpec{
		Command: []string{"/bin/true"},
		Cwd:     home,
	}
	if err := d.ValidateSpec(specHome); err != nil {
		t.Errorf("ValidateSpec with Cwd=HOME should succeed under container parity, got: %v", err)
	}

	// 2. Cwd == "/" -> should succeed (isolated to script file only under container parity)
	specRoot := &ExecutionSpec{
		Command: []string{"/bin/true"},
		Cwd:     "/",
	}
	if err := d.ValidateSpec(specRoot); err != nil {
		t.Errorf("ValidateSpec with Cwd=/ should succeed under container parity, got: %v", err)
	}

	// 3. Cwd == subdirectory of $HOME -> should succeed
	subDir := filepath.Join(home, "starkite-test-sandbox-sub")
	specSub := &ExecutionSpec{
		Command: []string{"/bin/true"},
		Cwd:     subDir,
	}
	if err := d.ValidateSpec(specSub); err != nil {
		t.Errorf("ValidateSpec with Cwd=subDir failed: %v", err)
	}
}

func TestLandlockDriver_ApplyResourceLimits(t *testing.T) {
	var origNproc, origCPU unix.Rlimit
	_ = unix.Getrlimit(unix.RLIMIT_NPROC, &origNproc)
	_ = unix.Getrlimit(unix.RLIMIT_CPU, &origCPU)
	defer func() {
		_ = unix.Setrlimit(unix.RLIMIT_NPROC, &origNproc)
		_ = unix.Setrlimit(unix.RLIMIT_CPU, &origCPU)
	}()

	// Test applyResourceLimits with various spec configurations
	specDefault := &ExecutionSpec{}
	if err := applyResourceLimits(specDefault); err != nil {
		t.Errorf("applyResourceLimits(default) failed: %v", err)
	}

	specCustom := &ExecutionSpec{
		MaxPIDs: 8192,
		Timeout: 60 * time.Second,
	}
	if err := applyResourceLimits(specCustom); err != nil {
		t.Errorf("applyResourceLimits(custom) failed: %v", err)
	}
}

func TestLandlockDriver_ABINegotiation(t *testing.T) {
	d := NewLandlockDriver()
	abi := landlockABI()
	t.Logf("Detected Linux Landlock ABI version: v%d", abi)
	if d.Available() && abi < 1 {
		t.Errorf("d.Available() is true, but landlockABI() returned %d", abi)
	}
	if !d.Available() && abi >= 1 {
		t.Errorf("d.Available() is false, but landlockABI() returned %d", abi)
	}
}

func TestLandlockDriver_WXAndNoExec(t *testing.T) {
	d := NewLandlockDriver()
	if !d.Available() {
		t.Skip("Landlock LSM not available on this kernel")
	}

	// 1. Write-XOR-Execute (W^X): create an executable script inside a writable mount
	// and verify Landlock blocks executing it while still allowing reading/writing.
	scratchDir := t.TempDir()
	scriptPath := filepath.Join(scratchDir, "payload.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho wx-bypassed\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	wxSpec := &ExecutionSpec{
		Command: []string{scriptPath},
		Mounts: []Mount{
			{
				Source:      scratchDir,
				Destination: scratchDir,
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
		Network: NetworkHost,
		Timeout: 5 * time.Second,
	}
	res, err := d.Exec(context.Background(), wxSpec)
	if err == nil && res.ExitCode == 0 {
		t.Errorf("W^X violation: executing script from writable mount succeeded! stdout=%q", res.Stdout)
	}
	if res != nil && !strings.Contains(res.Stderr, "W^X") && !strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
		t.Errorf("expected W^X diagnostic in stderr, got: %q", res.Stderr)
	}

	// 2. NoExec: verify subprocess execution is blocked when NoExec=true
	noExecSpec := &ExecutionSpec{
		Command: []string{"/bin/echo", "noexec-bypass"},
		NoExec:  true,
		Network: NetworkHost,
		Timeout: 5 * time.Second,
	}
	resNoExec, err := d.Exec(context.Background(), noExecSpec)
	if err == nil && resNoExec.ExitCode == 0 {
		t.Errorf("NoExec violation: /bin/echo succeeded when NoExec=true! stdout=%q", resNoExec.Stdout)
	}
	if resNoExec != nil && !strings.Contains(resNoExec.Stderr, "no_exec") {
		t.Errorf("expected no_exec diagnostic in stderr, got: %q", resNoExec.Stderr)
	}

	// 3. AllowedExecPaths: allow /bin/echo (and /usr/bin/echo), verify /bin/echo works and /usr/bin/id is blocked
	allowedSpec := &ExecutionSpec{
		Command:          []string{"/bin/echo", "allowed-exec-ok"},
		AllowedExecPaths: []string{"/bin/echo", "/usr/bin/echo"},
		Network:          NetworkHost,
		Timeout:          5 * time.Second,
	}
	resAllowed, err := d.Exec(context.Background(), allowedSpec)
	if err != nil || resAllowed.ExitCode != 0 {
		t.Errorf("AllowedExecPaths failed for permitted binary /bin/echo: err=%v, exit=%d, stderr=%s", err, resAllowed.ExitCode, resAllowed.Stderr)
	}

	deniedSpec := &ExecutionSpec{
		Command:          []string{"/usr/bin/id"},
		AllowedExecPaths: []string{"/bin/echo", "/usr/bin/echo"},
		Network:          NetworkHost,
		Timeout:          5 * time.Second,
	}
	resDenied, err := d.Exec(context.Background(), deniedSpec)
	if err == nil && resDenied.ExitCode == 0 {
		t.Errorf("AllowedExecPaths violation: /usr/bin/id succeeded when not in AllowedExecPaths! stdout=%q", resDenied.Stdout)
	}

	// 4. Shared memory (/dev/shm) isolation: reading/writing /dev/shm must be blocked by Landlock
	shmSpec := &ExecutionSpec{
		Command: []string{"/bin/sh", "-c", "touch /dev/shm/starkite_shm_test"},
		Network: NetworkHost,
		Timeout: 5 * time.Second,
	}
	resShm, _ := d.Exec(context.Background(), shmSpec)
	if resShm != nil && resShm.ExitCode == 0 {
		_ = os.Remove("/dev/shm/starkite_shm_test")
		t.Errorf("/dev/shm isolation violation: touch /dev/shm/starkite_shm_test succeeded inside Landlock!")
	}
}
