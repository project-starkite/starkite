//go:build linux

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestLandlockDriver_ValidateSpec_HomeGuard(t *testing.T) {
	d := NewLandlockDriver()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("skipping test: os.UserHomeDir() unavailable")
	}

	// 1. Cwd == $HOME and AllowHomeCwd == false -> should fail
	specHome := &ExecutionSpec{
		Command: []string{"/bin/true"},
		Cwd:     home,
	}
	if err := d.ValidateSpec(specHome); err == nil {
		t.Errorf("ValidateSpec with Cwd=HOME without AllowHomeCwd should fail, got nil")
	} else if !strings.Contains(err.Error(), "prohibited under Landlock") {
		t.Errorf("ValidateSpec error = %v, expected Landlock home prohibition", err)
	}

	// 2. Cwd == "/" and AllowHomeCwd == false -> should fail
	specRoot := &ExecutionSpec{
		Command: []string{"/bin/true"},
		Cwd:     "/",
	}
	if err := d.ValidateSpec(specRoot); err == nil {
		t.Errorf("ValidateSpec with Cwd=/ without AllowHomeCwd should fail, got nil")
	} else if !strings.Contains(err.Error(), "prohibited under Landlock") {
		t.Errorf("ValidateSpec error = %v, expected Landlock home prohibition", err)
	}

	// 3. Cwd == $HOME and AllowHomeCwd == true -> should succeed
	specHomeAllowed := &ExecutionSpec{
		Command:      []string{"/bin/true"},
		Cwd:          home,
		AllowHomeCwd: true,
	}
	if err := d.ValidateSpec(specHomeAllowed); err != nil {
		t.Errorf("ValidateSpec with Cwd=HOME and AllowHomeCwd=true failed: %v", err)
	}

	// 4. Cwd == "/" and AllowHomeCwd == true -> should succeed
	specRootAllowed := &ExecutionSpec{
		Command:      []string{"/bin/true"},
		Cwd:          "/",
		AllowHomeCwd: true,
	}
	if err := d.ValidateSpec(specRootAllowed); err != nil {
		t.Errorf("ValidateSpec with Cwd=/ and AllowHomeCwd=true failed: %v", err)
	}

	// 5. Cwd == subdirectory of $HOME and AllowHomeCwd == false -> should succeed
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
	// Test applyResourceLimits with various spec configurations
	specDefault := &ExecutionSpec{}
	if err := applyResourceLimits(specDefault); err != nil {
		t.Errorf("applyResourceLimits(default) failed: %v", err)
	}

	specCustom := &ExecutionSpec{
		MaxPIDs: 1024,
		Timeout: 5 * time.Second,
	}
	if err := applyResourceLimits(specCustom); err != nil {
		t.Errorf("applyResourceLimits(custom) failed: %v", err)
	}
}
