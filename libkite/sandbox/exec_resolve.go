package sandbox

import (
	"os"
	osexec "os/exec"
	"path/filepath"
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
		if clean == "" || seen[clean] {
			return
		}
		seen[clean] = true
		resolved = append(resolved, clean)
	}

	for _, entry := range allowList {
		for _, part := range strings.Split(entry, ",") {
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
				for _, dir := range defaultExecSearchDirs {
					cand := filepath.Join(dir, trimmed)
					if fi, err := os.Lstat(cand); err == nil && !fi.IsDir() {
						foundPath = cand
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
	for i := 0; i < 16; i++ {
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
