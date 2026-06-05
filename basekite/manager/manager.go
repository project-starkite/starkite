// Package manager provides module installation and management for starkite.
package manager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/project-starkite/starkite/libkite"
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

// Install installs a starlark module from a git repository or a local directory.
// The module's identity (namespace/name) comes from its module.yaml; the install
// source only supplies a fallback namespace (the git org) and provenance.
func (m *Manager) Install(source string, opts InstallOptions) (*ModuleInfo, error) {
	// Stage the module in a temp dir so its manifest can be read before deciding
	// where it lands.
	staging, err := os.MkdirTemp("", "starkite-install-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer os.RemoveAll(staging)

	var repo, version, sourceNamespace string
	if isLocalPath(source) {
		// Local directory install — copy the tree.
		abs, err := filepath.Abs(expandHome(source))
		if err != nil {
			return nil, fmt.Errorf("cannot resolve path: %w", err)
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("local module source must be an existing directory: %s", source)
		}
		if err := copyDir(abs, staging); err != nil {
			return nil, fmt.Errorf("failed to copy local module: %w", err)
		}
		repo = abs
	} else {
		// Git install — clone into staging.
		repo, version = ParseSource(source)
		// Only a hosted repo (https/ssh/git@) carries a meaningful org to use
		// as the fallback namespace; file:// and bare local clones do not.
		if isHostedRepo(repo) {
			sourceNamespace, _ = InferNamespaceName(repo)
		}
		if err := GitClone(repo, version, staging); err != nil {
			return nil, fmt.Errorf("failed to clone repository: %w", err)
		}
		if version == "" {
			if commit, err := GitGetCurrentCommit(staging); err == nil {
				version = commit
			}
		}
		// The installed module is the source tree, not a working clone — drop
		// the .git directory.
		os.RemoveAll(filepath.Join(staging, ".git"))
	}

	// Read the author's manifest — the source of truth for identity.
	manifest, err := parseStarlarkManifest(staging)
	if err != nil {
		return nil, fmt.Errorf("invalid module: %w", err)
	}

	namespace, name, err := resolveIdentity(manifest, sourceNamespace, opts.Name)
	if err != nil {
		return nil, err
	}

	if err := m.validateModule(staging, name); err != nil {
		return nil, fmt.Errorf("invalid module structure: %w", err)
	}

	destPath := filepath.Join(m.starlarkDir, namespace, name)
	if _, err := os.Stat(destPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("module %q already installed at %s (use --force to overwrite)", namespace+"/"+name, destPath)
		}
		if err := os.RemoveAll(destPath); err != nil {
			return nil, fmt.Errorf("failed to remove existing module: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create namespace directory: %w", err)
	}
	if err := os.Rename(staging, destPath); err != nil {
		// Rename can fail across filesystems; fall back to copy.
		if err := copyDir(staging, destPath); err != nil {
			return nil, fmt.Errorf("failed to install module: %w", err)
		}
	}

	// Record an install receipt — never overwrite the author's module.yaml.
	prov := &Provenance{
		Namespace:     namespace,
		Name:          name,
		Source:        repo,
		Version:       version,
		InstalledFrom: source,
	}
	if err := WriteProvenance(destPath, prov); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write install receipt: %v\n", err)
	}

	return &ModuleInfo{
		Namespace:   namespace,
		Name:        name,
		Type:        "starlark",
		Path:        destPath,
		Repository:  repo,
		Version:     version,
		Description: manifest.Description,
		Permissions: manifest.Permissions,
	}, nil
}

