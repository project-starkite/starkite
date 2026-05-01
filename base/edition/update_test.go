package edition

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/project-starkite/starkite/base/version"
)

func TestLatestVersion(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"v0.2.0", "0.2.0"},
		{"0.2.0", "0.2.0"},
		{"v1.0.0-rc1", "1.0.0-rc1"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			r := &ReleaseInfo{TagName: tt.tag}
			if got := r.LatestVersion(); got != tt.want {
				t.Errorf("LatestVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsDevBuild(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"dev", true},
		{"0.1.0-dev", true},
		{"abc-dirty", true},
		{"", true},
		{"0.2.0", false},
		{"1.0.0", false},
		{"1.0.0-rc1", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.version), func(t *testing.T) {
			orig := version.Version
			version.Version = tt.version
			t.Cleanup(func() { version.Version = orig })

			if got := IsDevBuild(); got != tt.want {
				t.Errorf("IsDevBuild() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBinaryFileNameForBase(t *testing.T) {
	name := binaryFileName(EditionBase)
	expected := fmt.Sprintf("kite-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	if name != expected {
		t.Errorf("binaryFileName(%q) = %q, want %q", EditionBase, name, expected)
	}

	// Empty string should also produce base filename
	nameEmpty := binaryFileName("")
	if nameEmpty != expected {
		t.Errorf("binaryFileName(\"\") = %q, want %q", nameEmpty, expected)
	}
}

func TestBinaryFileNameForEdition(t *testing.T) {
	name := binaryFileName("cloud")
	expected := fmt.Sprintf("kite-cloud-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	if name != expected {
		t.Errorf("binaryFileName(%q) = %q, want %q", "cloud", name, expected)
	}
}

func TestDownloadAndReplace(t *testing.T) {
	// Create a fake binary payload
	fakeBinary := []byte("#!/bin/sh\necho updated")
	hash := sha256.Sum256(fakeBinary)
	hashHex := fmt.Sprintf("%x", hash)

	binaryName := binaryFileName(EditionBase)
	checksumBody := fmt.Sprintf("%s  %s\n", hashHex, binaryName)

	// Set up test server
	mux := http.NewServeMux()
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Write(fakeBinary)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksumBody))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Create a target file to replace
	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, "kite")
	if err := os.WriteFile(dstPath, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Run downloadAndReplace
	err := downloadAndReplace(
		server.URL+"/binary",
		server.URL+"/checksums.txt",
		dstPath,
		binaryName,
	)
	if err != nil {
		t.Fatalf("downloadAndReplace failed: %v", err)
	}

	// Verify the file was replaced
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("cannot read replaced file: %v", err)
	}
	if string(data) != string(fakeBinary) {
		t.Errorf("replaced file content = %q, want %q", string(data), string(fakeBinary))
	}

	// Verify executable permission
	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0111 == 0 {
		t.Error("replaced file is not executable")
	}
}

func TestDownloadAndReplaceChecksumMismatch(t *testing.T) {
	fakeBinary := []byte("binary content")
	binaryName := binaryFileName(EditionBase)
	checksumBody := fmt.Sprintf("%s  %s\n", "0000000000000000000000000000000000000000000000000000000000000000", binaryName)

	mux := http.NewServeMux()
	mux.HandleFunc("/binary", func(w http.ResponseWriter, r *http.Request) {
		w.Write(fakeBinary)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksumBody))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, "kite")
	if err := os.WriteFile(dstPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	// Should still succeed (checksum mismatch is a warning, not fatal)
	err := downloadAndReplace(
		server.URL+"/binary",
		server.URL+"/checksums.txt",
		dstPath,
		binaryName,
	)
	if err != nil {
		t.Fatalf("downloadAndReplace should succeed despite checksum mismatch warning: %v", err)
	}
}

func TestDownloadAndReplaceHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	dstPath := filepath.Join(tmpDir, "kite")
	if err := os.WriteFile(dstPath, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}

	err := downloadAndReplace(server.URL+"/binary", server.URL+"/checksums.txt", dstPath, "kite")
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

func TestDownloadURLForVersion(t *testing.T) {
	url := downloadURLForVersion("cloud", "0.3.0")
	expected := fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v0.3.0/kite-cloud-%s-%s",
		runtime.GOOS, runtime.GOARCH,
	)
	if url != expected {
		t.Errorf("downloadURLForVersion = %q, want %q", url, expected)
	}
}

func TestChecksumURLForVersion(t *testing.T) {
	url := checksumURLForVersion("0.3.0")
	expected := "https://github.com/project-starkite/starkite/releases/download/v0.3.0/checksums.txt"
	if url != expected {
		t.Errorf("checksumURLForVersion = %q, want %q", url, expected)
	}
}
