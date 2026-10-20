package libkite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"go.starlark.net/syntax"
)

// StaticLoadClosure holds the pre-resolved static module dependency graph
// discovered by walking AST load() statements starting from an entry script.
type StaticLoadClosure struct {
	// Files maps clean absolute file paths to their raw bytes.
	Files map[string][]byte
	// ModuleDirs maps clean absolute module directory paths to their sorted .star file paths.
	ModuleDirs map[string][]string
	// ReadPaths lists the clean absolute file and directory paths required at runtime
	// so sandbox drivers can mount or unveil them read-only.
	ReadPaths []string
}

// ResolveStaticLoadClosure parses the entry script's AST for load() statements and
// recursively resolves the static module dependency graph across local files,
// local module directories, and installed modules in ~/.starkite/modules.
func ResolveStaticLoadClosure(scriptPath string, workDir string, modulePaths []string) (*StaticLoadClosure, error) {
	if scriptPath == "" {
		return nil, errors.New("script path is required")
	}

	absScript, err := filepath.Abs(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path for %s: %w", scriptPath, err)
	}

	scriptBytes, err := os.ReadFile(absScript)
	if err != nil {
		return nil, fmt.Errorf("reading script %s: %w", absScript, err)
	}

	closure := &StaticLoadClosure{
		Files:      make(map[string][]byte),
		ModuleDirs: make(map[string][]string),
	}

	visitedFiles := make(map[string]bool)
	closure.Files[absScript] = scriptBytes
	closure.ReadPaths = append(closure.ReadPaths, absScript)

	if workDir == "" {
		workDir = filepath.Dir(absScript)
	}
	absWorkDir, _ := filepath.Abs(workDir)

	searchDirs := []string{absWorkDir}
	if scriptDir := filepath.Dir(absScript); scriptDir != absWorkDir {
		searchDirs = append(searchDirs, scriptDir)
	}

	var activeLock *Lock
	for _, dir := range searchDirs {
		for _, manifest := range []string{LockFile, ManifestFile, "star.mod"} {
			p := filepath.Join(dir, manifest)
			if data, err := os.ReadFile(p); err == nil {
				if _, ok := closure.Files[p]; !ok {
					closure.Files[p] = data
					closure.ReadPaths = append(closure.ReadPaths, p)
				}
			}
		}
		if activeLock == nil {
			if l, err := LoadLock(dir); err == nil {
				activeLock = l
			}
		}
	}

	var walkFile func(filePath string) error
	var walkModuleDir func(dirPath string) error

	walkFile = func(filePath string) error {
		clean := filepath.Clean(filePath)
		if visitedFiles[clean] {
			return nil
		}
		visitedFiles[clean] = true

		data := closure.Files[clean]
		if data == nil {
			var err error
			data, err = os.ReadFile(clean)
			if err != nil {
				return nil
			}
			closure.Files[clean] = data
			closure.ReadPaths = append(closure.ReadPaths, clean)
		}

		targets, err := extractLoadTargets(clean, data)
		if err != nil {
			return nil
		}

		callerDir := filepath.Dir(clean)
		for _, target := range targets {
			if strings.HasPrefix(target, "starkite/") {
				continue
			}

			// Path-based reference (./, ../, /, or .star)
			if isPathRef(target) || strings.HasSuffix(target, ".star") {
				candidates := []string{
					filepath.Join(callerDir, target),
					filepath.Join(absWorkDir, target),
				}
				for _, mp := range modulePaths {
					candidates = append(candidates, filepath.Join(mp, target))
				}

				for _, cand := range candidates {
					cleanCand := filepath.Clean(cand)
					if fileExists(cleanCand) {
						_ = walkFile(cleanCand)
						break
					} else if isModuleDir(cleanCand) {
						_ = walkModuleDir(cleanCand)
						break
					}
				}
				continue
			}

			// Named module (e.g. namespace/name or bare name)
			foundLocal := false
			for _, baseDir := range append([]string{callerDir, absWorkDir}, modulePaths...) {
				candDir := filepath.Join(baseDir, target)
				if isModuleDir(candDir) {
					_ = walkModuleDir(candDir)
					foundLocal = true
					break
				}
			}
			if foundLocal {
				continue
			}

			if strings.Contains(target, "/") {
				if home, err := os.UserHomeDir(); err == nil && home != "" {
					ns, name, ok := strings.Cut(target, "/")
					if ok && isSafeModuleSegment(ns) && isSafeModuleSegment(name) {
						nsDir := filepath.Join(home, ".starkite", "modules", ns)
						if activeLock != nil {
							if locked, ok := activeLock.Modules[target]; ok && locked.Rev != "" {
								lockedDir := filepath.Join(nsDir, name+"@"+locked.Rev)
								if isModuleDir(lockedDir) {
									_ = walkModuleDir(lockedDir)
									continue
								}
							}
						}

						if entries, err := os.ReadDir(nsDir); err == nil {
							for _, e := range entries {
								if e.IsDir() {
									if n, _ := SplitModuleRev(e.Name()); n == name {
										dir := filepath.Join(nsDir, e.Name())
										if isModuleDir(dir) {
											_ = walkModuleDir(dir)
											break
										}
									}
								}
							}
						}
					}
				}
			}
		}

		return nil
	}

	walkModuleDir = func(dirPath string) error {
		absDir, _ := filepath.Abs(dirPath)
		cleanDir := filepath.Clean(absDir)
		if _, ok := closure.ModuleDirs[cleanDir]; ok {
			return nil
		}

		manifestPath := filepath.Join(cleanDir, ManifestFile)
		if data, err := os.ReadFile(manifestPath); err == nil {
			closure.Files[manifestPath] = data
			closure.ReadPaths = append(closure.ReadPaths, manifestPath)
		} else {
			return nil
		}

		entries, err := os.ReadDir(cleanDir)
		if err != nil {
			return nil
		}

		var starFiles []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".star") {
				starPath := filepath.Join(cleanDir, e.Name())
				starFiles = append(starFiles, starPath)
				_ = walkFile(starPath)
			}
		}
		sort.Strings(starFiles)
		closure.ModuleDirs[cleanDir] = starFiles
		closure.ReadPaths = append(closure.ReadPaths, cleanDir)
		return nil
	}

	if filepath.Base(absScript) == EntryFile && isModuleDir(filepath.Dir(absScript)) {
		if err := walkModuleDir(filepath.Dir(absScript)); err != nil {
			return nil, err
		}
	} else if err := walkFile(absScript); err != nil {
		return nil, err
	}

	seenPaths := make(map[string]bool)
	var dedupedPaths []string
	for _, p := range closure.ReadPaths {
		clean := filepath.Clean(p)
		if !seenPaths[clean] {
			seenPaths[clean] = true
			dedupedPaths = append(dedupedPaths, clean)
		}
	}
	closure.ReadPaths = dedupedPaths

	return closure, nil
}

