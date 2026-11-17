package sandbox

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// ResolveExecutablePaths resolves a list of allowed executable patterns or names
// into clean absolute host paths, traversing symlink chains so that kernel sandbox
// drivers can grant read and execute access to both the symlink hop paths and the
// final canonical binary target.
func ResolveExecutablePaths(allowList []string, cwd, home string) []string {
	var resolved []string
	seen := make(map[string]bool)

	addPath := func(p string) {
		clean := filepath.Clean(p)
		if clean == "" {
			return
		}
		key := clean
		if runtime.GOOS == "windows" {
			key = strings.ToLower(clean)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		resolved = append(resolved, clean)
	}

	var winExts []string
	if runtime.GOOS == "windows" {
		pathext := os.Getenv("PATHEXT")
		if pathext == "" {
			pathext = ".COM;.EXE;.BAT;.CMD;.PS1"
		}
		winExts = filepath.SplitList(pathext)
	}

	for _, entry := range allowList {
		for part := range strings.SplitSeq(entry, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" || trimmed == "*" {
				continue
			}

			if strings.ContainsRune(trimmed, filepath.Separator) || strings.Contains(trimmed, "/") {
				exp := expandPath(trimmed, cwd, home)
				var startPath string
				if isHostAbs(exp) {
					startPath = exp
				} else if cwd != "" {
					startPath = filepath.Join(cwd, exp)
				} else {
					startPath = exp
				}

				if _, err := os.Lstat(startPath); err == nil {
					for _, hop := range resolveSymlinkChain(startPath) {
						addPath(hop)
					}
				} else {
					addPath(startPath)
				}
				continue
			}

			// Bare binary name (e.g. "kubectl", "helm")
			var foundPath string
			if path, err := osexec.LookPath(trimmed); err == nil {
				foundPath = path
			} else {
				candidates := []string{trimmed}
				if runtime.GOOS == "windows" && filepath.Ext(trimmed) == "" {
					for _, ext := range winExts {
						candidates = append(candidates, trimmed+ext)
					}
				}
				for _, dir := range defaultExecSearchDirs {
					for _, name := range candidates {
						cand := filepath.Join(dir, name)
						if fi, err := os.Lstat(cand); err == nil && !fi.IsDir() {
							foundPath = cand
							break
						}
					}
					if foundPath != "" {
						break
					}
				}
			}

			if foundPath != "" {
				for _, hop := range resolveSymlinkChain(foundPath) {
					addPath(hop)
				}
			}
		}
	}

	return resolved
}

// resolveSymlinkChain returns the start path, every intermediate symlink target,
// and the final canonical target discovered via filepath.EvalSymlinks.
func resolveSymlinkChain(startPath string) []string {
	cleanStart := filepath.Clean(startPath)
	var chain []string
	seen := make(map[string]bool)

	cur := cleanStart
	for range 16 {
		if cur == "" || seen[cur] {
			break
		}
		seen[cur] = true
		chain = append(chain, cur)

		fi, err := os.Lstat(cur)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}

		linkTarget, err := os.Readlink(cur)
		if err != nil {
			break
		}

		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(cur), linkTarget)
		}
		cur = filepath.Clean(linkTarget)
	}

	if canonical, err := filepath.EvalSymlinks(cleanStart); err == nil {
		cleanCanon := filepath.Clean(canonical)
		if cleanCanon != "" && !slices.Contains(chain, cleanCanon) {
			chain = append(chain, cleanCanon)
		}
	}

	return chain
}

// normalizeExecEntryPath resolves symlinks along the parent directory while
// preserving the base filename so directory symlinks match without collapsing
// distinct multicall binary symlinks onto a shared target inode.
func normalizeExecEntryPath(p string) string {
	clean := filepath.Clean(p)
	if clean == "" || clean == "." || clean == string(filepath.Separator) {
		return clean
	}
	dir := filepath.Dir(clean)
	if resolvedDir, err := filepath.EvalSymlinks(dir); err == nil && resolvedDir != "" {
		return filepath.Join(resolvedDir, filepath.Base(clean))
	}
	return clean
}

func equalOrHasExecPrefix(target, allowed string) bool {
	if target == "" || allowed == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return equalOrHasPathPrefixFold(target, allowed)
	}
	if target == allowed {
		return true
	}
	prefix := allowed
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(target, prefix)
}

// isAllowedExecTarget reports whether targetBin matches any entry in
// allowedPaths directly or after normalizing parent directory symlinks.
func isAllowedExecTarget(targetBin, cwd string, allowedPaths []string) bool {
	resolved := targetBin
	if !isHostAbs(resolved) && cwd != "" && (strings.ContainsRune(resolved, filepath.Separator) || strings.Contains(resolved, "/") || strings.HasPrefix(resolved, ".")) {
		resolved = filepath.Join(cwd, resolved)
	}
	cleanBin := filepath.Clean(resolved)
	normBin := normalizeExecEntryPath(cleanBin)

	for _, p := range allowedPaths {
		cleanP := filepath.Clean(p)
		if equalOrHasExecPrefix(cleanBin, cleanP) {
			return true
		}
		normP := normalizeExecEntryPath(cleanP)
		if equalOrHasExecPrefix(normBin, normP) {
			return true
		}
	}
	return false
}
