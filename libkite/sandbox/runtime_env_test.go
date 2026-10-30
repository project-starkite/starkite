package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSensitiveHomeDir(t *testing.T) {
	home := filepath.Clean("/Users/developer")

	sensitiveCases := []string{
		home,
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".kube", "config"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".gnupg", "pubring.kbx"),
		filepath.Join(home, ".config"),
		filepath.Join(home, ".config", "gcloud"),
		filepath.Join(home, ".config", "gcloud", "credentials.db"),
		filepath.Join(home, ".docker"),
		filepath.Join(home, ".docker", "config.json"),
		filepath.Join(home, ".gitconfig"),
	}

	for _, p := range sensitiveCases {
		if !isSensitiveHomeDir(p, home) {
			t.Errorf("isSensitiveHomeDir(%q, %q) = false, want true", p, home)
		}
	}

	nonSensitiveCases := []string{
		filepath.Join(home, ".venv"),
		filepath.Join(home, "venv"),
		filepath.Join(home, "projects", "repo"),
		filepath.Join(home, "node_modules"),
		filepath.Join(home, ".config", "my-app"),
		filepath.Join(home, ".gem"),
		filepath.Join(home, ".nvm"),
		"/tmp/scratch",
		"/usr/lib/python3.11",
		"",
	}

	for _, p := range nonSensitiveCases {
		if isSensitiveHomeDir(p, home) {
			t.Errorf("isSensitiveHomeDir(%q, %q) = true, want false", p, home)
		}
	}

	// Test relative sensitive paths
	relSensitiveCases := []string{
		".ssh",
		".ssh/id_rsa",
		".aws",
		".aws/credentials",
		".kube/config",
		".gnupg",
		".config/gcloud",
		".gitconfig",
	}
	for _, rel := range relSensitiveCases {
		if !isSensitiveHomeDir(rel, home) {
			t.Errorf("isSensitiveHomeDir(relative %q, %q) = false, want true", rel, home)
		}
	}

	// Test symlinks pointing to sensitive home directories
	tmpDir := t.TempDir()
	realSSH := filepath.Join(tmpDir, "home", ".ssh")
	if err := os.MkdirAll(realSSH, 0700); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(tmpDir, "external_link_to_ssh")
	if err := os.Symlink(realSSH, symlinkPath); err == nil {
		if !isSensitiveHomeDir(symlinkPath, filepath.Join(tmpDir, "home")) {
			t.Errorf("isSensitiveHomeDir symlink %q pointing to %q = false, want true", symlinkPath, realSSH)
		}
	}

	// Test case-insensitive sensitive path traversal (Finding HIGH-02)
	caseInsensitiveHome := "/Users/Developer"
	caseInsensitiveSensitiveCases := []string{
		"/users/developer/.ssh",
		"/users/developer/.ssh/id_ed25519",
		"/USERS/DEVELOPER/.SSH",
		"/Users/Developer/.aws",
		"/users/developer/.kube",
		"/users/developer/.gnupg",
		"/users/developer/.config/gcloud",
		"/users/developer/.gitconfig",
	}
	for _, p := range caseInsensitiveSensitiveCases {
		if !isSensitiveHomeDir(p, caseInsensitiveHome) {
			t.Errorf("isSensitiveHomeDir(%q, %q) = false, want true", p, caseInsensitiveHome)
		}
	}
	if isSensitiveHomeDir("/users/developer/projects/repo", caseInsensitiveHome) {
		t.Errorf("isSensitiveHomeDir(/users/developer/projects/repo, %q) = true, want false", caseInsensitiveHome)
	}

	if isSensitiveHomeDir("", "") {
		t.Error("isSensitiveHomeDir(\"\", \"\") = true, want false")
	}
}

