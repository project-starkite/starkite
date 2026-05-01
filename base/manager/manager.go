// Package manager provides module installation and management for starkite.
package manager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Manager handles module installation, updates, and removal.
type Manager struct {
	rootDir     string // ~/.starkite/modules/
	starlarkDir string // ~/.starkite/modules/starlark/
	wasmDir     string // ~/.starkite/modules/wasm/
}

// New creates a new module manager.
// If rootDir is empty, uses ~/.starkite/modules/
func New(rootDir string) (*Manager, error) {
	if rootDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		rootDir = filepath.Join(home, ".starkite", "modules")
	}

	starlarkDir := filepath.Join(rootDir, "starlark")
	wasmDir := filepath.Join(rootDir, "wasm")

	// Ensure all directories exist
	for _, dir := range []string{rootDir, starlarkDir, wasmDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create directory %s: %w", dir, err)
		}
	}

	return &Manager{
		rootDir:     rootDir,
		starlarkDir: starlarkDir,
		wasmDir:     wasmDir,
	}, nil
}

// ModulesDir returns the root modules directory path.
func (m *Manager) ModulesDir() string {
	return m.rootDir
}

// StarlarkDir returns the starlark modules directory path.
func (m *Manager) StarlarkDir() string {
	return m.starlarkDir
}

// WasmDir returns the WASM modules directory path.
func (m *Manager) WasmDir() string {
	return m.wasmDir
}

// Install installs a starlark module from a git repository.
func (m *Manager) Install(source string, opts InstallOptions) (*ModuleInfo, error) {
	// Parse source to extract repo and version
	repo, version := ParseSource(source)

	// Determine module name
	name := opts.Name
	if name == "" {
		name = InferModuleName(repo)
	}

	destPath := filepath.Join(m.starlarkDir, name)

	// Check if already installed
	if _, err := os.Stat(destPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("module %q already installed at %s (use --force to overwrite)", name, destPath)
		}
		// Remove existing installation
		if err := os.RemoveAll(destPath); err != nil {
			return nil, fmt.Errorf("failed to remove existing module: %w", err)
		}
	}

	// Clone the repository
	if err := GitClone(repo, version, destPath); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Validate the module structure
	if err := m.validateModule(destPath, name); err != nil {
		os.RemoveAll(destPath)
		return nil, fmt.Errorf("invalid module structure: %w", err)
	}

	// Write metadata
	meta := &Metadata{
		Name:       name,
		Repository: repo,
		Version:    version,
	}
	if err := WriteMetadata(destPath, meta); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "warning: failed to write metadata: %v\n", err)
	}

	return &ModuleInfo{
		Name:       name,
		Type:       "starlark",
		Path:       destPath,
		Repository: repo,
		Version:    version,
	}, nil
}

// InstallOptions holds options for module installation.
type InstallOptions struct {
	Name  string // Custom name for the module (default: inferred from repo)
	Force bool   // Overwrite existing module
}

// ModuleInfo holds information about an installed module.
type ModuleInfo struct {
	Name        string
	Type        string // "starlark" or "wasm"
	Path        string
	Repository  string
	Version     string
	Description string
	EntryPoint  string
	// WASM-specific (empty for starlark modules)
	Functions   []string
	Permissions []string
	WasmFile    string
}

// InstallWasm installs a WASM module from a local path or git repository.
func (m *Manager) InstallWasm(source string, opts InstallOptions) (*ModuleInfo, error) {
	if isLocalPath(source) {
		return m.installWasmFromLocal(source, opts)
	}
	return m.installWasmFromGit(source, opts)
}

