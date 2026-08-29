//go:build darwin

package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// SeatbeltDriver implements the sandbox.Driver interface for macOS using
// Apple's kernel-level Seatbelt Mandatory Access Control facility.
type SeatbeltDriver struct {
	once        sync.Once
	initFn      func(profile *byte, flags uint64, errorbuf **byte) int
	freeErrorFn func(errorbuf *byte)
	available   bool
}

func init() {
	Register(NewSeatbeltDriver())
}

// NewSeatbeltDriver creates a new SeatbeltDriver and initializes dynamic symbols.
func NewSeatbeltDriver() *SeatbeltDriver {
	d := &SeatbeltDriver{}
	d.initSymbols()
	return d
}

func (d *SeatbeltDriver) initSymbols() {
	d.once.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			d.available = false
			return
		}
		purego.RegisterLibFunc(&d.initFn, lib, "sandbox_init")
		purego.RegisterLibFunc(&d.freeErrorFn, lib, "sandbox_free_error")
		d.available = (d.initFn != nil)
	})
}

// Name returns the driver identifier "seatbelt".
func (d *SeatbeltDriver) Name() string {
	return DriverSeatbelt
}

// Available reports whether the Seatbelt subsystem is accessible on this macOS system.
func (d *SeatbeltDriver) Available() bool {
	d.initSymbols()
	return d.available
}

// ValidateSpec verifies that the ExecutionSpec is valid for Seatbelt isolation.
func (d *SeatbeltDriver) ValidateSpec(spec *ExecutionSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	return nil
}

// ApplyInProcess applies the sandbox policy directly to the current running process
// using the dynamic purego sandbox_init binding. Once applied, restrictions are irreversible.
func (d *SeatbeltDriver) ApplyInProcess(spec *ExecutionSpec) error {
	if !d.Available() {
		return errors.New("sandbox: seatbelt driver not available")
	}

	sbpl := GenerateSeatbeltSBPL(spec)
	cProfile, err := stringToNullTerminatedBytes(sbpl)
	if err != nil {
		return fmt.Errorf("sandbox: failed to serialize seatbelt profile: %w", err)
	}

	var errorBuf *byte
	ret := d.initFn(&cProfile[0], 0, &errorBuf)
	if ret != 0 {
		var errMsg string
		if errorBuf != nil {
			errMsg = goStringFromPointer(errorBuf)
			if d.freeErrorFn != nil {
				d.freeErrorFn(errorBuf)
			}
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("seatbelt sandbox_init returned error code %d", ret)
		}
		return fmt.Errorf("sandbox: seatbelt error: %s", errMsg)
	}

	return nil
}

// Exec runs a child command inside a contained Seatbelt sandbox using the
// system sandbox-exec wrapper with dynamically generated SBPL profiles.
func (d *SeatbeltDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := d.ValidateSpec(spec); err != nil {
		return nil, err
	}

	start := time.Now()
	sbpl := GenerateSeatbeltSBPL(spec)

	execCtx := ctx
	var cancel context.CancelFunc
	if spec.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	// Prepare /usr/bin/sandbox-exec invocation
	args := append([]string{"-p", sbpl, "--"}, spec.Command...)
	cmd := exec.CommandContext(execCtx, "/usr/bin/sandbox-exec", args...)

	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}
	if len(spec.Env) > 0 {
		cmd.Env = spec.Env
	} else {
		cmd.Env = os.Environ()
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	if spec.Stdout != nil {
		cmd.Stdout = spec.Stdout
	} else {
		cmd.Stdout = &stdoutBuf
	}

	if spec.Stderr != nil {
		cmd.Stderr = spec.Stderr
	} else {
		cmd.Stderr = &stderrBuf
	}

	if spec.Stdin != nil {
		cmd.Stdin = spec.Stdin
	}

	err := cmd.Run()
	duration := time.Since(start)

	result := &ExecResult{
		Duration: duration,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}

	if execCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = 124
		return result, fmt.Errorf("sandbox: command timed out after %v", spec.Timeout)
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, fmt.Errorf("sandbox: seatbelt exec failed: %w", err)
	}

	return result, nil
}

func stringToNullTerminatedBytes(s string) ([]byte, error) {
	b := make([]byte, len(s)+1)
	copy(b, s)
	b[len(s)] = 0
	return b, nil
}

func goStringFromPointer(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var b []byte
	for i := 0; ; i++ {
		val := *(*byte)(unsafe.Add(unsafe.Pointer(ptr), i))
		if val == 0 {
			break
		}
		b = append(b, val)
	}
	return string(b)
}
