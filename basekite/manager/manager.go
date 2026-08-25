// Package manager provides module installation and management for starkite.
package manager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/project-starkite/starkite/libkite"
)

// Manager handles module installation, updates, and removal. Installed modules
// live directly under the modules root as <namespace>/<name>/.
type Manager struct {
	rootDir string // ~/.starkite/modules/
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

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create directory %s: %w", rootDir, err)
	}

	return &Manager{rootDir: rootDir}, nil
}

// ModulesDir returns the root modules directory path.
func (m *Manager) ModulesDir() string {
	return m.rootDir
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

	var repo, version, sourceNamespace, commit string
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
		// Capture the commit as the immutable revision (commit-not-tag), even
		// when a tag or branch was requested.
		commit, _ = GitGetCurrentCommit(staging)
		if version == "" {
			version = commit
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

	// Key the cache directory by an immutable revision: the commit SHA for a git
	// source, or the content hash for a local source. The cache is write-once —
	// a given revision is installed exactly once.
	rev := commit
	if rev == "" {
		h, err := libkite.HashModuleTree(staging)
		if err != nil {
			return nil, fmt.Errorf("failed to hash module: %w", err)
		}
		rev = strings.TrimPrefix(h, "sha256:")
		if len(rev) > 16 {
			rev = rev[:16]
		}
	}

	destPath := filepath.Join(m.rootDir, namespace, name+"@"+rev)
	if _, err := os.Stat(destPath); err == nil {
		// This exact revision is already cached. Reinstalling identical content is
		// a no-op unless forced.
		if !opts.Force {
			return m.starlarkInfo(namespace, name+"@"+rev, destPath), nil
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

	// Compute the portable content hash and a machine-local stat fingerprint of
	// the installed tree for later verification. Both exclude the receipt itself,
	// so they are stable once written.
	hash, err := libkite.HashModuleTree(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to hash installed module: %w", err)
	}
	fingerprint, err := libkite.FingerprintTree(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to fingerprint installed module: %w", err)
	}

	// Record an install receipt — never overwrite the author's mod.yaml.
	prov := &Provenance{
		Namespace:     namespace,
		Name:          name,
		Source:        repo,
		Version:       version,
		Rev:           rev,
		Hash:          hash,
		Fingerprint:   fingerprint,
		InstalledFrom: source,
	}
	if err := WriteProvenance(destPath, prov); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write install receipt: %v\n", err)
	}

	return &ModuleInfo{
		Namespace:   namespace,
		Name:        name,
		Rev:         rev,
		Hash:        hash,
		Type:        "starlark",
		Path:        destPath,
		Repository:  repo,
		Version:     version,
		Description: manifest.Description,
		EntryPoint:  filepath.Join(destPath, libkite.EntryFile),
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
	Namespace string
	Name      string
	// Rev is the immutable revision the cache directory is keyed by: the commit
	// SHA for a git source, or the content hash for a local source.
	Rev string
	// Hash is the portable content hash of the installed tree ("sha256:...").
	Hash        string
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
// <namespace>/<name>/.
func (m *Manager) listStarlarkModules() ([]*ModuleInfo, error) {
	nsEntries, err := os.ReadDir(m.rootDir)
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
		nsDir := filepath.Join(m.rootDir, ns.Name())
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
// its manifest. dirName is the cache directory name "<name>@<rev>".
func (m *Manager) starlarkInfo(namespace, dirName, modulePath string) *ModuleInfo {
	name, rev := libkite.SplitModuleRev(dirName)
	info := &ModuleInfo{
		Namespace: namespace,
		Name:      name,
		Rev:       rev,
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
		info.Hash = prov.Hash
		if info.Version == "" {
			info.Version = prov.Version
		}
	}
	return info
}

// installedRevDirs returns the cache directory names under <rootDir>/<ns> whose
// module name matches name. Each entry is a version-addressed "<name>@<rev>"
// directory (or a bare "<name>" for an unversioned install).
func (m *Manager) installedRevDirs(ns, name string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(m.rootDir, ns))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if n, _ := libkite.SplitModuleRev(e.Name()); n == name {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs, nil
}

// Get returns information about a specific module. ref is "namespace/name". When
// more than one revision is installed the reference is ambiguous and the caller
// must pin a revision in mod.lock.
func (m *Manager) Get(ref string) (*ModuleInfo, error) {
	ns, name, ok := strings.Cut(ref, "/")
	if !ok {
		return nil, fmt.Errorf("module %q not installed", ref)
	}
	dirs, err := m.installedRevDirs(ns, name)
	if err != nil {
		return nil, err
	}
	switch len(dirs) {
	case 0:
		return nil, fmt.Errorf("module %q not installed", ref)
	case 1:
		return m.starlarkInfo(ns, dirs[0], filepath.Join(m.rootDir, ns, dirs[0])), nil
	default:
		return nil, fmt.Errorf("module %q has multiple installed revisions; pin one in mod.lock", ref)
	}
}

// Revisions returns every installed revision of a module identity
// ("namespace/name"), in directory order. It is empty when none are installed.
// Revisions of one identity coexist because the cache is version-addressed and
// write-once.
func (m *Manager) Revisions(ref string) ([]*ModuleInfo, error) {
	ns, name, ok := strings.Cut(ref, "/")
	if !ok {
		return nil, nil
	}
	dirs, err := m.installedRevDirs(ns, name)
	if err != nil {
		return nil, err
	}
	infos := make([]*ModuleInfo, 0, len(dirs))
	for _, d := range dirs {
		infos = append(infos, m.starlarkInfo(ns, d, filepath.Join(m.rootDir, ns, d)))
	}
	return infos, nil
}

// Resolve selects a single installed revision of ref ("namespace/name"). With an
// empty rev it returns the most recently installed revision; otherwise the
// revision whose id equals rev or is uniquely prefixed by it. It errors when
// nothing matches or a prefix is ambiguous.
func (m *Manager) Resolve(ref, rev string) (*ModuleInfo, error) {
	revs, err := m.Revisions(ref)
	if err != nil {
		return nil, err
	}
	if len(revs) == 0 {
		return nil, fmt.Errorf("module %q not installed", ref)
	}

	if rev == "" {
		return newestRevision(revs), nil
	}

	var matches []*ModuleInfo
	for _, info := range revs {
		if info.Rev == rev || strings.HasPrefix(info.Rev, rev) {
			matches = append(matches, info)
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("module %q has no installed revision %q", ref, rev)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("revision %q of %q is ambiguous; use a longer prefix", rev, ref)
	}
}

// newestRevision returns the revision whose cache directory was modified most
// recently — i.e. the most recently installed.
func newestRevision(revs []*ModuleInfo) *ModuleInfo {
	newest := revs[0]
	newestMod := dirModTime(newest.Path)
	for _, info := range revs[1:] {
		if t := dirModTime(info.Path); t.After(newestMod) {
			newest, newestMod = info, t
		}
	}
	return newest
}

func dirModTime(path string) time.Time {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

// Update fetches the latest revision of an installed module and adds it to the
// cache. ref is "namespace/name". Cached revisions are immutable, so an update
// installs a new revision alongside any existing ones rather than mutating in
// place.
func (m *Manager) Update(ref string) (*ModuleInfo, error) {
	ns, name, ok := strings.Cut(ref, "/")
	if !ok {
		return nil, fmt.Errorf("module %q not installed", ref)
	}
	dirs, err := m.installedRevDirs(ns, name)
	if err != nil {
		return nil, err
	}
	if len(dirs) == 0 {
		return nil, fmt.Errorf("module %q not installed", ref)
	}

	// Recover the original install source from any installed revision's receipt.
	var source string
	for _, d := range dirs {
		if prov, err := ReadProvenance(filepath.Join(m.rootDir, ns, d)); err == nil && prov != nil {
			source = prov.InstalledFrom
			break
		}
	}
	if source == "" {
		return nil, fmt.Errorf("cannot update %q: no install source recorded; reinstall it", ref)
	}

	return m.Install(source, InstallOptions{})
}

// Remove removes an installed module. ref is "namespace/name". With an empty
// rev every cached revision is removed; otherwise only the revision matching
// rev (full id or unique prefix).
func (m *Manager) Remove(ref, rev string) error {
	ns, name, ok := strings.Cut(ref, "/")
	if !ok {
		return fmt.Errorf("module %q not installed", ref)
	}

	if rev != "" {
		info, err := m.Resolve(ref, rev)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(info.Path); err != nil {
			return err
		}
		pruneEmptyDir(filepath.Join(m.rootDir, ns))
		return nil
	}

	dirs, err := m.installedRevDirs(ns, name)
	if err != nil {
		return err
	}
	if len(dirs) == 0 {
		return fmt.Errorf("module %q not installed", ref)
	}
	for _, d := range dirs {
		if err := os.RemoveAll(filepath.Join(m.rootDir, ns, d)); err != nil {
			return err
		}
	}
	// Prune the namespace directory if now empty.
	pruneEmptyDir(filepath.Join(m.rootDir, ns))
	return nil
}

// VerifyResult reports the integrity check for one installed module revision.
type VerifyResult struct {
	Identity string
	Rev      string
	Path     string
	OK       bool
	Reason   string // populated when OK is false
}

// Verify re-hashes installed modules and compares against the content hash
// recorded at install, detecting on-disk tampering or corruption. With an empty
// ref it checks every installed module; with a "namespace/name" it checks every
// revision of that module; with a rev it checks only the matching revision.
// This is the full-content check; the run-time fast path uses the stat
// fingerprint instead.
func (m *Manager) Verify(ref, rev string) ([]VerifyResult, error) {
	var targets []*ModuleInfo
	switch {
	case ref == "":
		all, err := m.List()
		if err != nil {
			return nil, err
		}
		targets = all
	case rev != "":
		info, err := m.Resolve(ref, rev)
		if err != nil {
			return nil, err
		}
		targets = []*ModuleInfo{info}
	default:
		revs, err := m.Revisions(ref)
		if err != nil {
			return nil, err
		}
		if len(revs) == 0 {
			return nil, fmt.Errorf("module %q not installed", ref)
		}
		targets = revs
	}

	results := make([]VerifyResult, 0, len(targets))
	for _, info := range targets {
		identity := info.Name
		if info.Namespace != "" {
			identity = info.Namespace + "/" + info.Name
		}
		res := VerifyResult{Identity: identity, Rev: info.Rev, Path: info.Path, OK: true}

		prov, err := ReadProvenance(info.Path)
		switch {
		case err != nil:
			res.OK, res.Reason = false, fmt.Sprintf("cannot read receipt: %v", err)
		case prov == nil || prov.Hash == "":
			res.OK, res.Reason = false, "no recorded hash; reinstall to record one"
		default:
			if vErr := libkite.VerifyTree(info.Path, prov.Hash); vErr != nil {
				res.OK, res.Reason = false, vErr.Error()
			}
		}
		results = append(results, res)
	}
	return results, nil
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
	for p := range strings.SplitSeq(s, sep) {
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
	if filepath.IsAbs(source) || filepath.VolumeName(source) != "" {
		return true
	}
	// Direct slash or backslash prefix
	if strings.HasPrefix(source, "/") || strings.HasPrefix(source, "\\") {
		return true
	}
	// Relative paths starting with . or ..
	if source == "." || source == ".." ||
		strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, ".\\") || strings.HasPrefix(source, "..\\") {
		return true
	}
	// Home directory expansion
	if strings.HasPrefix(source, "~/") || strings.HasPrefix(source, "~\\") || source == "~" {
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
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
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