func extractLoadTargets(filePath string, src []byte) ([]string, error) {
	opts := syntax.FileOptions{Set: true, While: true, TopLevelControl: true, GlobalReassign: true}
	f, err := opts.Parse(filePath, src, 0)
	if err != nil {
		return nil, err
	}

	var targets []string
	syntax.Walk(f, func(n syntax.Node) bool {
		if ls, ok := n.(*syntax.LoadStmt); ok && ls.Module != nil {
			if s, ok := ls.Module.Value.(string); ok {
				targets = append(targets, s)
			}
		}
		return true
	})
	return targets, nil
}

func (rt *Runtime) readSourceFile(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	if rt != nil && rt.preloaded != nil {
		if data, ok := rt.preloaded.Files[clean]; ok {
			return data, nil
		}
		if abs, err := filepath.Abs(clean); err == nil {
			if data, ok := rt.preloaded.Files[abs]; ok {
				return data, nil
			}
		}
	}
	return os.ReadFile(clean)
}

func (rt *Runtime) sourceFileExists(path string) bool {
	clean := filepath.Clean(path)
	if rt != nil && rt.preloaded != nil {
		if _, ok := rt.preloaded.Files[clean]; ok {
			return true
		}
		if abs, err := filepath.Abs(clean); err == nil {
			if _, ok := rt.preloaded.Files[abs]; ok {
				return true
			}
		}
	}
	return fileExists(clean)
}

func (rt *Runtime) isSourceModuleDir(path string) bool {
	clean := filepath.Clean(path)
	if rt != nil && rt.preloaded != nil {
		if _, ok := rt.preloaded.ModuleDirs[clean]; ok {
			return true
		}
		if abs, err := filepath.Abs(clean); err == nil {
			if _, ok := rt.preloaded.ModuleDirs[abs]; ok {
				return true
			}
		}
	}
	return isModuleDir(clean)
}

func (rt *Runtime) listModuleStarFiles(dirPath string) ([]string, error) {
	clean := filepath.Clean(dirPath)
	if rt != nil && rt.preloaded != nil {
		if files, ok := rt.preloaded.ModuleDirs[clean]; ok {
			return slices.Clone(files), nil
		}
		if abs, err := filepath.Abs(clean); err == nil {
			if files, ok := rt.preloaded.ModuleDirs[abs]; ok {
				return slices.Clone(files), nil
			}
		}
	}
	entries, err := os.ReadDir(clean)
	if err != nil {
		return nil, err
	}
	var starFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".star") {
			starFiles = append(starFiles, filepath.Join(clean, e.Name()))
		}
	}
	sort.Strings(starFiles)
	return starFiles, nil
}
