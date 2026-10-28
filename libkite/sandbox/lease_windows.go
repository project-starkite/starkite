//go:build windows

package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

var leaseMutex sync.Mutex

type windowsLease struct {
	PID             int       `json:"pid"`
	ProfileName     string    `json:"profile_name"`
	AppContainerSID string    `json:"appcontainer_sid"`
	Paths           []string  `json:"paths"`
	CreatedAt       time.Time `json:"created_at"`
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
		return err
	}

	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fn()
	}
	defer f.Close()

	var ol windows.Overlapped
	handle := windows.Handle(f.Fd())
	if err := windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &ol); err == nil {
		defer func() {
			_ = windows.UnlockFileEx(handle, 0, 1, 0, &ol)
		}()
	}

	return fn()
}

func loadWindowsLeases(path string) ([]windowsLease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
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

func saveWindowsLeases(path string, leases []windowsLease) error {
	if len(leases) == 0 {
		err := os.Remove(path)
		if os.IsNotExist(err) {
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
		return os.WriteFile(path, data, 0600)
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
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
		return os.WriteFile(path, data, 0600)
	}
	return nil
}

func recordWindowsLease(lease windowsLease) error {
	path := getLeaseFilePath()
	return withLeaseLock(path, func() error {
		leases, _ := loadWindowsLeases(path)
		leases = append(leases, lease)
		return saveWindowsLeases(path, leases)
	})
}

func removeWindowsLease(profileName string) error {
	path := getLeaseFilePath()
	return withLeaseLock(path, func() error {
		leases, err := loadWindowsLeases(path)
		if err != nil || len(leases) == 0 {
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
// that are no longer running, revokes their DACL ACEs, deletes their AppContainer profiles,
// and purges their lease records.
func sweepStaleWindowsLeases() error {
	path := getLeaseFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	return withLeaseLock(path, func() error {
		leases, err := loadWindowsLeases(path)
		if err != nil || len(leases) == 0 {
			return nil
		}

		var active []windowsLease
		var changed bool
		for _, l := range leases {
			if isProcessAlive(l.PID) {
				active = append(active, l)
				continue
			}

			changed = true
			if l.AppContainerSID != "" {
				if sid, err := windows.StringToSid(l.AppContainerSID); err == nil {
					for _, p := range l.Paths {
						_ = revokePathAccess(p, sid)
					}
				}
			}
			if l.ProfileName != "" {
				_ = deleteAppContainerProfileByName(l.ProfileName)
			}
		}

		if !changed {
			return nil
		}
		return saveWindowsLeases(path, active)
	})
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	// STILL_ACTIVE = 259
	return exitCode == 259
}
