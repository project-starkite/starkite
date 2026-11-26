//go:build unix

package runtime

import (
	"fmt"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/sys/unix"
)

// uname returns system information as a struct.
func (m *Module) uname(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("uname takes no arguments")
	}

	var utsname unix.Utsname
	if err := unix.Uname(&utsname); err != nil {
		return nil, err
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"system":  starlark.String(bytesToString(utsname.Sysname[:])),
		"node":    starlark.String(bytesToString(utsname.Nodename[:])),
		"release": starlark.String(bytesToString(utsname.Release[:])),
		"version": starlark.String(bytesToString(utsname.Version[:])),
		"machine": starlark.String(bytesToString(utsname.Machine[:])),
	}), nil
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
