//go:build windows

package fs

import (
	"fmt"
	"unsafe"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"golang.org/x/sys/windows"
)

// diskUsageInfo returns disk usage stats for the given path as a struct.
func diskUsageInfo(path string) (*starlarkstruct.Struct, error) {
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

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"total": starlark.MakeInt64(int64(totalBytes)),
		"used":  starlark.MakeInt64(int64(totalBytes - totalFreeBytes)),
		"free":  starlark.MakeInt64(int64(totalFreeBytes)),
	}), nil
}
