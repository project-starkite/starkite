package edition

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsKnownEdition(t *testing.T) {
	tests := []struct {
		name     string
		edition  string
		expected bool
	}{
		{"cloud is known", "cloud", true},
		{"base is not in KnownEditions", "base", false},
		{"empty is not known", "", false},
		{"random is not known", "random", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownEdition(tt.edition); got != tt.expected {
				t.Errorf("IsKnownEdition(%q) = %v, want %v", tt.edition, got, tt.expected)
			}
		})
	}
}

func TestEditionBinaryPath(t *testing.T) {
	// Override HOME for testing
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := EditionBinaryPath("cloud")
	if err != nil {
		t.Fatalf("EditionBinaryPath failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".starkite", "editions", "cloud", "kite")
	if path != expected {
		t.Errorf("EditionBinaryPath = %q, want %q", path, expected)
	}
}

func TestEditionsDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir, err := EditionsDir()
	if err != nil {
		t.Fatalf("EditionsDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".starkite", "editions")
	if dir != expected {
		t.Errorf("EditionsDir = %q, want %q", dir, expected)
	}

	// Directory should have been created
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("EditionsDir did not create the directory")
	}
}

func TestStarkiteDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	dir, err := StarkiteDir()
	if err != nil {
		t.Fatalf("StarkiteDir failed: %v", err)
	}

	expected := filepath.Join(tmpDir, ".starkite")
	if dir != expected {
		t.Errorf("StarkiteDir = %q, want %q", dir, expected)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("StarkiteDir did not create the directory")
	}
}

// Config tests

func TestActiveEditionDefault(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// No config file exists — should return "base"
	active := ActiveEdition()
	if active != EditionBase {
		t.Errorf("ActiveEdition() = %q, want %q", active, EditionBase)
	}
}

func TestActiveEditionRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Set active edition to cloud
	if err := SetActiveEdition("cloud"); err != nil {
		t.Fatalf("SetActiveEdition failed: %v", err)
	}

	active := ActiveEdition()
	if active != "cloud" {
		t.Errorf("ActiveEdition() = %q, want %q", active, "cloud")
	}

	// Set back to base (should remove key)
	if err := SetActiveEdition(EditionBase); err != nil {
		t.Fatalf("SetActiveEdition(base) failed: %v", err)
	}

	active = ActiveEdition()
	if active != EditionBase {
		t.Errorf("ActiveEdition() = %q, want %q", active, EditionBase)
	}
}

func TestSetActiveEditionPreservesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a config with existing data
	configDir := filepath.Join(tmpDir, ".starkite")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	existingConfig := `project:
    name: test-project
    version: 1.0.0
defaults:
    log_level: debug
`
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(existingConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Set active edition
	if err := SetActiveEdition("cloud"); err != nil {
		t.Fatalf("SetActiveEdition failed: %v", err)
	}

	// Read back and verify project data preserved
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !contains(content, "test-project") {
		t.Error("SetActiveEdition did not preserve project name")
	}
	if !contains(content, "active_edition") {
		t.Error("SetActiveEdition did not write active_edition")
	}
	if !contains(content, "cloud") {
		t.Error("SetActiveEdition did not write edition value")
	}
}

// Install tests

func TestInstallFromLocal(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Create a fake binary
	srcBinary := filepath.Join(tmpDir, "fake-kite")
	if err := os.WriteFile(srcBinary, []byte("#!/bin/sh\necho cloud"), 0755); err != nil {
		t.Fatal(err)
	}

	// Install from local
	err := Install("cloud", InstallOptions{FromPath: srcBinary})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify binary was copied
	binaryPath, _ := EditionBinaryPath("cloud")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		t.Error("binary was not installed")
	}
}

func TestInstallFromLocalAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	srcBinary := filepath.Join(tmpDir, "fake-kite")
	if err := os.WriteFile(srcBinary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Install first time
	if err := Install("cloud", InstallOptions{FromPath: srcBinary}); err != nil {
		t.Fatal(err)
	}

	// Install again without force — should fail
	err := Install("cloud", InstallOptions{FromPath: srcBinary})
	if err == nil {
		t.Error("expected error when installing existing edition without --force")
	}

	// Install again with force — should succeed
	err = Install("cloud", InstallOptions{FromPath: srcBinary, Force: true})
	if err != nil {
		t.Errorf("Install with --force failed: %v", err)
	}
}

func TestInstallMissingSource(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := Install("cloud", InstallOptions{FromPath: "/nonexistent/path"})
	if err == nil {
		t.Error("expected error when source does not exist")
	}
}

func TestInstallUnknownEdition(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := Install("unknown", InstallOptions{FromPath: "/some/path"})
	if err == nil {
		t.Error("expected error for unknown edition")
	}
}

// Remove tests

func TestRemove(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Install first
	srcBinary := filepath.Join(tmpDir, "fake-kite")
	if err := os.WriteFile(srcBinary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Install("cloud", InstallOptions{FromPath: srcBinary}); err != nil {
		t.Fatal(err)
	}

	// Remove
	if err := Remove("cloud"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify removed
	binaryPath, _ := EditionBinaryPath("cloud")
	if _, err := os.Stat(filepath.Dir(binaryPath)); !os.IsNotExist(err) {
		t.Error("edition directory was not removed")
	}
}

func TestRemoveResetsActiveEdition(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Install and set as active
	srcBinary := filepath.Join(tmpDir, "fake-kite")
	if err := os.WriteFile(srcBinary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Install("cloud", InstallOptions{FromPath: srcBinary}); err != nil {
		t.Fatal(err)
	}
	if err := SetActiveEdition("cloud"); err != nil {
		t.Fatal(err)
	}

	// Verify it's active
	if ActiveEdition() != "cloud" {
		t.Fatal("edition should be cloud before remove")
	}

	// Remove — should reset active to base
	if err := Remove("cloud"); err != nil {
		t.Fatal(err)
	}

	if ActiveEdition() != EditionBase {
		t.Errorf("ActiveEdition() = %q after remove, want %q", ActiveEdition(), EditionBase)
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := Remove("cloud")
	if err == nil {
		t.Error("expected error when removing non-installed edition")
	}
}

func TestRemoveBase(t *testing.T) {
	err := Remove("base")
	if err == nil {
		t.Error("expected error when removing base edition")
	}
}

// List tests

func TestListInstalledEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	editions, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}

	// Should always have base
	if len(editions) != 1 {
		t.Errorf("expected 1 edition (base), got %d", len(editions))
	}
	if editions[0].Name != EditionBase {
		t.Errorf("first edition should be base, got %q", editions[0].Name)
	}
	if !editions[0].Active {
		t.Error("base should be active when no other edition is set")
	}
}

func TestListInstalledWithEditions(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Install cloud
	srcBinary := filepath.Join(tmpDir, "fake-kite")
	if err := os.WriteFile(srcBinary, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Install("cloud", InstallOptions{FromPath: srcBinary}); err != nil {
		t.Fatal(err)
	}

	editions, err := ListInstalled()
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}

	if len(editions) != 2 {
		t.Errorf("expected 2 editions, got %d", len(editions))
	}

	// Find cloud edition
	var cloud *EditionInfo
	for _, e := range editions {
		if e.Name == "cloud" {
			cloud = e
			break
		}
	}
	if cloud == nil {
		t.Fatal("cloud edition not found in list")
	}
	if !cloud.Installed {
		t.Error("cloud edition should be marked as installed")
	}
}

// DownloadURL tests

func TestDownloadURL(t *testing.T) {
	url := DownloadURL("cloud")

	expectedSuffix := "kite-cloud-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		expectedSuffix += ".exe"
	}

	if !contains(url, expectedSuffix) {
		t.Errorf("DownloadURL = %q, expected to contain %q", url, expectedSuffix)
	}
	if !contains(url, "github.com") {
		t.Errorf("DownloadURL = %q, expected to contain github.com", url)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
