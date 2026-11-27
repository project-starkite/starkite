package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/project-starkite/starkite/basekite/version"
	"github.com/spf13/cobra"
)

const (
	GitHubReleasesAPI = "https://api.github.com/repos/project-starkite/starkite/releases/latest"

	updateMetadataTimeout = 30 * time.Second
	updateDownloadTimeout = 5 * time.Minute
	maxMetadataBytes      = 1 << 20
	maxUpdateBinaryBytes  = 256 << 20
)

var (
	updateCheck bool
	updateForce bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update starkite to the latest version",
	Long: `Check for and install the latest version of starkite.

Downloads the latest release from GitHub and replaces the current running binary.

Examples:
  # Update to the latest version
  kite update

  # Check for updates without installing
  kite update --check

  # Force update even if already up-to-date or running a dev build
  kite update --force
`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Check for updates without installing")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if versions match or dev build")

	rootCmd.AddCommand(updateCmd)
}

type releaseInfo struct {
	TagName string `json:"tag_name"` // e.g. "v0.2.0"
}

func (r *releaseInfo) latestVersion() string {
	return strings.TrimPrefix(r.TagName, "v")
}

func newUpdateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func readBounded(r io.Reader, limit int64, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds maximum allowed size (%d bytes)", label, limit)
	}
	return data, nil
}

func fetchLatestRelease(ctx context.Context) (*releaseInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GitHubReleasesAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "starkite/"+version.Version)

	resp, err := newUpdateHTTPClient(updateMetadataTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	body, err := readBounded(resp.Body, maxMetadataBytes, "release metadata")
	if err != nil {
		return nil, fmt.Errorf("cannot read release info: %w", err)
	}

	var release releaseInfo
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("cannot parse release info: %w", err)
	}

	if release.TagName == "" {
		return nil, fmt.Errorf("no tag_name in release response")
	}

	return &release, nil
}

func isDevBuild() bool {
	v := version.Version
	return v == "" || v == "dev" || strings.HasSuffix(v, "-dev") || strings.HasSuffix(v, "-dirty")
}

func binaryFileName() string {
	var prefix string
	switch version.EditionName() {
	case "all":
		prefix = "kite"
	case "base":
		prefix = "kitecmd"
	case "cloud":
		prefix = "kitecloud"
	case "ai":
		prefix = "kiteai"
	default:
		prefix = "kite"
	}
	name := fmt.Sprintf("%s-%s-%s", prefix, runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func downloadURLForVersion(ver string) string {
	return fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v%s/%s",
		ver,
		binaryFileName(),
	)
}

func checksumURLForVersion(ver string) string {
	return fmt.Sprintf(
		"https://github.com/project-starkite/starkite/releases/download/v%s/checksums.txt",
		ver,
	)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Refuse dev builds unless --force
	if isDevBuild() && !updateForce {
		return fmt.Errorf("running a dev build (%s); use --force to update anyway", version.Version)
	}

	ctx := commandContext(cmd)

	// Fetch latest release from GitHub
	fmt.Println("Checking for updates...")
	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("cannot check for updates: %w", err)
	}

	latestVersion := release.latestVersion()
	fmt.Printf("Current version: %s\n", version.Version)
	fmt.Printf("Latest version:  %s\n", latestVersion)

	// Compare versions
	if version.Version == latestVersion && !updateForce {
		fmt.Println("Already up-to-date.")
		return nil
	}

	// --check: just print and stop
	if updateCheck {
		if version.Version != latestVersion {
			fmt.Printf("\nUpdate available: %s → %s\n", version.Version, latestVersion)
			fmt.Println("Run 'kite update' to install.")
		}
		return nil
	}

	// Update the running binary
	fmt.Printf("\nUpdating kite to %s...\n", latestVersion)
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	binaryName := binaryFileName()
	url := downloadURLForVersion(latestVersion)
	checksumURL := checksumURLForVersion(latestVersion)

	if err := downloadAndReplaceWithContext(ctx, url, checksumURL, execPath, binaryName, maxUpdateBinaryBytes); err != nil {
		return fmt.Errorf("self-update failed: %w", err)
	}

	fmt.Printf("Updated: %s\n", execPath)
	return nil
}

func downloadAndReplace(url, checksumURL, dstPath, binaryName string) error {
	return downloadAndReplaceWithContext(context.Background(), url, checksumURL, dstPath, binaryName, maxUpdateBinaryBytes)
}

func downloadAndReplaceWithContext(ctx context.Context, url, checksumURL, dstPath, binaryName string, maxBytes int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	req.Header.Set("User-Agent", "starkite/"+version.Version)

	resp, err := newUpdateHTTPClient(updateDownloadTimeout).Do(req)
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

	written, err := io.Copy(writer, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	if written > maxBytes {
		_ = tmpFile.Close()
		return fmt.Errorf("download exceeds maximum allowed size (%d bytes)", maxBytes)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("cannot finalize downloaded binary: %w", err)
	}

	// Verify checksum (fail closed)
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if err := verifyChecksumFromURLWithContext(ctx, checksumURL, actualHash, binaryName); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		return fmt.Errorf("cannot set permissions: %w", err)
	}

	if err := replaceExecutable(tmpPath, dstPath); err != nil {
		return fmt.Errorf("cannot replace binary: %w", err)
	}

	return nil
}

func replaceExecutable(tmpPath, dstPath string) error {
	if runtime.GOOS == "windows" {
		return replaceExecutableWithBackup(tmpPath, dstPath)
	}
	return os.Rename(tmpPath, dstPath)
}

func replaceExecutableWithBackup(tmpPath, dstPath string) error {
	oldPath := dstPath + ".old"
	_ = os.Remove(oldPath)
	if err := os.Rename(dstPath, oldPath); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Rename(oldPath, dstPath)
		return err
	}
	_ = os.Remove(oldPath)
	return nil
}

func verifyChecksumFromURL(checksumURL, actualHash, binaryName string) error {
	return verifyChecksumFromURLWithContext(context.Background(), checksumURL, actualHash, binaryName)
}

func verifyChecksumFromURLWithContext(ctx context.Context, checksumURL, actualHash, binaryName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return fmt.Errorf("cannot fetch checksum: %w", err)
	}
	req.Header.Set("User-Agent", "starkite/"+version.Version)

	resp, err := newUpdateHTTPClient(updateMetadataTimeout).Do(req)
	if err != nil {
		return fmt.Errorf("cannot fetch checksum: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum file not available (HTTP %d)", resp.StatusCode)
	}

	body, err := readBounded(resp.Body, maxMetadataBytes, "checksums file")
	if err != nil {
		return fmt.Errorf("cannot read checksum: %w", err)
	}

	expectedHash := ""
	for line := range strings.SplitSeq(string(body), "\n") {
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