// installWasmFromLocal installs a WASM module from a local directory or .wasm file.
func (m *Manager) installWasmFromLocal(source string, opts InstallOptions) (*ModuleInfo, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve path: %w", err)
	}

	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("source not found: %w", err)
	}

	var sourceDir string
	if info.IsDir() {
		sourceDir = source
	} else if strings.HasSuffix(source, ".wasm") {
		sourceDir = filepath.Dir(source)
	} else {
		return nil, fmt.Errorf("source must be a directory or .wasm file")
	}

	// Parse manifest
	manifestPath := filepath.Join(sourceDir, metadataFile)
	manifest, err := parseWasmManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("invalid WASM module: %w", err)
	}

	// Verify .wasm file exists in source
	wasmPath := filepath.Join(sourceDir, manifest.Wasm)
	if !fileExists(wasmPath) {
		return nil, fmt.Errorf("WASM file %q not found in %s", manifest.Wasm, sourceDir)
	}

	name := opts.Name
	if name == "" {
		name = manifest.Name
	}

	destPath := filepath.Join(m.wasmDir, name)

	// Check if already installed
	if _, err := os.Stat(destPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("module %q already installed at %s (use --force to overwrite)", name, destPath)
		}
		if err := os.RemoveAll(destPath); err != nil {
			return nil, fmt.Errorf("failed to remove existing module: %w", err)
		}
	}

	// Create destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create module directory: %w", err)
	}

	// Copy module.yaml
	if err := copyFile(manifestPath, filepath.Join(destPath, metadataFile)); err != nil {
		os.RemoveAll(destPath)
		return nil, fmt.Errorf("failed to copy manifest: %w", err)
	}

	// Copy .wasm file
	if err := copyFile(wasmPath, filepath.Join(destPath, manifest.Wasm)); err != nil {
		os.RemoveAll(destPath)
		return nil, fmt.Errorf("failed to copy WASM file: %w", err)
	}

	// Build function name list
	var funcNames []string
	for _, fn := range manifest.Functions {
		funcNames = append(funcNames, fn.Name)
	}

	return &ModuleInfo{
		Name:        name,
		Type:        "wasm",
		Path:        destPath,
		Version:     manifest.Version,
		Description: manifest.Description,
		WasmFile:    manifest.Wasm,
		Functions:   funcNames,
		Permissions: manifest.Permissions,
	}, nil
}

// installWasmFromGit installs a WASM module from a git repository.
func (m *Manager) installWasmFromGit(source string, opts InstallOptions) (*ModuleInfo, error) {
	repo, version := ParseSource(source)

	// Clone to a temporary directory
	tmpDir, err := os.MkdirTemp("", "starkite-wasm-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := GitClone(repo, version, tmpDir); err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Parse manifest from cloned repo
	manifestPath := filepath.Join(tmpDir, metadataFile)
	manifest, err := parseWasmManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("invalid WASM module: %w", err)
	}

	// Verify .wasm file exists
	wasmPath := filepath.Join(tmpDir, manifest.Wasm)
	if !fileExists(wasmPath) {
		return nil, fmt.Errorf("WASM file %q not found in repository", manifest.Wasm)
	}

	name := opts.Name
	if name == "" {
		name = manifest.Name
	}

	destPath := filepath.Join(m.wasmDir, name)

	// Check if already installed
	if _, err := os.Stat(destPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("module %q already installed at %s (use --force to overwrite)", name, destPath)
		}
		if err := os.RemoveAll(destPath); err != nil {
			return nil, fmt.Errorf("failed to remove existing module: %w", err)
		}
	}

	// Create destination directory
	if err := os.MkdirAll(destPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create module directory: %w", err)
	}

	// Copy module.yaml
	if err := copyFile(manifestPath, filepath.Join(destPath, metadataFile)); err != nil {
		os.RemoveAll(destPath)
		return nil, fmt.Errorf("failed to copy manifest: %w", err)
	}

	// Copy .wasm file
	if err := copyFile(wasmPath, filepath.Join(destPath, manifest.Wasm)); err != nil {
		os.RemoveAll(destPath)
		return nil, fmt.Errorf("failed to copy WASM file: %w", err)
	}

	// Build function name list
	var funcNames []string
	for _, fn := range manifest.Functions {
		funcNames = append(funcNames, fn.Name)
	}

	return &ModuleInfo{
		Name:        name,
		Type:        "wasm",
		Path:        destPath,
		Repository:  repo,
		Version:     manifest.Version,
		Description: manifest.Description,
		WasmFile:    manifest.Wasm,
		Functions:   funcNames,
		Permissions: manifest.Permissions,
	}, nil
}

// List returns all installed modules (starlark and wasm).
func (m *Manager) List() ([]*ModuleInfo, error) {
	starlark, err := m.listStarlarkModules()
	if err != nil {
		return nil, err
	}

	wasm, err := m.listWasmModules()
	if err != nil {
		return nil, err
	}

	return append(starlark, wasm...), nil
}

