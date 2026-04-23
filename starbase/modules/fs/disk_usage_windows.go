//go:build windows

package fs

import (
	"fmt"
	"unsafe"

	"go.starlark.net/starlark"
	"golang.org/x/sys/windows"
)

// diskUsageInfo returns disk usage stats for the given path as a Starlark dict.
func diskUsageInfo(path string) (*starlark.Dict, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(
		pathPtr,
		(*uint64)(unsafe.Pointer(&freeBytesAvailable)),
		(*uint64)(unsafe.Pointer(&totalBytes)),
		(*uint64)(unsafe.Pointer(&totalFreeBytes)),
	); err != nil {
		return nil, err
	}

	dict := starlark.NewDict(3)
	dict.SetKey(starlark.String("total"), starlark.MakeInt64(int64(totalBytes)))
	dict.SetKey(starlark.String("used"), starlark.MakeInt64(int64(totalBytes-totalFreeBytes)))
	dict.SetKey(starlark.String("free"), starlark.MakeInt64(int64(totalFreeBytes)))
	return dict, nil
}
