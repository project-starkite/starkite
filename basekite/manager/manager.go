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

	// Ensure all directories exist
	for _, dir := range []string{rootDir, starlarkDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create directory %s: %w", dir, err)
		}
	}

	return &Manager{
		rootDir:     rootDir,
		starlarkDir: starlarkDir,
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

// Install installs a starlark module from a git repository or a local directory.
// The module's identity (namespace/name) comes from its mod.yaml; the install
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

	// Record an install receipt — never overwrite the author's mod.yaml.
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
		return "", "", fmt.Errorf("mod.yaml is missing required field: name")
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
		return "", "", fmt.Errorf("module %q has no namespace; declare it in mod.yaml or install with --as <namespace>/<name>", name)
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
	Type        string // "starlark"
	Path        string
	Repository  string
	Version     string
	Description string
	EntryPoint  string
}

// List returns all installed modules.
func (m *Manager) List() ([]*ModuleInfo, error) {
	return m.listStarlarkModules()
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
		info.EntryPoint = filepath.Join(modulePath, libkite.EntryFile)
	}
	if prov, err := ReadProvenance(modulePath); err == nil && prov != nil {
		info.Repository = prov.Source
		if info.Version == "" {
			info.Version = prov.Version
		}
	}
	return info
}

// Get returns information about a specific module. ref is "namespace/name".
func (m *Manager) Get(ref string) (*ModuleInfo, error) {
	if ns, name, ok := strings.Cut(ref, "/"); ok {
		starlarkPath := filepath.Join(m.starlarkDir, ns, name)
		if info, err := os.Stat(starlarkPath); err == nil && info.IsDir() {
			return m.starlarkInfo(ns, name, starlarkPath), nil
		}
	}

	return nil, fmt.Errorf("module %q not installed", ref)
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

// Remove removes an installed module. ref is "namespace/name".
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

	return fmt.Errorf("module %q not installed", ref)
}

// pruneEmptyDir removes dir if it contains no entries.
func pruneEmptyDir(dir string) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		os.Remove(dir)
	}
}

// validateModule checks that the module has a valid structure: a mod.yaml
// manifest and the fixed main.star entry file at its root.
func (m *Manager) validateModule(modulePath, name string) error {
	if !fileExists(filepath.Join(modulePath, metadataFile)) {
		return fmt.Errorf("module is missing %s at its root", metadataFile)
	}
	if !fileExists(filepath.Join(modulePath, libkite.EntryFile)) {
		return fmt.Errorf("module is missing its %s entry file", libkite.EntryFile)
	}
	return nil
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
	return false
}

// parseStarlarkManifest reads and validates the mod.yaml of a starlark
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
