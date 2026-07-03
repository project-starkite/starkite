package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/basekite/version"
)

func TestBinaryFileName(t *testing.T) {
	origEdition := version.Edition
	defer func() { version.Edition = origEdition }()

	tests := []struct {
		edition string
		want    string
	}{
		{"all", "kite"},
		{"base", "kitecmd"},
		{"cloud", "kitecloud"},
		{"ai", "kiteai"},
		{"", "kitecmd"},
	}

	for _, tc := range tests {
		version.Edition = tc.edition
		got := binaryFileName()
		suffix := ""
		if runtime.GOOS == "windows" {
			suffix = ".exe"
		}
		expected := fmt.Sprintf("%s-%s-%s%s", tc.want, runtime.GOOS, runtime.GOARCH, suffix)
		if got != expected {
			t.Errorf("binaryFileName() for edition %q = %q, want %q", tc.edition, got, expected)
		}
	}
}

func TestIsDevBuild(t *testing.T) {
	origVer := version.Version
	defer func() { version.Version = origVer }()

	tests := []struct {
		ver  string
		want bool
	}{
		{"dev", true},
		{"0.1.0-dev", true},
		{"0.1.0-dirty", true},
		{"", true},
		{"0.1.0", false},
		{"1.0.0", false},
	}

	for _, tc := range tests {
		version.Version = tc.ver
		if got := isDevBuild(); got != tc.want {
			t.Errorf("isDevBuild() for version %q = %v, want %v", tc.ver, got, tc.want)
		}
	}
}

func TestVerifyChecksumFromURL(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hash123  some-other-file\nexpected_hash  test-binary\n"))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	err := verifyChecksumFromURL(server.URL, "expected_hash", "test-binary")
	if err != nil {
		t.Fatalf("verifyChecksumFromURL failed: %v", err)
	}

	err = verifyChecksumFromURL(server.URL, "wrong_hash", "test-binary")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got %v", err)
	}

	err = verifyChecksumFromURL(server.URL, "expected_hash", "nonexistent-binary")
	if err == nil || !strings.Contains(err.Error(), "no checksum found") {
		t.Errorf("expected no checksum found error, got %v", err)
	}
}

func TestDownloadAndReplace(t *testing.T) {
	binaryData := []byte("new-binary-content")
	hasher := sha256.New()
	hasher.Write(binaryData)
	binaryHash := hex.EncodeToString(hasher.Sum(nil))

	binaryName := "test-bin"

	mux := http.NewServeMux()
	mux.HandleFunc("/bin", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(binaryData)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("%s  %s\n", binaryHash, binaryName)))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "starkite-test-update")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dstPath := filepath.Join(tmpDir, "kite")
	if err := os.WriteFile(dstPath, []byte("old-binary-content"), 0755); err != nil {
		t.Fatalf("failed to write dummy destination: %v", err)
	}

	err = downloadAndReplace(
		server.URL+"/bin",
		server.URL+"/checksums.txt",
		dstPath,
		binaryName,
	)
	if err != nil {
		t.Fatalf("downloadAndReplace failed: %v", err)
	}

	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read replaced file: %v", err)
	}

	if string(content) != string(binaryData) {
		t.Errorf("dstPath content = %q, want %q", string(content), string(binaryData))
	}
}
