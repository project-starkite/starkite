//go:build unix

package fs

import (
	"syscall"

	"go.starlark.net/starlark"
)

// diskUsageInfo returns disk usage stats for the given path as a Starlark dict.
func diskUsageInfo(path string) (*starlark.Dict, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	dict := starlark.NewDict(3)
	dict.SetKey(starlark.String("total"), starlark.MakeInt64(int64(total)))
	dict.SetKey(starlark.String("used"), starlark.MakeInt64(int64(used)))
	dict.SetKey(starlark.String("free"), starlark.MakeInt64(int64(free)))
	return dict, nil
}
