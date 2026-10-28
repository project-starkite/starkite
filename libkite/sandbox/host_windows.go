//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// minWindowsBuild24H2 is Windows 11 Version 24H2 / Windows Server 2025 (Build 26100).
	minWindowsBuild24H2 = 26100

	// filePersistentACLs indicates that the volume preserves and enforces ACLs (NTFS/ReFS).
	filePersistentACLs = 0x00000008
)

var (
	modKernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procGetVolumePathNameW     = modKernel32.NewProc("GetVolumePathNameW")
	procGetVolumeInformationW  = modKernel32.NewProc("GetVolumeInformationW")
	procCreateJobObjectW       = modKernel32.NewProc("CreateJobObjectW")
	procInitProcThreadAttrList = modKernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateProcThreadAttr   = modKernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteProcThreadAttr   = modKernel32.NewProc("DeleteProcThreadAttributeList")
)

func checkWindowsOSBaseline() error {
	major, _, build := windows.RtlGetNtVersionNumbers()
	return validateWindowsVersion(major, build)
}

func validateWindowsVersion(major, build uint32) error {
	if major < 10 || build < minWindowsBuild24H2 {
		return fmt.Errorf(
			"sandbox: host environment does not support required Windows kernel sandbox primitives: Windows build %d < %d (requires Windows 11 24H2+ or Windows Server 2025+)\n  Error: Refusing to execute insecurely under default Zero-Trust policy.\n  Hint: To run unconfined on this unsupported host, explicitly disable sandboxing via --sandbox-enabled=false on CLI or 'sandbox: false' in config.yaml",
			build, minWindowsBuild24H2,
		)
	}
	return nil
}

func checkWindowsKernelPrimitives() error {
	requiredProcs := []struct {
		dll  string
		name string
		proc *windows.LazyProc
	}{
		{"userenv.dll", "CreateAppContainerProfile", procCreateAppContainerProfile},
		{"userenv.dll", "DeleteAppContainerProfile", procDeleteAppContainerProfile},
		{"userenv.dll", "DeriveAppContainerSidFromAppContainerName", procDeriveAppContainerSidFromAppContainerName},
		{"advapi32.dll", "SetEntriesInAclW", procSetEntriesInAclW},
		{"kernel32.dll", "CreateJobObjectW", procCreateJobObjectW},
		{"kernel32.dll", "InitializeProcThreadAttributeList", procInitProcThreadAttrList},
		{"kernel32.dll", "UpdateProcThreadAttribute", procUpdateProcThreadAttr},
		{"kernel32.dll", "DeleteProcThreadAttributeList", procDeleteProcThreadAttr},
	}
	for _, p := range requiredProcs {
		if err := p.proc.Find(); err != nil {
			return fmt.Errorf(
				"sandbox: host environment does not support required Windows kernel sandbox primitives: missing %s!%s (%w)\n  Error: Refusing to execute insecurely under default Zero-Trust policy.\n  Hint: To run unconfined on this unsupported host, explicitly disable sandboxing via --sandbox-enabled=false on CLI or 'sandbox: false' in config.yaml",
				p.dll, p.name, err,
			)
		}
	}
	return nil
}

func checkWindowsHostBaseline() error {
	if err := checkWindowsOSBaseline(); err != nil {
		return err
	}
	if err := checkWindowsKernelPrimitives(); err != nil {
		return err
	}
	return nil
}

func getVolumeFilesystemInfo(path string) (fsName string, flags uint32, err error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", 0, err
	}
	pathPtr, err := windows.UTF16PtrFromString(absPath)
	if err != nil {
		return "", 0, err
	}

	volRootBuf := make([]uint16, 512)
	r1, _, err := procGetVolumePathNameW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&volRootBuf[0])),
		uintptr(len(volRootBuf)),
	)
	if r1 == 0 {
		return "", 0, fmt.Errorf("GetVolumePathNameW failed for %q: %w", absPath, err)
	}

	fsNameBuf := make([]uint16, 256)
	var fsFlags uint32
	r2, _, err := procGetVolumeInformationW.Call(
		uintptr(unsafe.Pointer(&volRootBuf[0])),
		0, 0, 0, 0,
		uintptr(unsafe.Pointer(&fsFlags)),
		uintptr(unsafe.Pointer(&fsNameBuf[0])),
		uintptr(len(fsNameBuf)),
	)
	if r2 == 0 {
		return "", 0, fmt.Errorf("GetVolumeInformationW failed for %q: %w", windows.UTF16ToString(volRootBuf), err)
	}

	return windows.UTF16ToString(fsNameBuf), fsFlags, nil
}

func validateVolumeACLSupport(targetPath, fsName string, fsFlags uint32) error {
	if fsFlags&filePersistentACLs == 0 {
		if fsName == "" {
			fsName = "unknown"
		}
		return fmt.Errorf(
			"sandbox: path %q resides on a %q volume that does not support persistent ACLs (NTFS / FILE_PERSISTENT_ACLS required for AppContainer DACL isolation)\n  Error: Refusing to execute insecurely under default Zero-Trust policy.\n  Hint: Move the workspace to an NTFS volume, or explicitly disable sandboxing via --sandbox-enabled=false on CLI or 'sandbox: false' in config.yaml",
			targetPath, fsName,
		)
	}
	return nil
}

func checkPathSupportsACLs(path string) error {
	fsName, flags, err := getVolumeFilesystemInfo(path)
	if err != nil {
		// Non-fatal if path doesn't exist yet on disk
		if os.IsNotExist(err) || errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
			return nil
		}
		return err
	}
	return validateVolumeACLSupport(path, fsName, flags)
}

func validateSpecVolumes(spec *ExecutionSpec) error {
	if spec == nil {
		return nil
	}

	checkedVols := make(map[string]bool)

	checkOne := func(p string) error {
		if p == "" {
			return nil
		}
		clean := filepath.Clean(p)
		if abs, err := filepath.Abs(clean); err == nil {
			clean = abs
		}
		vol := strings.ToLower(filepath.VolumeName(clean))
		if vol == "" {
			vol = strings.ToLower(clean)
		}
		if checkedVols[vol] {
			return nil
		}
		if _, err := os.Stat(clean); err != nil {
			return nil // do not block on uncreated paths
		}
		if err := checkPathSupportsACLs(clean); err != nil {
			return err
		}
		checkedVols[vol] = true
		return nil
	}

	if err := checkOne(os.TempDir()); err != nil {
		return err
	}
	if err := checkOne(spec.Cwd); err != nil {
		return err
	}
	if err := checkOne(spec.ScriptFile); err != nil {
		return err
	}
	for _, m := range spec.Mounts {
		if err := checkOne(m.Source); err != nil {
			return err
		}
	}
	return nil
}
