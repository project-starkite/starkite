//go:build !linux && !darwin

package sandbox

import (
	"context"
	"fmt"
	"runtime"
)

// FallbackDriver is a placeholder driver for operating systems without
// native kernel-level sandboxing (e.g. Windows).
type FallbackDriver struct{}

func init() {
	Register(&FallbackDriver{})
}

// Name returns the driver identifier "fallback".
func (d *FallbackDriver) Name() string {
	return "fallback"
}

// Available returns false on unsupported platforms.
func (d *FallbackDriver) Available() bool {
	return false
}

// ValidateSpec returns an unsupported error on non-Linux/non-Darwin platforms.
func (d *FallbackDriver) ValidateSpec(spec *ExecutionSpec) error {
	return fmt.Errorf("sandbox: native kernel sandboxing is not supported on %s; use container isolation", runtime.GOOS)
}

// Exec returns an unsupported error on non-Linux/non-Darwin platforms.
func (d *FallbackDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	return nil, fmt.Errorf("sandbox: native kernel sandboxing is not supported on %s; use container isolation", runtime.GOOS)
}