func TestResolveRuntimeEnvMounts_Python(t *testing.T) {
	tmpHome := t.TempDir()
	tmpCwd := t.TempDir()

	venvDir := filepath.Join(tmpCwd, ".venv")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatal(err)
	}

	mounts, err := ResolveRuntimeEnvMounts(RuntimePython, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts(RuntimePython): %v", err)
	}

	foundVenv := false
	for _, m := range mounts {
		if m.Destination == venvDir {
			foundVenv = true
			if m.Type != MountBind {
				t.Errorf("Mount.Type = %v, want %v", m.Type, MountBind)
			}
			if m.Mode != MountRO {
				t.Errorf("Mount.Mode = %v, want %v", m.Mode, MountRO)
			}
		}
	}
	if !foundVenv {
		t.Errorf("expected %q to be mounted, got %v", venvDir, mounts)
	}

	// Test VIRTUAL_ENV environment discovery
	customVenv := t.TempDir()
	t.Setenv("VIRTUAL_ENV", customVenv)

	mountsWithEnv, err := ResolveRuntimeEnvMounts(RuntimePython, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts with VIRTUAL_ENV: %v", err)
	}
	foundCustom := false
	for _, m := range mountsWithEnv {
		if m.Destination == customVenv {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Errorf("expected custom VIRTUAL_ENV %q in mounts, got %v", customVenv, mountsWithEnv)
	}

	// Test sensitive VIRTUAL_ENV suppression
	sensitiveVenv := filepath.Join(tmpHome, ".ssh")
	if err := os.MkdirAll(sensitiveVenv, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VIRTUAL_ENV", sensitiveVenv)

	mountsSensitive, err := ResolveRuntimeEnvMounts(RuntimePython, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts: %v", err)
	}
	for _, m := range mountsSensitive {
		if m.Destination == sensitiveVenv {
			t.Errorf("sensitive path %q must not be mounted", sensitiveVenv)
		}
	}

	// Test symlink candidate pointing to sensitive directory under home (Finding HIGH-01)
	symlinkToSensitive := filepath.Join(tmpCwd, "external_link_ssh")
	if err := os.Symlink(sensitiveVenv, symlinkToSensitive); err == nil {
		t.Setenv("VIRTUAL_ENV", symlinkToSensitive)
		mountsSymlink, err := ResolveRuntimeEnvMounts(RuntimePython, tmpCwd, tmpHome)
		if err != nil {
			t.Fatalf("ResolveRuntimeEnvMounts with symlink: %v", err)
		}
		for _, m := range mountsSymlink {
			if m.Destination == symlinkToSensitive {
				t.Errorf("symlink %q pointing to sensitive path %q must not be mounted", symlinkToSensitive, sensitiveVenv)
			}
		}
	}

	// Test duplicate mount elimination via symlink (Finding HIGH-01)
	altVenv := filepath.Join(tmpCwd, "alt_venv")
	if err := os.Symlink(venvDir, altVenv); err == nil {
		t.Setenv("VIRTUAL_ENV", altVenv)
		mountsDup, err := ResolveRuntimeEnvMounts(RuntimePython, tmpCwd, tmpHome)
		if err != nil {
			t.Fatalf("ResolveRuntimeEnvMounts with symlink alias: %v", err)
		}
		seenCount := 0
		for _, m := range mountsDup {
			if m.Destination == venvDir || m.Destination == altVenv {
				seenCount++
			}
		}
		if seenCount > 1 {
			t.Errorf("expected at most 1 mount for target and symlink, got %d", seenCount)
		}
	}

	// Test PYTHONPATH environment discovery
	t.Setenv("VIRTUAL_ENV", "")
	pyPathDir := t.TempDir()
	t.Setenv("PYTHONPATH", pyPathDir)

	mountsPyPath, err := ResolveRuntimeEnvMounts(RuntimePython, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts with PYTHONPATH: %v", err)
	}
	foundPyPath := false
	for _, m := range mountsPyPath {
		if m.Destination == pyPathDir {
			foundPyPath = true
		}
	}
	if !foundPyPath {
		t.Errorf("expected PYTHONPATH %q in mounts, got %v", pyPathDir, mountsPyPath)
	}
}

func TestResolveRuntimeEnvMounts_Node(t *testing.T) {
	tmpHome := t.TempDir()
	tmpCwd := t.TempDir()

	nodeModulesDir := filepath.Join(tmpCwd, "node_modules")
	if err := os.MkdirAll(nodeModulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	mounts, err := ResolveRuntimeEnvMounts(RuntimeNode, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts(RuntimeNode): %v", err)
	}

	foundNodeModules := false
	for _, m := range mounts {
		if m.Destination == nodeModulesDir {
			foundNodeModules = true
			if m.Mode != MountRO {
				t.Errorf("Mount.Mode = %v, want %v", m.Mode, MountRO)
			}
		}
	}
	if !foundNodeModules {
		t.Errorf("expected %q in mounts, got %v", nodeModulesDir, mounts)
	}

	// Test NVM_BIN environment discovery
	nvmRoot := t.TempDir()
	nvmBin := filepath.Join(nvmRoot, "bin")
	if err := os.MkdirAll(nvmBin, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NVM_BIN", nvmBin)

	mountsNvm, err := ResolveRuntimeEnvMounts(RuntimeNode, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts with NVM_BIN: %v", err)
	}
	foundNvm := false
	for _, m := range mountsNvm {
		if m.Destination == nvmBin {
			foundNvm = true
		}
	}
	if !foundNvm {
		t.Errorf("expected %q in mounts, got %v", nvmBin, mountsNvm)
	}

	// Test NODE_PATH environment discovery
	nodePathDir := t.TempDir()
	t.Setenv("NODE_PATH", nodePathDir)

	mountsNodePath, err := ResolveRuntimeEnvMounts(RuntimeNode, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts with NODE_PATH: %v", err)
	}
	foundNodePath := false
	for _, m := range mountsNodePath {
		if m.Destination == nodePathDir {
			foundNodePath = true
		}
	}
	if !foundNodePath {
		t.Errorf("expected NODE_PATH %q in mounts, got %v", nodePathDir, mountsNodePath)
	}
}

func TestResolveRuntimeEnvMounts_Ruby(t *testing.T) {
	tmpHome := t.TempDir()
	tmpCwd := t.TempDir()

	gemHome := t.TempDir()
	t.Setenv("GEM_HOME", gemHome)

	mounts, err := ResolveRuntimeEnvMounts(RuntimeRuby, tmpCwd, tmpHome)
	if err != nil {
		t.Fatalf("ResolveRuntimeEnvMounts(RuntimeRuby): %v", err)
	}

	foundGemHome := false
	for _, m := range mounts {
		if m.Destination == gemHome {
			foundGemHome = true
			if m.Mode != MountRO {
				t.Errorf("Mount.Mode = %v, want %v", m.Mode, MountRO)
			}
		}
	}
	if !foundGemHome {
		t.Errorf("expected %q in mounts, got %v", gemHome, mounts)
	}
}

func TestResolveRuntimeEnvMounts_EdgeCases(t *testing.T) {
	m, err := ResolveRuntimeEnvMounts("", "/cwd", "/home")
	if err != nil || m != nil {
		t.Errorf("ResolveRuntimeEnvMounts(\"\"): got (%v, %v), want (nil, nil)", m, err)
	}

	_, err = ResolveRuntimeEnvMounts("unsupported-lang", "/cwd", "/home")
	if err == nil {
		t.Error("expected error for unsupported runtime, got nil")
	}
}

func TestBuiltinProfiles_RuntimeProfiles(t *testing.T) {
	tests := []struct {
		name        string
		wantRuntime RuntimeType
	}{
		{ProfilePythonEnv, RuntimePython},
		{ProfileNodeEnv, RuntimeNode},
		{ProfileRubyEnv, RuntimeRuby},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := LoadProfile(tt.name)
			if err != nil {
				t.Fatalf("LoadProfile(%q) failed: %v", tt.name, err)
			}
			if p.Name != tt.name {
				t.Errorf("Name = %q, want %q", p.Name, tt.name)
			}
			if p.Runtime != tt.wantRuntime {
				t.Errorf("Runtime = %q, want %q", p.Runtime, tt.wantRuntime)
			}
			if !p.Enabled {
				t.Errorf("Enabled = false, want true")
			}
			if p.Network != NetworkNone {
				t.Errorf("Network = %v, want %v", p.Network, NetworkNone)
			}
			if !p.NoExec {
				t.Errorf("NoExec = false, want true")
			}
			hasTmp := false
			for _, m := range p.Mounts {
				if m.Destination == "/tmp" {
					hasTmp = true
					break
				}
			}
			if !hasTmp {
				t.Errorf("expected /tmp mount in profile %q", tt.name)
			}
		})
	}
}

func TestResolvePermissions_RuntimeProfiles(t *testing.T) {
	tmpCwd := t.TempDir()
	venv := filepath.Join(tmpCwd, ".venv")
	if err := os.MkdirAll(venv, 0755); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpCwd); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	perms, err := ResolvePermissions(ProfilePythonEnv, nil)
	if err != nil {
		t.Fatalf("ResolvePermissions(ProfilePythonEnv) failed: %v", err)
	}
	if perms == nil {
		t.Fatal("expected non-nil permissions")
	}

	// Verify that compiled permissions include read rules for auto-discovered runtime mount
	foundRule := false
	expectedPrefix := fmt.Sprintf("fs.read(%s", filepath.Clean(venv))
	realVenv, err := filepath.EvalSymlinks(venv)
	var realPrefix string
	if err == nil {
		realPrefix = fmt.Sprintf("fs.read(%s", realVenv)
	}

	for _, rule := range perms.Allow {
		if strings.HasPrefix(rule, expectedPrefix) || (realPrefix != "" && strings.HasPrefix(rule, realPrefix)) {
			foundRule = true
			break
		}
	}
	if !foundRule {
		t.Errorf("expected read permission rule starting with %q (or %q) in %v", expectedPrefix, realPrefix, perms.Allow)
	}
}
