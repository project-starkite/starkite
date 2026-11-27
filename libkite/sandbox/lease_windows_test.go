//go:build windows

package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsLeaseJournal_CorruptBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "windows-leases.json")
	if err := os.WriteFile(path, []byte("{corrupt json"), 0o600); err != nil {
		t.Fatalf("write corrupt journal: %v", err)
	}

	lease := windowsLease{
		PID:             99999999,
		ProfileName:     "starkite.sbx.recovered",
		AppContainerSID: "S-1-15-2-1",
		Paths:           []string{dir},
		ScratchDir:      filepath.Join(dir, "starkite-sbx-recovered"),
		CreatedAt:       time.Now().UTC(),
	}
	if err := recordWindowsLeaseAt(path, lease); err != nil {
		t.Fatalf("recordWindowsLeaseAt failed on corrupt journal: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var backedUp bool
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "windows-leases.json.corrupt.") {
			backedUp = true
			break
		}
	}
	if !backedUp {
		t.Errorf("expected corrupt journal backup file to be created")
	}

	loaded, err := loadWindowsLeases(path)
	if err != nil {
		t.Fatalf("loadWindowsLeases: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ProfileName != lease.ProfileName || loaded[0].ScratchDir != lease.ScratchDir {
		t.Errorf("unexpected loaded leases: %+v", loaded)
	}
}

func TestWindowsLeaseJournal_SweepScratchAndPIDReuse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "windows-leases.json")

	liveScratch := filepath.Join(dir, "starkite-sbx-live")
	if err := os.MkdirAll(liveScratch, 0o700); err != nil {
		t.Fatalf("mkdir liveScratch: %v", err)
	}
	deadScratch := filepath.Join(dir, "starkite-sbx-dead")
	if err := os.MkdirAll(deadScratch, 0o700); err != nil {
		t.Fatalf("mkdir deadScratch: %v", err)
	}

	creationTime, err := getProcessCreationTime(os.Getpid())
	if err != nil {
		t.Fatalf("getProcessCreationTime(self): %v", err)
	}

	leases := []windowsLease{
		{
			// Live current process with matching creation time must be retained.
			PID:                 os.Getpid(),
			ProcessCreationTime: creationTime,
			ProfileName:         "starkite.sbx.live",
			ScratchDir:          liveScratch,
			CreatedAt:           time.Now().UTC(),
		},
		{
			// Same live PID with mismatched creation time simulates PID reuse: must be swept.
			PID:                 os.Getpid(),
			ProcessCreationTime: creationTime + 12345,
			ProfileName:         "starkite.sbx.reusedpid",
			ScratchDir:          deadScratch,
			CreatedAt:           time.Now().UTC(),
		},
	}
	data, err := json.Marshal(leases)
	if err != nil {
		t.Fatalf("marshal leases: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write lease file: %v", err)
	}

	if err := sweepStaleWindowsLeasesAt(path); err != nil {
		t.Fatalf("sweepStaleWindowsLeasesAt failed: %v", err)
	}

	if _, err := os.Stat(deadScratch); !os.IsNotExist(err) {
		t.Errorf("expected stale PID-reused scratch dir %q to be removed, err=%v", deadScratch, err)
	}
	if _, err := os.Stat(liveScratch); err != nil {
		t.Errorf("expected active scratch dir %q to be preserved, err=%v", liveScratch, err)
	}

	remaining, err := loadWindowsLeases(path)
	if err != nil {
		t.Fatalf("loadWindowsLeases: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ProfileName != "starkite.sbx.live" {
		t.Errorf("expected only live lease to remain, got %+v", remaining)
	}
}
