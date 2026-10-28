//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modAdvapi32          = windows.NewLazySystemDLL("advapi32.dll")
	procSetEntriesInAclW = modAdvapi32.NewProc("SetEntriesInAclW")
)

const (
	// Standard Windows NTFS access masks for sandboxed paths.
	readOnlyAccessMask    = windows.FILE_GENERIC_READ
	readWriteAccessMask   = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.DELETE
	execAccessMask        = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE
	denyExecAccessMask    = windows.FILE_EXECUTE
	traverseDirAccessMask = windows.FILE_LIST_DIRECTORY | windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.READ_CONTROL | windows.SYNCHRONIZE
)

// grantPathAccess adds an ACCESS_ALLOWED_ACE for sid to the DACL of the target path.
// When the path is a directory, inheritance flags (OBJECT_INHERIT_ACE | CONTAINER_INHERIT_ACE)
// are applied so child files and subdirectories inherit the grant.
func grantPathAccess(path string, sid *windows.SID, accessMask windows.ACCESS_MASK) error {
	return modifyPathDacl(path, sid, accessMask, windows.GRANT_ACCESS)
}

// grantDirTraverse adds a non-inheriting directory traverse ACE for sid so the directory
// can serve as the process working directory without exposing sibling files inside it.
func grantDirTraverse(path string, sid *windows.SID) error {
	return modifyPathDaclWithInheritance(path, sid, traverseDirAccessMask, windows.GRANT_ACCESS, windows.NO_INHERITANCE)
}

// denyPathAccess adds an explicit ACCESS_DENIED_ACE for sid to the DACL of the target path.
func denyPathAccess(path string, sid *windows.SID, accessMask windows.ACCESS_MASK) error {
	return modifyPathDacl(path, sid, accessMask, windows.DENY_ACCESS)
}

// setScratchDirDacl configures a scratch directory with read/write access and an explicit
// W^X FILE_EXECUTE denial in a single Win32 ACL transaction.
func setScratchDirDacl(path string, sid *windows.SID) error {
	if sid == nil || path == "" {
		return nil
	}
	cleanPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		cleanPath = filepath.Clean(path)
	}

	inheritance := uint32(windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE)
	trustee := windows.TRUSTEE{
		TrusteeForm:  windows.TRUSTEE_IS_SID,
		TrusteeType:  windows.TRUSTEE_IS_USER,
		TrusteeValue: windows.TrusteeValueFromSID(sid),
	}

	entries := [2]windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: readWriteAccessMask,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       inheritance,
			Trustee:           trustee,
		},
		{
			AccessPermissions: denyExecAccessMask,
			AccessMode:        windows.DENY_ACCESS,
			Inheritance:       inheritance,
			Trustee:           trustee,
		},
	}

	return applyExplicitEntries(cleanPath, entries[:])
}

// revokePathAccess removes explicit ACEs for sid from the DACL of the target path.
func revokePathAccess(path string, sid *windows.SID) error {
	return modifyPathDacl(path, sid, 0, windows.REVOKE_ACCESS)
}

func modifyPathDacl(path string, sid *windows.SID, accessMask windows.ACCESS_MASK, mode windows.ACCESS_MODE) error {
	if sid == nil || path == "" {
		return nil
	}

	cleanPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		cleanPath = filepath.Clean(path)
	}

	fi, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var inheritance uint32 = windows.NO_INHERITANCE
	if fi.IsDir() && mode != windows.REVOKE_ACCESS {
		inheritance = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}

	return modifyCleanedPathDacl(cleanPath, sid, accessMask, mode, inheritance)
}

func modifyPathDaclWithInheritance(path string, sid *windows.SID, accessMask windows.ACCESS_MASK, mode windows.ACCESS_MODE, inheritance uint32) error {
	if sid == nil || path == "" {
		return nil
	}

	cleanPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		cleanPath = filepath.Clean(path)
	}

	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return modifyCleanedPathDacl(cleanPath, sid, accessMask, mode, inheritance)
}

func modifyCleanedPathDacl(cleanPath string, sid *windows.SID, accessMask windows.ACCESS_MASK, mode windows.ACCESS_MODE, inheritance uint32) error {
	ea := [1]windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: accessMask,
			AccessMode:        mode,
			Inheritance:       inheritance,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		},
	}
	return applyExplicitEntries(cleanPath, ea[:])
}

func applyExplicitEntries(cleanPath string, entries []windows.EXPLICIT_ACCESS) error {
	if len(entries) == 0 {
		return nil
	}

	sd, err := windows.GetNamedSecurityInfo(
		cleanPath,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if err != nil {
		return fmt.Errorf("sandbox: GetNamedSecurityInfo(%s): %w", cleanPath, err)
	}

	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("sandbox: get DACL (%s): %w", cleanPath, err)
	}

	var newDacl *windows.ACL
	r1, _, _ := procSetEntriesInAclW.Call(
		uintptr(len(entries)),
		uintptr(unsafe.Pointer(&entries[0])),
		uintptr(unsafe.Pointer(dacl)),
		uintptr(unsafe.Pointer(&newDacl)),
	)
	if r1 != 0 {
		return fmt.Errorf("sandbox: SetEntriesInAclW(%s): %w", cleanPath, windows.Errno(r1))
	}
	if newDacl != nil {
		defer windows.LocalFree(windows.Handle(unsafe.Pointer(newDacl)))
	}

	err = windows.SetNamedSecurityInfo(
		cleanPath,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil,
		nil,
		newDacl,
		nil,
	)
	if err != nil {
		return fmt.Errorf("sandbox: SetNamedSecurityInfo(%s): %w", cleanPath, err)
	}

	return nil
}
