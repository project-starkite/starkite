//go:build windows

package runtime

import (
	"fmt"
	"os"
	goruntime "runtime"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// uname returns system information as a struct.
// On Windows, syscall.Utsname is not available; this returns the
// subset available from the Go runtime and os packages.
func (m *Module) uname(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("uname takes no arguments")
	}

	hostname, _ := os.Hostname()

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"system":  starlark.String(goruntime.GOOS),
		"node":    starlark.String(hostname),
		"release": starlark.String(""),
		"version": starlark.String(""),
		"machine": starlark.String(goruntime.GOARCH),
	}), nil
}
