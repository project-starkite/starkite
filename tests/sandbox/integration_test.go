//go:build linux

// Package sandbox_test drives the .star sandbox integration tests by
// invoking `kite test --sandbox` against tests/sandbox/*_test.star from
// a clean temp directory (so credential-isolation tests aren't fooled
// by $HOME being the $CWD bind).
//
// Skipped on non-Linux (build tag).
// Skipped unless STARKITE_SANDBOX_INTEGRATION=1 — these tests build
// allkite, spawn kite under gVisor, and depend on the kernel allowing
// unprivileged user namespaces. CI opts in explicitly; local `go test
// ./...` doesn't trigger them.
package sandbox_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const perTestTimeout = 60 * time.Second

func TestSandboxDefaultProfile(t *testing.T) {
	if os.Getenv("STARKITE_SANDBOX_INTEGRATION") != "1" {
		t.Skip("set STARKITE_SANDBOX_INTEGRATION=1 to run sandbox integration tests")
	}
	if reason := unprivilegedUsernsBlocked(); reason != "" {
		t.Skipf("kernel restricts unprivileged user namespaces: %s\n"+
			"To run these tests: install kite's AppArmor profile, or\n"+
			"  sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0",
			reason)
	}

	kite := buildKite(t)

	// Run from a temp dir explicitly OUTSIDE $HOME so the $CWD bind
	// doesn't accidentally expose ~/.aws, ~/.ssh, etc. to the script.
	workDir := t.TempDir()
	home, err := os.UserHomeDir()
	if err == nil && strings.HasPrefix(workDir, home) {
		t.Fatalf("temp dir %s is under $HOME — sandbox isolation tests would be confounded", workDir)
	}

	// The default sandbox profile only mounts $CWD (and a few public
	// system files). The test script must live inside $CWD so kite
	// inside the sandbox can find it.
	srcPath := mustAbs(t, "sandbox_default_test.star")
	dstPath := filepath.Join(workDir, "sandbox_default_test.star")
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("staging test script into workdir: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, kite, "test", dstPath, "--sandbox")
	cmd.Dir = workDir

	out, err := cmd.CombinedOutput()
	t.Logf("kite test --sandbox output:\n%s", out)
	if err != nil {
		t.Fatalf("kite test --sandbox failed: %v", err)
	}

	// kite test prints "Tests: N passed, M failed, K total".
	// Pass condition: exit 0 AND non-zero "passed" count AND zero "failed" count.
	output := string(out)
	if !strings.Contains(output, "passed") {
		t.Errorf("expected 'passed' summary in output")
	}
	if strings.Contains(output, "failed") && !strings.Contains(output, "0 failed") {
		t.Errorf("kite test reports failures; see output above")
	}
}

// buildKite builds the allkite binary (the kite all-in-one) into a
// t.TempDir() and returns its path.
func buildKite(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolving repo root: %v", err)
	}
	out := filepath.Join(t.TempDir(), "kite")

	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = filepath.Join(repoRoot, "allkite")
	if buildOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building kite: %v\n%s", err, buildOut)
	}
	return out
}

// unprivilegedUsernsBlocked returns a non-empty reason string when the
// kernel state prevents unprivileged user namespaces (the precondition
// gVisor's rootless mode needs). Empty string means preflight is happy.
func unprivilegedUsernsBlocked() string {
	if v, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns"); err == nil {
		if strings.TrimSpace(string(v)) == "1" {
			return "apparmor_restrict_unprivileged_userns=1"
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		// Unreadable but present — odd; let gVisor surface the real error.
	}
	if v, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone"); err == nil {
		if strings.TrimSpace(string(v)) == "0" {
			return "unprivileged_userns_clone=0"
		}
	}
	return ""
}

func mustAbs(t *testing.T, rel string) string {
	t.Helper()
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", rel, err)
	}
	return abs
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
