//go:build windows

package sandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var leaseMutex sync.Mutex

type windowsLease struct {
	PID                 int       `json:"pid"`
	ProcessCreationTime int64     `json:"process_creation_time,omitempty"`
	ProfileName         string    `json:"profile_name"`
	AppContainerSID     string    `json:"appcontainer_sid"`
	ScratchDir          string    `json:"scratch_dir,omitempty"`
	Paths               []string  `json:"paths"`
	CreatedAt           time.Time `json:"created_at"`
}

func getLeaseFilePath() string {
	base := os.Getenv("USERPROFILE")
	if base == "" {
		base = os.Getenv("LOCALAPPDATA")
	}
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, ".starkite", "run", "windows-leases.json")
}

func withLeaseLock(path string, fn func() error) error {
	leaseMutex.Lock()
	defer leaseMutex.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("sandbox: create lease directory %q: %w", dir, err)
	}

	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("sandbox: open lease lock %q: %w", lockPath, err)
	}
	defer func() {
		_ = f.Close()
	}()

	var ol windows.Overlapped
	handle := windows.Handle(f.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &ol); err != nil {
		return fmt.Errorf("sandbox: acquire lease lock %q: %w", lockPath, err)
	}
	defer func() {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, &ol)
	}()

	return fn()
}

func loadWindowsLeases(path string) ([]windowsLease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var leases []windowsLease
	if err := json.Unmarshal(data, &leases); err != nil {
		return nil, err
	}
	return leases, nil
}

func loadWindowsLeasesOrBackupCorrupt(path string) ([]windowsLease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("sandbox: read lease journal %q: %w", path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var leases []windowsLease
	if err := json.Unmarshal(data, &leases); err != nil {
		backupPath := fmt.Sprintf("%s.corrupt.%s", path, time.Now().UTC().Format("20060102T150405Z"))
		if renameErr := os.Rename(path, backupPath); renameErr != nil {
			return nil, fmt.Errorf("sandbox: corrupt lease journal %q (backup failed: %w): %w", path, renameErr, err)
		}
		return nil, nil
	}
	return leases, nil
}

func saveWindowsLeases(path string, leases []windowsLease) error {
	if len(leases) == 0 {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(leases, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "windows-leases-*.tmp")
	if err != nil {
		return fmt.Errorf("sandbox: create temp lease journal in %q: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("sandbox: commit lease journal %q: %w", path, err)
	}
	return nil
}

func recordWindowsLease(lease windowsLease) error {
	return recordWindowsLeaseAt(getLeaseFilePath(), lease)
}

func recordWindowsLeaseAt(path string, lease windowsLease) error {
	return withLeaseLock(path, func() error {
		leases, err := loadWindowsLeasesOrBackupCorrupt(path)
		if err != nil {
			return err
		}
		leases = append(leases, lease)
		return saveWindowsLeases(path, leases)
	})
}

func removeWindowsLease(profileName string) error {
	return removeWindowsLeaseAt(getLeaseFilePath(), profileName)
}

func removeWindowsLeaseAt(path, profileName string) error {
	return withLeaseLock(path, func() error {
		leases, err := loadWindowsLeasesOrBackupCorrupt(path)
		if err != nil {
			return err
		}
		if len(leases) == 0 {
			return nil
		}

		filtered := make([]windowsLease, 0, len(leases))
		for _, l := range leases {
			if l.ProfileName != profileName {
				filtered = append(filtered, l)
			}
		}
		return saveWindowsLeases(path, filtered)
	})
}

// sweepStaleWindowsLeases inspects the lease journal for orphaned leases from processes
// that are no longer running, revokes their DACL ACEs, deletes their scratch dirs and
// AppContainer profiles, and purges their lease records.
func sweepStaleWindowsLeases() error {
	return sweepStaleWindowsLeasesAt(getLeaseFilePath())
}

func sweepStaleWindowsLeasesAt(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("sandbox: stat lease journal %q: %w", path, err)
	}

	return withLeaseLock(path, func() error {
		leases, err := loadWindowsLeasesOrBackupCorrupt(path)
		if err != nil {
			return err
		}
		if len(leases) == 0 {
			return nil
		}

		var active []windowsLease
		var changed bool
		var sweepErrs []error
		for _, l := range leases {
			if isLeaseProcessAlive(l) {
				active = append(active, l)
				continue
			}

			var leaseErrs []error
			var remainingPaths []string
			if l.AppContainerSID != "" {
				sid, sidErr := windows.StringToSid(l.AppContainerSID)
				if sidErr != nil {
					leaseErrs = append(leaseErrs, fmt.Errorf("parse SID %q: %w", l.AppContainerSID, sidErr))
				} else {
					for _, p := range l.Paths {
						if err := revokePathAccess(p, sid); err != nil {
							remainingPaths = append(remainingPaths, p)
							leaseErrs = append(leaseErrs, err)
						}
					}
				}
			}

			scratchCleaned := true
			if l.ScratchDir != "" && isManagedScratchDir(l.ScratchDir) {
				if err := os.RemoveAll(l.ScratchDir); err != nil && !errors.Is(err, os.ErrNotExist) {
					scratchCleaned = false
					leaseErrs = append(leaseErrs, fmt.Errorf("remove scratch dir %q: %w", l.ScratchDir, err))
				}
			}

			profileCleaned := true
			if l.ProfileName != "" {
				if err := deleteAppContainerProfileByName(l.ProfileName); err != nil {
					profileCleaned = false
					leaseErrs = append(leaseErrs, err)
				}
			}

			if len(leaseErrs) > 0 {
				l.Paths = remainingPaths
				if scratchCleaned {
					l.ScratchDir = ""
				}
				if profileCleaned {
					l.ProfileName = ""
				}
				active = append(active, l)
				changed = true
				sweepErrs = append(sweepErrs, leaseErrs...)
				continue
			}

			changed = true
		}

		if changed {
			if saveErr := saveWindowsLeases(path, active); saveErr != nil {
				sweepErrs = append(sweepErrs, saveErr)
			}
		}
		return errors.Join(sweepErrs...)
	})
}

func isManagedScratchDir(dir string) bool {
	if dir == "" {
		return false
	}
	return strings.HasPrefix(filepath.Base(filepath.Clean(dir)), "starkite-sbx-")
}

func filetimeToInt64(ft windows.Filetime) int64 {
	return (int64(ft.HighDateTime) << 32) | int64(ft.LowDateTime)
}

func getProcessCreationTime(pid int) (int64, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid %d", pid)
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = windows.CloseHandle(handle)
	}()

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	return filetimeToInt64(creation), nil
}

func isLeaseProcessAlive(l windowsLease) bool {
	if l.PID <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(l.PID))
	if err != nil {
		return false
	}
	defer func() {
		_ = windows.CloseHandle(handle)
	}()

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	// STILL_ACTIVE = 259
	if exitCode != 259 {
		return false
	}
	if l.ProcessCreationTime != 0 {
		var creation, exit, kernel, user windows.Filetime
		if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
			return false
		}
		if filetimeToInt64(creation) != l.ProcessCreationTime {
			return false
		}
	}
	return true
}