// resolveIdentity determines the module's (namespace, name). Name comes from the
// manifest (required). Namespace resolves in order: manifest, then the source
// (git org), then an explicit --as "namespace/name". An explicit namespace must
// not conflict with the manifest's.
func resolveIdentity(manifest *libkite.ModuleManifest, sourceNamespace, asOpt string) (namespace, name string, err error) {
	var asNamespace, asName string
	if asOpt != "" {
		if ns, n, ok := strings.Cut(asOpt, "/"); ok {
			asNamespace, asName = ns, n
		} else {
			asName = asOpt
		}
	}

	name = manifest.Name
	if name == "" {
		name = asName
	}
	if name == "" {
		return "", "", fmt.Errorf("module.yaml is missing required field: name")
	}

	namespace = manifest.Namespace
	switch {
	case namespace == "":
		namespace = firstNonEmpty(asNamespace, sourceNamespace)
	case asNamespace != "" && asNamespace != namespace:
		return "", "", fmt.Errorf("--as namespace %q conflicts with manifest namespace %q", asNamespace, namespace)
	case sourceNamespace != "" && sourceNamespace != namespace:
		return "", "", fmt.Errorf("source org %q does not match manifest namespace %q", sourceNamespace, namespace)
	}

	if namespace == "" {
		return "", "", fmt.Errorf("module %q has no namespace; declare it in module.yaml or install with --as <namespace>/<name>", name)
	}
	return namespace, name, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// isHostedRepo reports whether a repo reference points at a network git host
// (and therefore carries an org usable as a fallback namespace). file:// and
// local paths do not.
func isHostedRepo(repo string) bool {
	if strings.HasPrefix(repo, "git@") ||
		strings.HasPrefix(repo, "https://") ||
		strings.HasPrefix(repo, "http://") ||
		strings.HasPrefix(repo, "ssh://") {
		return true
	}
	// "host/org/repo" shape (dotted first segment), no scheme.
	if first, _, ok := strings.Cut(repo, "/"); ok && strings.Contains(first, ".") {
		return true
	}
	return false
}

// InstallOptions holds options for module installation.
type InstallOptions struct {
	Name  string // Custom name for the module (default: inferred from repo)
	Force bool   // Overwrite existing module
}

// ModuleInfo holds information about an installed module.
type ModuleInfo struct {
	Namespace   string
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

// listStarlarkModules lists all installed starlark modules. Modules live under
// starlark/<namespace>/<name>/.
func (m *Manager) listStarlarkModules() ([]*ModuleInfo, error) {
	nsEntries, err := os.ReadDir(m.starlarkDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read starlark modules directory: %w", err)
	}

	var modules []*ModuleInfo
	for _, ns := range nsEntries {
		if !ns.IsDir() {
			continue
		}
		nsDir := filepath.Join(m.starlarkDir, ns.Name())
		modEntries, err := os.ReadDir(nsDir)
		if err != nil {
			continue
		}
		for _, mod := range modEntries {
			if !mod.IsDir() {
				continue
			}
			modulePath := filepath.Join(nsDir, mod.Name())
			modules = append(modules, m.starlarkInfo(ns.Name(), mod.Name(), modulePath))
		}
	}

	return modules, nil
}

// starlarkInfo builds a ModuleInfo for an installed starlark module by reading
// its manifest.
func (m *Manager) starlarkInfo(namespace, name, modulePath string) *ModuleInfo {
	info := &ModuleInfo{
		Namespace: namespace,
		Name:      name,
		Type:      "starlark",
		Path:      modulePath,
	}
	if manifest, err := parseStarlarkManifest(modulePath); err == nil {
		info.Version = manifest.Version
		info.Description = manifest.Description
		info.Permissions = manifest.Permissions
		info.EntryPoint = filepath.Join(modulePath, manifest.EntryFile())
	}
	if prov, err := ReadProvenance(modulePath); err == nil && prov != nil {
		info.Repository = prov.Source
		if info.Version == "" {
			info.Version = prov.Version
		}
	}
	return info
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

// Get returns information about a specific module. ref is "namespace/name" for
// starlark modules (or a bare name for WASM modules, which are flat).
func (m *Manager) Get(ref string) (*ModuleInfo, error) {
	// Starlark: namespace/name.
	if ns, name, ok := strings.Cut(ref, "/"); ok {
		starlarkPath := filepath.Join(m.starlarkDir, ns, name)
		if info, err := os.Stat(starlarkPath); err == nil && info.IsDir() {
			return m.starlarkInfo(ns, name, starlarkPath), nil
		}
	}

	// WASM: bare name.
	wasmPath := filepath.Join(m.wasmDir, ref)
	if info, err := os.Stat(wasmPath); err == nil && info.IsDir() {
		return m.getWasmModule(ref, wasmPath)
	}

	return nil, fmt.Errorf("module %q not installed", ref)
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

// Update updates an installed starlark module to the latest version. ref is
// "namespace/name".
func (m *Manager) Update(ref string) (*ModuleInfo, error) {
	if ns, name, ok := strings.Cut(ref, "/"); ok {
		starlarkPath := filepath.Join(m.starlarkDir, ns, name)
		if _, err := os.Stat(starlarkPath); err == nil {
			return m.updateStarlarkModule(ns, name, starlarkPath)
		}
	}

	wasmPath := filepath.Join(m.wasmDir, ref)
	if _, err := os.Stat(wasmPath); err == nil {
		return nil, fmt.Errorf("WASM modules cannot be updated; reinstall with --force")
	}

	return nil, fmt.Errorf("module %q not installed", ref)
}

// updateStarlarkModule updates a starlark module via git pull.
func (m *Manager) updateStarlarkModule(namespace, name, modulePath string) (*ModuleInfo, error) {
	if !GitIsRepo(modulePath) {
		return nil, fmt.Errorf("module %q is not git-managed; reinstall to update", namespace+"/"+name)
	}

	newVersion, err := GitPull(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to update module: %w", err)
	}

	if prov, err := ReadProvenance(modulePath); err == nil && prov != nil {
		prov.Version = newVersion
		if err := WriteProvenance(modulePath, prov); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update provenance: %v\n", err)
		}
	}

	info := m.starlarkInfo(namespace, name, modulePath)
	info.Version = newVersion
	return info, nil
}

// Remove removes an installed module. ref is "namespace/name" for starlark
// modules (or a bare name for WASM modules).
func (m *Manager) Remove(ref string) error {
	if ns, name, ok := strings.Cut(ref, "/"); ok {
		starlarkPath := filepath.Join(m.starlarkDir, ns, name)
		if _, err := os.Stat(starlarkPath); err == nil {
			if err := os.RemoveAll(starlarkPath); err != nil {
				return err
			}
			// Prune the namespace directory if now empty.
			pruneEmptyDir(filepath.Dir(starlarkPath))
			return nil
		}
	}

	wasmPath := filepath.Join(m.wasmDir, ref)
	if _, err := os.Stat(wasmPath); err == nil {
		return os.RemoveAll(wasmPath)
	}

	return fmt.Errorf("module %q not installed", ref)
}

// pruneEmptyDir removes dir if it contains no entries.
func pruneEmptyDir(dir string) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		os.Remove(dir)
	}
}

// validateModule checks if the module has a valid structure.
func (m *Manager) validateModule(modulePath, name string) error {
	// A module must carry a module.yaml manifest.
	manifestPath := filepath.Join(modulePath, "module.yaml")
	if !fileExists(manifestPath) {
		return fmt.Errorf("module is missing module.yaml at its root")
	}

	// The manifest's entry file (default main.star) must exist.
	entryPoint := m.findEntryPoint(modulePath, name)
	if entryPoint == "" {
		return fmt.Errorf("no entry .star file found (expected main.star or %s.star)", name)
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

// InferModuleName extracts the repository name (the last path segment) from a
// repository URL, with no assumption about the git host.
func InferModuleName(repo string) string {
	_, name := InferNamespaceName(repo)
	return name
}

// InferNamespaceName extracts (org, repo) from a repository reference of any
// git host. The org becomes the fallback namespace when the module's manifest
// does not declare one. Either value may be empty if it cannot be determined.
//
// Handled forms (host-agnostic):
//
//	https://host/org/repo(.git)
//	ssh://host/org/repo
//	git@host:org/repo(.git)
//	file:///path/to/repo  -> ("", repo)
//	/local/path/to/repo   -> ("", repo)
func InferNamespaceName(repo string) (namespace, name string) {
	repo = strings.TrimSuffix(repo, ".git")

	// scp-like: git@host:org/repo
	if strings.HasPrefix(repo, "git@") {
		if _, rest, ok := strings.Cut(repo, ":"); ok {
			repo = rest // "org/repo"
		}
	} else {
		// Strip a scheme if present.
		if _, rest, ok := strings.Cut(repo, "://"); ok {
			repo = rest // "host/org/repo" or "/path/to/repo"
		}
	}

	segs := splitNonEmpty(repo, "/")
	switch {
	case len(segs) >= 3:
		// host / org / repo  (drop the host)
		return segs[len(segs)-2], segs[len(segs)-1]
	case len(segs) == 2:
		// org / repo
		return segs[0], segs[1]
	case len(segs) == 1:
		return "", segs[0]
	default:
		return "", ""
	}
}

// splitNonEmpty splits s on sep and drops empty fields.
func splitNonEmpty(s, sep string) []string {
	var out []string
	for _, p := range strings.Split(s, sep) {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
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

// parseStarlarkManifest reads and validates the module.yaml of a starlark
// module directory, returning its declared identity and configuration.
func parseStarlarkManifest(dir string) (*libkite.ModuleManifest, error) {
	return libkite.LoadModuleManifest(dir)
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~"+string(filepath.Separator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// copyDir recursively copies the contents of src into dst, skipping a top-level
// .git directory.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(s, d); err != nil {
			return err
		}
	}
	return nil
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
