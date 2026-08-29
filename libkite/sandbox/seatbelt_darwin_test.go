//go:build darwin

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSeatbeltDriver_RegistrationAndAvailability(t *testing.T) {
	d, err := Get(DriverSeatbelt)
	if err != nil {
		t.Fatalf("Get(DriverSeatbelt) failed: %v", err)
	}

	if d.Name() != DriverSeatbelt {
		t.Errorf("d.Name() = %s, want %s", d.Name(), DriverSeatbelt)
	}

	if !d.Available() {
		t.Fatalf("expected SeatbeltDriver to be available on darwin")
	}
}

func TestSeatbeltDriver_ExecBasic(t *testing.T) {
	d, err := Resolve(DriverDefault)
	if err != nil {
		t.Fatalf("Resolve(DriverDefault) error: %v", err)
	}

	spec := &ExecutionSpec{
		Command: []string{"/bin/echo", "hello-seatbelt"},
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("Exec failed: %v, stderr=%s", err, res.Stderr)
	}

	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}

	if !strings.Contains(res.Stdout, "hello-seatbelt") {
		t.Errorf("Stdout = %q, want 'hello-seatbelt'", res.Stdout)
	}
}

func TestSeatbeltDriver_FilesystemIsolation(t *testing.T) {
	tempDir := t.TempDir()
	allowedFile := filepath.Join(tempDir, "allowed.txt")
	if err := os.WriteFile(allowedFile, []byte("allowed-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Get(DriverSeatbelt)
	if err != nil {
		t.Fatalf("Get(DriverSeatbelt) error: %v", err)
	}

	// 1. Test reading allowed file
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

	// 2. Test writing to read-only directory (must fail / be denied)
	writeSpec := &ExecutionSpec{
		Command: []string{"/bin/sh", "-c", "echo 'bad' > " + filepath.Join(tempDir, "blocked.txt")},
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

	resWrite, _ := d.Exec(context.Background(), writeSpec)
	if resWrite.ExitCode == 0 {
		// Verify file was NOT written
		if _, err := os.Stat(filepath.Join(tempDir, "blocked.txt")); err == nil {
			t.Errorf("security violation: file was written into read-only mount!")
		}
	}
}
