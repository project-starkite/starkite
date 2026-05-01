//go:build windows

package runtime

import (
	"fmt"
	"os"
	goruntime "runtime"

	"go.starlark.net/starlark"
)

// uname returns system information as a dict.
// On Windows, syscall.Utsname is not available, so we return
// what we can from the Go runtime and os packages.
func (m *Module) uname(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("uname takes no arguments")
	}

	hostname, _ := os.Hostname()

	dict := starlark.NewDict(5)
	dict.SetKey(starlark.String("system"), starlark.String(goruntime.GOOS))
	dict.SetKey(starlark.String("node"), starlark.String(hostname))
	dict.SetKey(starlark.String("release"), starlark.String(""))
	dict.SetKey(starlark.String("version"), starlark.String(""))
	dict.SetKey(starlark.String("machine"), starlark.String(goruntime.GOARCH))
	return dict, nil
}
