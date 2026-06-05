package manager

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GitClone clones a repository to the destination path.
// If version is specified, checks out that version (tag, branch, or commit).
func GitClone(repo, version, destPath string) error {
	// Convert short form to full URL if needed
	repoURL := normalizeRepoURL(repo)

	// Clone the repository
	args := []string{"clone", "--depth", "1"}
	if version != "" {
		args = append(args, "--branch", version)
	}
	args = append(args, repoURL, destPath)

	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If shallow clone with branch failed, try full clone
		if version != "" && strings.Contains(stderr.String(), "not found") {
			return gitCloneAndCheckout(repoURL, version, destPath)
		}
		return fmt.Errorf("git clone failed: %s", stderr.String())
	}

	return nil
}

// gitCloneAndCheckout does a full clone and checks out a specific version.
// This handles cases where the version is a commit hash.
func gitCloneAndCheckout(repoURL, version, destPath string) error {
	// Full clone
	cloneCmd := exec.Command("git", "clone", repoURL, destPath)
	var stderr bytes.Buffer
	cloneCmd.Stderr = &stderr

	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %s", stderr.String())
	}

	// Checkout the version
	checkoutCmd := exec.Command("git", "-C", destPath, "checkout", version)
	checkoutCmd.Stderr = &stderr

	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("git checkout %s failed: %s", version, stderr.String())
	}

	return nil
}

// GitPull pulls the latest changes in a repository.
// Returns the new commit hash.
func GitPull(repoPath string) (string, error) {
	// Pull latest changes
	pullCmd := exec.Command("git", "-C", repoPath, "pull", "--ff-only")
	var stderr bytes.Buffer
	pullCmd.Stderr = &stderr

	if err := pullCmd.Run(); err != nil {
		return "", fmt.Errorf("git pull failed: %s", stderr.String())
	}

	// Get current commit hash
	return GitGetCurrentCommit(repoPath)
}

// GitGetCurrentCommit returns the current commit hash.
func GitGetCurrentCommit(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--short", "HEAD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse failed: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GitGetCurrentBranch returns the current branch name.
func GitGetCurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git rev-parse failed: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GitIsRepo checks if a path is a git repository.
func GitIsRepo(path string) bool {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

// GitGetRemoteURL returns the origin remote URL.
func GitGetRemoteURL(repoPath string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git remote get-url failed: %s", stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// normalizeRepoURL converts a repository reference to a clonable URL. It does
// not assume any particular git host: a reference must already carry an explicit
// scheme, a git@ form, a local path, or a "host/org/repo" shape. A bare
// "org/repo" with no host is ambiguous and is left unchanged so the git clone
// fails with a clear error rather than silently defaulting to a host.
func normalizeRepoURL(repo string) string {
	// Already a full URL or scp-like git@ form.
	if strings.HasPrefix(repo, "https://") ||
		strings.HasPrefix(repo, "http://") ||
		strings.HasPrefix(repo, "git@") ||
		strings.HasPrefix(repo, "ssh://") ||
		strings.HasPrefix(repo, "file://") {
		return repo
	}

	// Local path (absolute or relative) — git can clone it directly.
	if strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, "./") || strings.HasPrefix(repo, "../") {
		return repo
	}

	// "host/org/repo" — a dotted first segment looks like a hostname, so add
	// the https scheme. Works for any host, not just well-known ones.
	if firstSeg, _, ok := strings.Cut(repo, "/"); ok && strings.Contains(firstSeg, ".") {
		return "https://" + repo
	}

	// Anything else (e.g. bare "org/repo") is ambiguous; return as-is.
	return repo
}

// GitAvailable checks if git is available in PATH.
func GitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}
