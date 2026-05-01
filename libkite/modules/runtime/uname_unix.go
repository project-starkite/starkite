//go:build unix

package runtime

import (
	"fmt"

	"go.starlark.net/starlark"
	"golang.org/x/sys/unix"
)

// uname returns system information as a dict.
func (m *Module) uname(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("uname takes no arguments")
	}

	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return nil, err
	}

	dict := starlark.NewDict(5)
	dict.SetKey(starlark.String("system"), starlark.String(bytesToString(utsname.Sysname[:])))
	dict.SetKey(starlark.String("node"), starlark.String(bytesToString(utsname.Nodename[:])))
	dict.SetKey(starlark.String("release"), starlark.String(bytesToString(utsname.Release[:])))
	dict.SetKey(starlark.String("version"), starlark.String(bytesToString(utsname.Version[:])))
	dict.SetKey(starlark.String("machine"), starlark.String(bytesToString(utsname.Machine[:])))
	return dict, nil
}

// bytesToString converts a null-terminated byte slice to a Go string.
func bytesToString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
