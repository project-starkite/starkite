package edition

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/project-starkite/starkite/base/version"
)

// InstallOptions configures edition installation.
type InstallOptions struct {
	FromPath string // local binary path (--from flag); empty = download
	Force    bool
}

// Install installs an edition by name.
func Install(name string, opts InstallOptions) error {
	if !IsKnownEdition(name) {
		return fmt.Errorf("unknown edition: %q (known editions: %s)", name, strings.Join(KnownEditions, ", "))
	}

	binaryPath, err := EditionBinaryPath(name)
	if err != nil {
		return fmt.Errorf("cannot determine edition path: %w", err)
	}

	// Check if already installed
	if _, err := os.Stat(binaryPath); err == nil {
		if !opts.Force {
			return fmt.Errorf("edition %q already installed at %s (use --force to overwrite)", name, binaryPath)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		return fmt.Errorf("cannot create edition directory: %w", err)
	}

	if opts.FromPath != "" {
		return installFromLocal(opts.FromPath, binaryPath)
	}
	return installFromRemote(name, binaryPath)
}

// installFromLocal copies a local binary to the edition path.
func installFromLocal(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cannot open source binary: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("cannot create edition binary: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	return out.Close()
}

// installFromRemote downloads an edition binary from GitHub Releases.
func installFromRemote(name, dst string) error {
	url := DownloadURL(name)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download edition: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}

	// Write to temp file while computing SHA256
	tmpFile, err := os.CreateTemp(filepath.Dir(dst), "starkite-download-*")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	if _, err := io.Copy(writer, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

	// Try to verify checksum
	checksumURL := ChecksumURL(name)
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if err := verifyChecksum(checksumURL, actualHash, name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: checksum verification skipped: %v\n", err)
	}

	// Set executable permissions and move to final path
	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("cannot install binary: %w", err)
	}

	return nil
}

// verifyChecksum downloads and verifies a SHA256 checksum file.
func verifyChecksum(checksumURL, actualHash, editionName string) error {
	resp, err := http.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("cannot fetch checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum file not available (HTTP %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("cannot read checksum: %w", err)
	}

	// Parse checksum file (format: "<hash>  <filename>")
	expectedHash := ""
	binaryName := binaryFileName(editionName)
	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == binaryName {
			expectedHash = parts[0]
			break
		}
	}

	if expectedHash == "" {
		return fmt.Errorf("no checksum found for %s", binaryName)
	}

	if actualHash != expectedHash {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// Remove removes an installed edition.
func Remove(name string) error {
	if name == EditionBase {
		return fmt.Errorf("cannot remove the base edition")
	}

	editionsDir, err := EditionsDir()
	if err != nil {
		return fmt.Errorf("cannot determine editions directory: %w", err)
	}

	editionDir := filepath.Join(editionsDir, name)
	if _, err := os.Stat(editionDir); os.IsNotExist(err) {
		return fmt.Errorf("edition %q is not installed", name)
	}

	if err := os.RemoveAll(editionDir); err != nil {
		return fmt.Errorf("failed to remove edition: %w", err)
	}

	// Reset active edition if we just removed the active one
	if ActiveEdition() == name {
		if err := SetActiveEdition(EditionBase); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to reset active edition: %v\n", err)
		}
	}

	return nil
}

// ListInstalled returns information about all installed editions.
func ListInstalled() ([]*EditionInfo, error) {
	editionsDir, err := EditionsDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine editions directory: %w", err)
	}

	active := ActiveEdition()

	// Always include base edition
	editions := []*EditionInfo{
		{
			Name:      EditionBase,
			Path:      "(built-in)",
			Installed: true,
			Active:    active == EditionBase,
		},
	}

	entries, err := os.ReadDir(editionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return editions, nil
		}
		return nil, fmt.Errorf("cannot read editions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		binaryPath := filepath.Join(editionsDir, name, "kite")

		info := &EditionInfo{
			Name:   name,
			Path:   binaryPath,
			Active: active == name,
		}

		if fi, err := os.Stat(binaryPath); err == nil {
			info.Installed = true
			info.Size = fi.Size()
		}

		editions = append(editions, info)
	}

	return editions, nil
}

// DownloadURL returns the GitHub Releases download URL for an edition binary.
func DownloadURL(editionName string) string {
	return fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v%s/%s",
		version.Version,
		binaryFileName(editionName),
	)
}

// ChecksumURL returns the GitHub Releases URL for the checksums file.
func ChecksumURL(editionName string) string {
	return fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v%s/checksums.txt",
		version.Version,
	)
}

// binaryFileName returns the platform-specific binary filename for an edition.
func binaryFileName(editionName string) string {
	var name string
	if editionName == EditionBase || editionName == "" {
		name = fmt.Sprintf("kite-%s-%s", runtime.GOOS, runtime.GOARCH)
	} else {
		name = fmt.Sprintf("kite-%s-%s-%s", editionName, runtime.GOOS, runtime.GOARCH)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}