// listStarlarkModules lists all installed starlark modules.
func (m *Manager) listStarlarkModules() ([]*ModuleInfo, error) {
	entries, err := os.ReadDir(m.starlarkDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read starlark modules directory: %w", err)
	}

	var modules []*ModuleInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		modulePath := filepath.Join(m.starlarkDir, name)

		info := &ModuleInfo{
			Name: name,
			Type: "starlark",
			Path: modulePath,
		}

		// Try to read metadata
		if meta, err := ReadMetadata(modulePath); err == nil {
			info.Repository = meta.Repository
			info.Version = meta.Version
			info.Description = meta.Description
		}

		// Find entry point
		info.EntryPoint = m.findEntryPoint(modulePath, name)

		modules = append(modules, info)
	}

	return modules, nil
}

// listWasmModules lists all installed WASM modules.
func (m *Manager) listWasmModules() ([]*ModuleInfo, error) {
	entries, err := os.ReadDir(m.wasmDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read wasm modules directory: %w", err)
	}

	var modules []*ModuleInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		modulePath := filepath.Join(m.wasmDir, name)

		manifestPath := filepath.Join(modulePath, metadataFile)
		manifest, err := parseWasmManifest(manifestPath)
		if err != nil {
			continue // skip invalid WASM modules
		}

		var funcNames []string
		for _, fn := range manifest.Functions {
			funcNames = append(funcNames, fn.Name)
		}

		modules = append(modules, &ModuleInfo{
			Name:        name,
			Type:        "wasm",
			Path:        modulePath,
			Version:     manifest.Version,
			Description: manifest.Description,
			WasmFile:    manifest.Wasm,
			Functions:   funcNames,
			Permissions: manifest.Permissions,
		})
	}

	return modules, nil
}

// Get returns information about a specific module.
func (m *Manager) Get(name string) (*ModuleInfo, error) {
	// Check starlark dir first
	starlarkPath := filepath.Join(m.starlarkDir, name)
	if info, err := os.Stat(starlarkPath); err == nil && info.IsDir() {
		return m.getStarlarkModule(name, starlarkPath)
	}

	// Check wasm dir
	wasmPath := filepath.Join(m.wasmDir, name)
	if info, err := os.Stat(wasmPath); err == nil && info.IsDir() {
		return m.getWasmModule(name, wasmPath)
	}

	return nil, fmt.Errorf("module %q not installed", name)
}

// getStarlarkModule builds ModuleInfo for a starlark module.
func (m *Manager) getStarlarkModule(name, modulePath string) (*ModuleInfo, error) {
	moduleInfo := &ModuleInfo{
		Name: name,
		Type: "starlark",
		Path: modulePath,
	}

	// Try to read metadata
	if meta, err := ReadMetadata(modulePath); err == nil {
		moduleInfo.Repository = meta.Repository
		moduleInfo.Version = meta.Version
		moduleInfo.Description = meta.Description
	}

	// Find entry point
	moduleInfo.EntryPoint = m.findEntryPoint(modulePath, name)

	return moduleInfo, nil
}

// getWasmModule builds ModuleInfo for a WASM module.
func (m *Manager) getWasmModule(name, modulePath string) (*ModuleInfo, error) {
	manifestPath := filepath.Join(modulePath, metadataFile)
	manifest, err := parseWasmManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read WASM module manifest: %w", err)
	}

	var funcNames []string
	for _, fn := range manifest.Functions {
		funcNames = append(funcNames, fn.Name)
	}

	return &ModuleInfo{
		Name:        name,
		Type:        "wasm",
		Path:        modulePath,
		Version:     manifest.Version,
		Description: manifest.Description,
		WasmFile:    manifest.Wasm,
		Functions:   funcNames,
		Permissions: manifest.Permissions,
	}, nil
}

// Update updates an installed module to the latest version.
func (m *Manager) Update(name string) (*ModuleInfo, error) {
	// Check starlark dir first
	starlarkPath := filepath.Join(m.starlarkDir, name)
	if _, err := os.Stat(starlarkPath); err == nil {
		return m.updateStarlarkModule(name, starlarkPath)
	}

	// Check wasm dir
	wasmPath := filepath.Join(m.wasmDir, name)
	if _, err := os.Stat(wasmPath); err == nil {
		return nil, fmt.Errorf("WASM modules cannot be updated; reinstall with --force")
	}

	return nil, fmt.Errorf("module %q not installed", name)
}

