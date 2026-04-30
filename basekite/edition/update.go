package edition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/project-starkite/starkite/basekite/version"
)

const GitHubReleasesAPI = "https://api.github.com/repos/vladimirvivien/starkite/releases/latest"

// ReleaseInfo holds GitHub release metadata.
type ReleaseInfo struct {
	TagName string `json:"tag_name"` // e.g. "v0.2.0"
}

// LatestVersion returns the version string without a "v" prefix.
func (r *ReleaseInfo) LatestVersion() string {
	return strings.TrimPrefix(r.TagName, "v")
}

// FetchLatestRelease queries the GitHub Releases API for the latest release.
func FetchLatestRelease() (*ReleaseInfo, error) {
	req, err := http.NewRequest("GET", GitHubReleasesAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "starkite/"+version.Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var release ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("cannot parse release info: %w", err)
	}

	if release.TagName == "" {
		return nil, fmt.Errorf("no tag_name in release response")
	}

	return &release, nil
}

// IsDevBuild returns true if the current version looks like a development build.
func IsDevBuild() bool {
	v := version.Version
	return v == "" || v == "dev" || strings.HasSuffix(v, "-dev") || strings.HasSuffix(v, "-dirty")
}

// UpdateSelf replaces the currently running starkite binary with the given version.
// Returns the path of the replaced binary.
func UpdateSelf(newVersion string) (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	binaryName := binaryFileName(EditionBase)
	url := downloadURLForVersion(EditionBase, newVersion)
	checksumURL := checksumURLForVersion(newVersion)

	if err := downloadAndReplace(url, checksumURL, execPath, binaryName); err != nil {
		return "", fmt.Errorf("self-update failed: %w", err)
	}

	return execPath, nil
}

// UpdateEdition replaces an installed edition binary with the given version.
func UpdateEdition(name, newVersion string) error {
	binaryPath, err := EditionBinaryPath(name)
	if err != nil {
		return fmt.Errorf("cannot determine edition path: %w", err)
	}

	// Only update if the binary actually exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("edition %q not installed", name)
	}

	binaryName := binaryFileName(name)
	url := downloadURLForVersion(name, newVersion)
	checksumURL := checksumURLForVersion(newVersion)

	if err := downloadAndReplace(url, checksumURL, binaryPath, binaryName); err != nil {
		return fmt.Errorf("edition update failed: %w", err)
	}

	return nil
}

// downloadAndReplace downloads a binary from url, verifies its checksum, and
// atomically replaces dstPath. binaryName is the filename used to look up the
// expected hash in the checksums file.
func downloadAndReplace(url, checksumURL, dstPath, binaryName string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}

	// Write to temp file in the same directory (same filesystem for atomic rename)
	tmpFile, err := os.CreateTemp(filepath.Dir(dstPath), "starkite-update-*")
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

	// Verify checksum
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if err := verifyChecksumFromURL(checksumURL, actualHash, binaryName); err != nil {
		fmt.Fprintf(os.Stderr, "warning: checksum verification skipped: %v\n", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, dstPath); err != nil {
		return fmt.Errorf("cannot replace binary: %w", err)
	}

	return nil
}

// verifyChecksumFromURL downloads a checksums file and verifies the hash.
func verifyChecksumFromURL(checksumURL, actualHash, binaryName string) error {
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

	expectedHash := ""
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

// downloadURLForVersion returns the download URL for a specific version.
func downloadURLForVersion(editionName, ver string) string {
	return fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v%s/%s",
		ver,
		binaryFileName(editionName),
	)
}

// checksumURLForVersion returns the checksums URL for a specific version.
func checksumURLForVersion(ver string) string {
	return fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v%s/checksums.txt",
		ver,
	)
}
