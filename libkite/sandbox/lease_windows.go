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

func loadWindowsLeases(path string) ([]windowsLease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var leases []windowsLease
	if err := json.Unmarshal(data, &leases); err != nil {
		return nil, err
	}
	return leases, nil
}

func saveWindowsLeases(path string, leases []windowsLease) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(leases, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func recordWindowsLease(lease windowsLease) error {
	leaseMutex.Lock()
	defer leaseMutex.Unlock()

	path := getLeaseFilePath()
	leases, _ := loadWindowsLeases(path)
	leases = append(leases, lease)
	return saveWindowsLeases(path, leases)
}

func removeWindowsLease(profileName string) error {
	leaseMutex.Lock()
	defer leaseMutex.Unlock()

	path := getLeaseFilePath()
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
}

// sweepStaleWindowsLeases inspects the lease journal for orphaned leases from processes
// that are no longer running, revokes their DACL ACEs, deletes their AppContainer profiles,
// and purges their lease records.
func sweepStaleWindowsLeases() error {
	leaseMutex.Lock()
	defer leaseMutex.Unlock()

	path := getLeaseFilePath()
	leases, err := loadWindowsLeases(path)
	if err != nil || len(leases) == 0 {
		return nil
	}

	var active []windowsLease
	for _, l := range leases {
		if isProcessAlive(l.PID) {
			active = append(active, l)
			continue
		}

		// Process is dead; clean up orphaned DACL grants and AppContainer profile.
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

	return saveWindowsLeases(path, active)
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
