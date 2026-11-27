package manager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const defaultGitTimeout = 2 * time.Minute

func gitCommand(ctx context.Context, args ...string) (*exec.Cmd, context.CancelFunc) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultGitTimeout)
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		return cmd, cancel
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd, func() {}
}

// GitClone clones a repository to the destination path.
// If version is specified, checks out that version (tag, branch, or commit).
func GitClone(ctx context.Context, repo, version, destPath string) error {
	if strings.HasPrefix(repo, "-") {
		return fmt.Errorf("invalid repository reference %q: must not start with '-'", repo)
	}
	if strings.HasPrefix(version, "-") {
		return fmt.Errorf("invalid version %q: must not start with '-'", version)
	}

	// Convert short form to full URL if needed
	repoURL := normalizeRepoURL(repo)
	if strings.HasPrefix(repoURL, "-") {
		return fmt.Errorf("invalid repository URL %q: must not start with '-'", repoURL)
	}

	// Clone the repository
	args := []string{"-c", "core.symlinks=false", "clone", "--depth", "1"}
	if version != "" {
		args = append(args, "--branch", version)
	}
	args = append(args, "--", repoURL, destPath)

	cmd, cancel := gitCommand(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	cancel()

	if err != nil {
		// Shallow clone with --branch rejects commit SHAs; fall back to a full clone and checkout.
		if version != "" && (ctx == nil || ctx.Err() == nil) {
			_ = os.RemoveAll(destPath)
			return gitCloneAndCheckout(ctx, repoURL, version, destPath)
		}
		return fmt.Errorf("git clone failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// gitCloneAndCheckout does a full clone and checks out a specific version.
// This handles cases where the version is a commit hash.
func gitCloneAndCheckout(ctx context.Context, repoURL, version, destPath string) error {
	if strings.HasPrefix(repoURL, "-") {
		return fmt.Errorf("invalid repository URL %q: must not start with '-'", repoURL)
	}
	if strings.HasPrefix(version, "-") {
		return fmt.Errorf("invalid version %q: must not start with '-'", version)
	}

	// Full clone
	cloneCmd, cancelClone := gitCommand(ctx, "-c", "core.symlinks=false", "clone", "--", repoURL, destPath)
	var stderr bytes.Buffer
	cloneCmd.Stderr = &stderr
	err := cloneCmd.Run()
	cancelClone()
	if err != nil {
		return fmt.Errorf("git clone failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	// Checkout the version
	stderr.Reset()
	checkoutCmd, cancelCheckout := gitCommand(ctx, "-C", destPath, "-c", "core.symlinks=false", "checkout", version, "--")
	checkoutCmd.Stderr = &stderr
	err = checkoutCmd.Run()
	cancelCheckout()
	if err != nil {
		return fmt.Errorf("git checkout %s failed: %w: %s", version, err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// GitGetCurrentCommit returns the full commit SHA.
func GitGetCurrentCommit(ctx context.Context, repoPath string) (string, error) {
	cmd, cancel := gitCommand(ctx, "-C", repoPath, "rev-parse", "HEAD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	cancel()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GitGetCurrentBranch returns the current branch name.
func GitGetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	cmd, cancel := gitCommand(ctx, "-C", repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	cancel()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GitIsRepo checks if a path is a git repository.
func GitIsRepo(ctx context.Context, path string) bool {
	cmd, cancel := gitCommand(ctx, "-C", path, "rev-parse", "--git-dir")
	err := cmd.Run()
	cancel()
	return err == nil
}

// GitGetRemoteURL returns the origin remote URL.
func GitGetRemoteURL(ctx context.Context, repoPath string) (string, error) {
	cmd, cancel := gitCommand(ctx, "-C", repoPath, "remote", "get-url", "origin")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	cancel()
	if err != nil {
		return "", fmt.Errorf("git remote get-url failed: %w: %s", err, strings.TrimSpace(stderr.String()))
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
