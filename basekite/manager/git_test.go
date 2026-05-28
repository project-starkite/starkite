package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeRepoURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Already full URLs
		{"https://github.com/user/repo", "https://github.com/user/repo"},
		{"http://github.com/user/repo", "http://github.com/user/repo"},
		{"git@github.com:user/repo.git", "git@github.com:user/repo.git"},
		{"ssh://git@github.com/user/repo", "ssh://git@github.com/user/repo"},
		{"file:///path/to/repo", "file:///path/to/repo"},

		// Local paths
		{"/absolute/path/to/repo", "/absolute/path/to/repo"},
		{"./relative/path", "./relative/path"},
		{"../parent/path", "../parent/path"},

		// Known hosts
		{"github.com/user/repo", "https://github.com/user/repo"},
		{"gitlab.com/user/repo", "https://gitlab.com/user/repo"},
		{"bitbucket.org/user/repo", "https://bitbucket.org/user/repo"},

		// Short GitHub reference
		{"user/repo", "https://github.com/user/repo"},

		// Unknown host (defaults to HTTPS)
		{"example.com/user/repo", "https://example.com/user/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeRepoURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeRepoURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGitAvailable(t *testing.T) {
	// This test assumes git is installed on the test machine
	if !GitAvailable() {
		t.Skip("git not available")
	}

	// Verify git command works
	cmd := exec.Command("git", "--version")
	if err := cmd.Run(); err != nil {
		t.Errorf("git --version failed: %v", err)
	}
}

func TestGitClone(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}

	t.Run("clone local repo", func(t *testing.T) {
		// Create a source repo
		srcDir := t.TempDir()
		if err := exec.Command("git", "init", srcDir).Run(); err != nil {
			t.Fatalf("git init failed: %v", err)
		}

		// Create a file and commit
		testFile := filepath.Join(srcDir, "test.star")
		if err := os.WriteFile(testFile, []byte("# test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}

		cmd := exec.Command("git", "-C", srcDir, "add", "test.star")
		if err := cmd.Run(); err != nil {
			t.Fatalf("git add failed: %v", err)
		}

		cmd = exec.Command("git", "-C", srcDir, "commit", "-m", "initial")
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if err := cmd.Run(); err != nil {
			t.Fatalf("git commit failed: %v", err)
		}

		// Clone to destination
		destDir := filepath.Join(t.TempDir(), "cloned")
		if err := GitClone(srcDir, "", destDir); err != nil {
			t.Fatalf("GitClone failed: %v", err)
		}

		// Verify the file was cloned
		if _, err := os.Stat(filepath.Join(destDir, "test.star")); os.IsNotExist(err) {
			t.Error("cloned repo missing test.star")
		}
	})
}

func TestGitIsRepo(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}

	t.Run("is a repo", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
			t.Fatalf("git init failed: %v", err)
		}

		if !GitIsRepo(tmpDir) {
			t.Error("expected GitIsRepo to return true for git repo")
		}
	})

	t.Run("not a repo", func(t *testing.T) {
		tmpDir := t.TempDir()

		if GitIsRepo(tmpDir) {
			t.Error("expected GitIsRepo to return false for non-repo")
		}
	})
}

func TestGitGetCurrentCommit(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}

	// Create a repo with a commit
	tmpDir := t.TempDir()
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cmd := exec.Command("git", "-C", tmpDir, "add", "test.txt")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	cmd = exec.Command("git", "-C", tmpDir, "commit", "-m", "initial")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	commit, err := GitGetCurrentCommit(tmpDir)
	if err != nil {
		t.Fatalf("GitGetCurrentCommit failed: %v", err)
	}

	if len(commit) == 0 {
		t.Error("expected non-empty commit hash")
	}

	// Short hash should be 7-8 characters
	if len(commit) > 12 {
		t.Errorf("expected short commit hash, got %q", commit)
	}
}

func TestGitGetCurrentBranch(t *testing.T) {
	if !GitAvailable() {
		t.Skip("git not available")
	}

	// Create a repo with a commit
	tmpDir := t.TempDir()
	if err := exec.Command("git", "init", tmpDir).Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cmd := exec.Command("git", "-C", tmpDir, "add", "test.txt")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	cmd = exec.Command("git", "-C", tmpDir, "commit", "-m", "initial")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	branch, err := GitGetCurrentBranch(tmpDir)
	if err != nil {
		t.Fatalf("GitGetCurrentBranch failed: %v", err)
	}

	// Branch should be master or main (depending on git config)
	if branch != "master" && branch != "main" {
		t.Errorf("expected branch 'master' or 'main', got %q", branch)
	}
}
