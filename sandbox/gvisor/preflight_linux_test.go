//go:build linux

package gvisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFakeSysctls swaps the sysctl path variables to point at temp files
// so tests can simulate kernel state without sudo. Returns a cleanup that
// restores the originals.
func withFakeSysctls(t *testing.T, apparmor, userns string) {
	t.Helper()
	dir := t.TempDir()

	origApparmor := procApparmorRestrictUserns
	origUserns := procUnprivilegedUserns
	t.Cleanup(func() {
		procApparmorRestrictUserns = origApparmor
		procUnprivilegedUserns = origUserns
	})

	if apparmor != "" {
		p := filepath.Join(dir, "apparmor")
		if err := os.WriteFile(p, []byte(apparmor), 0o644); err != nil {
			t.Fatalf("write fake apparmor sysctl: %v", err)
		}
		procApparmorRestrictUserns = p
	} else {
		// Point at a non-existent path to simulate ENOENT.
		procApparmorRestrictUserns = filepath.Join(dir, "missing-apparmor")
	}

	if userns != "" {
		p := filepath.Join(dir, "userns")
		if err := os.WriteFile(p, []byte(userns), 0o644); err != nil {
			t.Fatalf("write fake userns sysctl: %v", err)
		}
		procUnprivilegedUserns = p
	} else {
		procUnprivilegedUserns = filepath.Join(dir, "missing-userns")
	}
}

func TestPreflight(t *testing.T) {
	tests := []struct {
		name       string
		apparmor   string // "" simulates missing file
		userns     string // ""
		wantErr    bool
		wantSubstr string // substring expected in the error message
	}{
		{name: "both files missing → ok (non-Ubuntu / older Ubuntu)"},
		{name: "apparmor=0 → ok", apparmor: "0\n"},
		{name: "userns=1 → ok", userns: "1\n"},
		{name: "apparmor=0, userns=1 → ok", apparmor: "0", userns: "1"},
		{
			name:       "apparmor=1 → friendly error",
			apparmor:   "1",
			wantErr:    true,
			wantSubstr: "apparmor_restrict_unprivileged_userns",
		},
		{
			name:       "userns=0 → friendly error",
			userns:     "0",
			wantErr:    true,
			wantSubstr: "unprivileged_userns_clone",
		},
		{
			// apparmor checked first, so it surfaces first.
			name:       "both restricted → apparmor error first",
			apparmor:   "1",
			userns:     "0",
			wantErr:    true,
			wantSubstr: "apparmor_restrict_unprivileged_userns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeSysctls(t, tt.apparmor, tt.userns)
			err := preflight()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantSubstr)
				}
				return
			}
			if err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
		})
	}
}

func TestReadSysctl_Missing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-file")
	_, err := readSysctl(missing)
	if err == nil {
		t.Fatal("expected error reading missing file")
	}
	// Just verify it returns an error; preflight wraps the missing case.
}
