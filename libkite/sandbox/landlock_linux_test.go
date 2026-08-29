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
