package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProvenance_JSONRoundTripAndCorruptHandling(t *testing.T) {
	dir := t.TempDir()
	want := &Provenance{
		Namespace:     "acme",
		Name:          "demo",
		Source:        "https://github.com/acme/demo",
		Version:       "v1.2.3",
		Rev:           "0123456789abcdef0123456789abcdef01234567",
		Hash:          "sha256:deadbeef",
		Fingerprint:   "fp1:1234",
		InstalledFrom: "github.com/acme/demo@v1.2.3",
	}

	if err := WriteProvenance(dir, want); err != nil {
		t.Fatalf("WriteProvenance: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, receiptFile))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(raw), `"schema_version": 1`) {
		t.Errorf("expected versioned JSON receipt, got:\n%s", raw)
	}

	got, err := ReadProvenance(dir)
	if err != nil {
		t.Fatalf("ReadProvenance: %v", err)
	}
	if got == nil || got.SchemaVersion != provenanceSchemaVersion || got.Rev != want.Rev || got.Hash != want.Hash {
		t.Fatalf("ReadProvenance mismatch: got=%+v want=%+v", got, want)
	}

	if err := os.WriteFile(filepath.Join(dir, receiptFile), []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadProvenance(dir); err == nil {
		t.Errorf("expected error for invalid JSON receipt")
	}
}
