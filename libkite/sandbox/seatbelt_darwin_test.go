//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"errors"
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

func TestSeatbeltDriver_StreamedStderrEnrichment(t *testing.T) {
	d, err := Get(DriverSeatbelt)
	if err != nil {
		t.Fatalf("Get(DriverSeatbelt) error: %v", err)
	}

	var stderrBuf bytes.Buffer
	spec := &ExecutionSpec{
		Command: []string{"/bin/sh", "-c", "echo 'permission denied' >&2; exit 1"},
		Stderr:  &stderrBuf,
		Timeout: 5 * time.Second,
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("Exec returned unexpected error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "sandbox:") || !strings.Contains(stderrBuf.String(), "sandbox:") {
		t.Errorf("expected enriched diagnostic in both res.Stderr and spec.Stderr;\nres.Stderr=%q\nspec.Stderr=%q", res.Stderr, stderrBuf.String())
	}
}

func TestSeatbeltDriver_CancelKillsProcessGroupAndReturnsErr(t *testing.T) {
	d, err := Get(DriverSeatbelt)
	if err != nil {
		t.Fatalf("Get(DriverSeatbelt) error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	spec := &ExecutionSpec{
		Command: []string{"/bin/sh", "-c", "sleep 30"},
	}

	start := time.Now()
	_, err = d.Exec(ctx, spec)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation error, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("process group cancellation took too long (%v)", elapsed)
	}
}

func TestSeatbeltDriver_NestedPolicyDenialReturnsResultNilErr(t *testing.T) {
	t.Setenv(InsideEnvVar, "1")
	d, err := Get(DriverSeatbelt)
	if err != nil {
		t.Fatalf("Get(DriverSeatbelt) error: %v", err)
	}

	var stderrBuf bytes.Buffer
	spec := &ExecutionSpec{
		Command: []string{"/bin/echo", "hi"},
		NoExec:  true,
		Stderr:  &stderrBuf,
	}
	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("expected nil error on policy denial, got %v", err)
	}
	if res == nil || res.ExitCode != 126 {
		t.Fatalf("expected ExitCode 126, got %+v", res)
	}
	if stderrBuf.Len() == 0 || res.Stderr == "" {
		t.Errorf("expected denial message in both res.Stderr and spec.Stderr")
	}
}