// updateStarlarkModule updates a starlark module via git pull.
func (m *Manager) updateStarlarkModule(name, modulePath string) (*ModuleInfo, error) {
	meta, err := ReadMetadata(modulePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read module metadata (module may not be git-managed): %w", err)
	}

	if meta.Repository == "" {
		return nil, fmt.Errorf("module %q has no repository information", name)
	}

	// Pull latest changes
	newVersion, err := GitPull(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to update module: %w", err)
	}

	// Update metadata
	meta.Version = newVersion
	if err := WriteMetadata(modulePath, meta); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to update metadata: %v\n", err)
	}

	return &ModuleInfo{
		Name:       name,
		Type:       "starlark",
		Path:       modulePath,
		Repository: meta.Repository,
		Version:    newVersion,
	}, nil
}

// Remove removes an installed module.
func (m *Manager) Remove(name string) error {
	// Check starlark dir first
	starlarkPath := filepath.Join(m.starlarkDir, name)
	if _, err := os.Stat(starlarkPath); err == nil {
		return os.RemoveAll(starlarkPath)
	}

	// Check wasm dir
	wasmPath := filepath.Join(m.wasmDir, name)
	if _, err := os.Stat(wasmPath); err == nil {
		return os.RemoveAll(wasmPath)
	}

	return fmt.Errorf("module %q not installed", name)
}

// validateModule checks if the module has a valid structure.
func (m *Manager) validateModule(modulePath, name string) error {
	// Check for entry point
	entryPoint := m.findEntryPoint(modulePath, name)
	if entryPoint == "" {
		return fmt.Errorf("no entry point found (expected main.star or %s.star)", name)
	}

	return nil
}

// findEntryPoint finds the module's entry point file.
func (m *Manager) findEntryPoint(modulePath, name string) string {
	// Check for main.star
	mainPath := filepath.Join(modulePath, "main.star")
	if fileExists(mainPath) {
		return mainPath
	}

	// Check for <name>.star
	namedPath := filepath.Join(modulePath, name+".star")
	if fileExists(namedPath) {
		return namedPath
	}

	// Check for any .star file
	entries, err := os.ReadDir(modulePath)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".star") {
			return filepath.Join(modulePath, entry.Name())
		}
	}

	return ""
}

// ParseSource parses a module source string into repository and version.
// Supports formats:
//   - github.com/user/repo
//   - github.com/user/repo@v1.0.0
//   - github.com/user/repo@main
//   - git@github.com:user/repo.git
func ParseSource(source string) (repo, version string) {
	// Check for @version suffix
	if idx := strings.LastIndex(source, "@"); idx > 0 {
		// Make sure @ is not part of git@ prefix
		if !strings.HasPrefix(source, "git@") || strings.Count(source, "@") > 1 {
			repo = source[:idx]
			version = source[idx+1:]
			return
		}
	}

	return source, ""
}

// InferModuleName extracts a module name from a repository URL.
func InferModuleName(repo string) string {
	// Remove .git suffix
	repo = strings.TrimSuffix(repo, ".git")

	// Handle git@ format: git@github.com:user/repo -> repo
	if strings.HasPrefix(repo, "git@") {
		if idx := strings.LastIndex(repo, "/"); idx > 0 {
			return repo[idx+1:]
		}
		if idx := strings.LastIndex(repo, ":"); idx > 0 {
			return repo[idx+1:]
		}
	}

	// Handle https/http format: github.com/user/repo -> repo
	parts := strings.Split(repo, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return repo
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// isLocalPath returns true if source looks like a local filesystem path.
func isLocalPath(source string) bool {
	if filepath.IsAbs(source) {
		return true
	}
	// Relative paths starting with . or ..
	if source == "." || source == ".." ||
		strings.HasPrefix(source, "."+string(filepath.Separator)) ||
		strings.HasPrefix(source, ".."+string(filepath.Separator)) {
		return true
	}
	// Home directory expansion
	if strings.HasPrefix(source, "~"+string(filepath.Separator)) || source == "~" {
		return true
	}
	// Bare .wasm file reference
	if strings.HasSuffix(source, ".wasm") {
		return true
	}
	return false
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}
