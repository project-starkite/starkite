package libkite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashModuleTree(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main(): pass\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "lib.star"), []byte("x = 1\n"), 0o644)

	h1, err := HashModuleTree(dir)
	if err != nil {
		t.Fatalf("HashModuleTree: %v", err)
	}
	if !strings.HasPrefix(h1, "sha256:") {
		t.Errorf("hash missing sha256: prefix: %q", h1)
	}

	// Deterministic: same tree → same hash.
	h2, _ := HashModuleTree(dir)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %q vs %q", h1, h2)
	}

	// Excluded files do not change the hash.
	os.WriteFile(filepath.Join(dir, LockFile), []byte("version: 1\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".mod.receipt"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: x"), 0o644)
	h3, _ := HashModuleTree(dir)
	if h3 != h1 {
		t.Errorf("excluded files changed the hash: %q vs %q", h3, h1)
	}

	// A content change does change the hash.
	os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main(): return 1\n"), 0o644)
	h4, _ := HashModuleTree(dir)
	if h4 == h1 {
		t.Error("content change did not change the hash")
	}
}

func TestFingerprintTree(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main(): pass\n"), 0o644)

	fp1, err := FingerprintTree(dir)
	if err != nil {
		t.Fatalf("FingerprintTree: %v", err)
	}
	if !strings.HasPrefix(fp1, "fp1:") {
		t.Errorf("fingerprint missing fp1: prefix: %q", fp1)
	}

	// Deterministic for an unchanged tree.
	fp2, _ := FingerprintTree(dir)
	if fp1 != fp2 {
		t.Errorf("fingerprint not stable: %q vs %q", fp1, fp2)
	}

	// Excluded files do not affect the fingerprint.
	os.WriteFile(filepath.Join(dir, ".mod.receipt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, LockFile), []byte("version: 1\n"), 0o644)
	fp3, _ := FingerprintTree(dir)
	if fp3 != fp1 {
		t.Errorf("excluded files changed the fingerprint: %q vs %q", fp3, fp1)
	}

	// A size change is detected.
	os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main(): return 1234567890\n"), 0o644)
	fp4, _ := FingerprintTree(dir)
	if fp4 == fp1 {
		t.Error("content size change did not change the fingerprint")
	}
}

func TestVerifyTree(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.star"), []byte("def main(): pass\n"), 0o644)
	h, _ := HashModuleTree(dir)

	if err := VerifyTree(dir, h); err != nil {
		t.Errorf("VerifyTree should pass for matching hash: %v", err)
	}
	if err := VerifyTree(dir, "sha256:deadbeef"); err == nil {
		t.Error("VerifyTree should fail for a mismatched hash")
	}
}

func TestLockRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// No lockfile → (nil, nil).
	l, err := LoadLock(dir)
	if err != nil || l != nil {
		t.Fatalf("LoadLock on empty dir = (%v, %v), want (nil, nil)", l, err)
	}

	want := &Lock{
		Modules: map[string]LockedModule{
			"acme/slack": {Source: "gitlab.com/acme/slack", Rev: "9f86d0", Hash: "sha256:1b4f"},
		},
	}
	if err := want.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if got.Version != lockVersion {
		t.Errorf("Version = %d, want %d", got.Version, lockVersion)
	}
	m, ok := got.Modules["acme/slack"]
	if !ok {
		t.Fatal("acme/slack missing from loaded lock")
	}
	if m.Source != "gitlab.com/acme/slack" || m.Rev != "9f86d0" || m.Hash != "sha256:1b4f" {
		t.Errorf("round-trip mismatch: %+v", m)
	}
}
