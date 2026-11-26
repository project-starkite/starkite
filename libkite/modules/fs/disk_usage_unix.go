//go:build unix

package fs

import (
	"syscall"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// diskUsageInfo returns disk usage stats for the given path as a struct.
func diskUsageInfo(path string) (*starlarkstruct.Struct, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"total": starlark.MakeInt64(int64(total)),
		"used":  starlark.MakeInt64(int64(used)),
		"free":  starlark.MakeInt64(int64(free)),
	}), nil
}
