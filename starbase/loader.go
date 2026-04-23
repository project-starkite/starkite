package starbase

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"
)

// loadModule handles load() statements in scripts.
// When loading external modules, the module's public exports are wrapped in a
// starlarkstruct.Module so they can be accessed via module.function().
//
// Example:
//
//	load("./modules/greeter.star", "greeter")  # imports 'greeter' symbol
//	greeter.greet("Alice")                     # call function from module
func (rt *Runtime) loadModule(thread *starlark.Thread, module string) (starlark.StringDict, error) {
	// Check if it's a registered built-in module
	if val, ok := rt.registry.GetLoaded(module); ok {
		return starlark.StringDict{module: val}, nil
	}

	// Get the caller's path from the thread for relative path resolution
	callerPath := thread.Name

	// Try to resolve and load as an external module
	return rt.loadExternalModuleFrom(module, nil, callerPath)
}

// loadExternalModule loads an external .star module with optional configuration.
// Returns exports wrapped in a module struct under the module name.
func (rt *Runtime) loadExternalModule(module string, config *starlark.Dict) (starlark.StringDict, error) {
	return rt.loadExternalModuleFrom(module, config, "")
}

// loadExternalModuleFrom loads an external module with a caller path for relative resolution.
func (rt *Runtime) loadExternalModuleFrom(module string, config *starlark.Dict, callerPath string) (starlark.StringDict, error) {
	// Resolve the module path
	modulePath, err := rt.resolveModulePathFrom(module, callerPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load module %s: %w", module, err)
	}

	// Check cache first
	if cached, ok := rt.moduleCache.Get(modulePath); ok {
		return cached, nil
	}

	// Check for circular dependencies
	if !rt.moduleCache.StartLoading(modulePath) {
		return nil, fmt.Errorf("circular dependency detected loading module %s", module)
	}

	// Load the module (may be single file or multi-file)
	exports, err := rt.loadModuleFromPath(modulePath, config)
	if err != nil {
		rt.moduleCache.StopLoading(modulePath)
		return nil, err
	}

	// Filter out private symbols (those starting with _)
	publicExports := filterPrivateSymbols(exports)

	// Determine the module name from the path
	moduleName := deriveModuleName(module, modulePath)

	// Wrap all exports in a module struct so they can be accessed as module.function()
	// This allows: load("./path.star", "alias") -> alias.func()
	moduleStruct := &starlarkstruct.Module{
		Name:    moduleName,
		Members: publicExports,
	}

	// Return the module struct under its name - this is what gets bound to the
	// identifier in the load() statement
	result := starlark.StringDict{
		moduleName: moduleStruct,
	}

	// Cache the wrapped module
	rt.moduleCache.Set(modulePath, result)

	return result, nil
}

// deriveModuleName extracts a module name from the module path or identifier.
func deriveModuleName(module, modulePath string) string {
	// If module doesn't contain path separators, use it directly
	if !strings.Contains(module, "/") && !strings.HasSuffix(module, ".star") {
		return module
	}

	// Extract from path: /path/to/module.star -> module
	// or /path/to/module/main.star -> module
	base := filepath.Base(modulePath)
	if base == "main.star" {
		// Multi-file module: use directory name
		return filepath.Base(filepath.Dir(modulePath))
	}
	// Directory path — use directory name
	if info, err := os.Stat(modulePath); err == nil && info.IsDir() {
		return base
	}
	// Single file: remove .star extension
	return strings.TrimSuffix(base, ".star")
}

// resolveModulePath resolves a module name to an absolute path.
func (rt *Runtime) resolveModulePath(module string) (string, error) {
	return rt.resolveModulePathFrom(module, "")
}

// resolveModulePathFrom resolves a module name with a caller path for relative resolution.
func (rt *Runtime) resolveModulePathFrom(module, callerPath string) (string, error) {
	// If it's an explicit path (contains "/" or ends with ".star"), resolve directly
	if strings.Contains(module, "/") || strings.HasSuffix(module, ".star") {
		return rt.resolvePathFrom(module, callerPath)
	}

	// Otherwise, search module directories
	searchPaths := rt.getModuleSearchPathsFrom(module, callerPath)

	for _, searchPath := range searchPaths {
		// Single-file module: <name>.star
		singleFile := filepath.Join(searchPath, module+".star")
		if fileExists(singleFile) {
			return singleFile, nil
		}

		// Multi-file module: directory containing main.star or <name>.star
		moduleDir := filepath.Join(searchPath, module)
		if isModuleDir(moduleDir) {
			return moduleDir, nil // return directory — loadModuleFromPath routes via IsDir()
		}
	}

	return "", fmt.Errorf("module %q not found in search paths", module)
}

// resolvePath resolves an explicit path relative to the current script or working directory.
// If callerPath is provided, relative paths are resolved relative to it first.
func (rt *Runtime) resolvePath(path string) (string, error) {
	return rt.resolvePathFrom(path, "")
}

// resolvePathFrom resolves a path relative to a given caller path.
func (rt *Runtime) resolvePathFrom(path, callerPath string) (string, error) {
	if filepath.IsAbs(path) {
		if fileExists(path) || isModuleDir(path) {
			return path, nil
		}
		return "", fmt.Errorf("file not found: %s", path)
	}

	// Try relative to caller (for nested module loads)
	if callerPath != "" {
		absPath := filepath.Join(filepath.Dir(callerPath), path)
		if fileExists(absPath) || isModuleDir(absPath) {
			return absPath, nil
		}
	}

	// Try relative to working directory
	if rt.config.WorkDir != "" {
		absPath := filepath.Join(rt.config.WorkDir, path)
		if fileExists(absPath) || isModuleDir(absPath) {
			return absPath, nil
		}
	}

	// Try relative to current script
	if rt.config.ScriptPath != "" {
		absPath := filepath.Join(filepath.Dir(rt.config.ScriptPath), path)
		if fileExists(absPath) || isModuleDir(absPath) {
			return absPath, nil
		}
	}

	// Try current directory
	absPath, err := filepath.Abs(path)
	if err == nil && (fileExists(absPath) || isModuleDir(absPath)) {
		return absPath, nil
	}

	return "", fmt.Errorf("file not found: %s", path)
}

// getModuleSearchPaths returns the search paths for module resolution.
func (rt *Runtime) getModuleSearchPaths(module string) []string {
	return rt.getModuleSearchPathsFrom(module, "")
}

// getModuleSearchPathsFrom returns search paths with a caller path for nested resolution.
func (rt *Runtime) getModuleSearchPathsFrom(module, callerPath string) []string {
	var paths []string

	// 0. Relative to caller (for nested module loads)
	if callerPath != "" {
		callerDir := filepath.Dir(callerPath)
		paths = append(paths, filepath.Join(callerDir, "modules"))
		paths = append(paths, callerDir)
	}

	// 1. Relative to current script: ./modules/
	if rt.config.ScriptPath != "" {
		scriptDir := filepath.Dir(rt.config.ScriptPath)
		paths = append(paths, filepath.Join(scriptDir, "modules"))
		paths = append(paths, scriptDir)
	}

	// 2. Relative to working directory: ./modules/
	if rt.config.WorkDir != "" {
		paths = append(paths, filepath.Join(rt.config.WorkDir, "modules"))
		paths = append(paths, rt.config.WorkDir)
	}

	// 3. STARKITE_MODULE_PATH entries (colon-separated)
	if modulePath := os.Getenv("STARKITE_MODULE_PATH"); modulePath != "" {
		for _, p := range strings.Split(modulePath, ":") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
	}

	// 4. User modules directory: ~/.starkite/modules/
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".starkite", "modules"))
	}

	return paths
}

// loadModuleFromPath loads a module from a resolved path.
func (rt *Runtime) loadModuleFromPath(modulePath string, config *starlark.Dict) (starlark.StringDict, error) {
	// Check if it's a directory (multi-file module) or single file
	info, err := os.Stat(modulePath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return rt.loadMultiFileModule(modulePath, config)
	}

	return rt.loadSingleFileModule(modulePath, config)
}

// loadSingleFileModule loads a single .star file.
func (rt *Runtime) loadSingleFileModule(filePath string, config *starlark.Dict) (starlark.StringDict, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read module file: %w", err)
	}

	// Build predeclared symbols for the module
	predecl := rt.buildModulePredeclared(config)

	// Create a new thread for the module
	moduleThread := &starlark.Thread{
		Name: filePath,
		Load: rt.loadModule,
		Print: func(thread *starlark.Thread, msg string) {
			fmt.Println(msg)
		},
	}

	// Propagate permissions to module thread
	if rt.permissions != nil {
		SetPermissions(moduleThread, rt.permissions)
	}

	globals, err := starlark.ExecFileOptions(
		&syntax.FileOptions{},
		moduleThread,
		filePath,
		data,
		predecl,
	)
	if err != nil {
		return nil, err
	}

	return globals, nil
}

// loadMultiFileModule loads all .star files in a module directory.
func (rt *Runtime) loadMultiFileModule(dirPath string, config *starlark.Dict) (starlark.StringDict, error) {
	// First, load main.star if it exists
	mainPath := filepath.Join(dirPath, "main.star")
	if !fileExists(mainPath) {
		// Try module name as entry point
		moduleName := filepath.Base(dirPath)
		mainPath = filepath.Join(dirPath, moduleName+".star")
		if !fileExists(mainPath) {
			return nil, fmt.Errorf("no entry point found in module directory %s (expected main.star or %s.star)", dirPath, moduleName)
		}
	}

	// Load the main file
	globals, err := rt.loadSingleFileModule(mainPath, config)
	if err != nil {
		return nil, err
	}

	// Scan for additional .star files and merge their exports
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".star") {
			continue
		}
		fullPath := filepath.Join(dirPath, name)
		if fullPath == mainPath {
			continue // Already loaded
		}

		// Load additional file
		additionalGlobals, err := rt.loadSingleFileModule(fullPath, config)
		if err != nil {
			return nil, fmt.Errorf("error loading %s: %w", fullPath, err)
		}

		// Merge public symbols into globals
		for k, v := range additionalGlobals {
			// Private symbols from additional files are not merged
			if !strings.HasPrefix(k, "_") {
				globals[k] = v
			}
		}
	}

	return globals, nil
}

// buildModulePredeclared builds predeclared symbols for a module.
func (rt *Runtime) buildModulePredeclared(config *starlark.Dict) starlark.StringDict {
	predecl := make(starlark.StringDict, len(rt.predecl)+1)

	// Copy all predeclared symbols
	for k, v := range rt.predecl {
		predecl[k] = v
	}

	// Add _config if provided
	if config != nil {
		predecl["_config"] = config
	} else {
		predecl["_config"] = &starlark.Dict{}
	}

	return predecl
}

// filterPrivateSymbols removes symbols starting with _ from exports.
func filterPrivateSymbols(exports starlark.StringDict) starlark.StringDict {
	result := make(starlark.StringDict, len(exports))
	for k, v := range exports {
		if !strings.HasPrefix(k, "_") {
			result[k] = v
		}
	}
	return result
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// dirExists checks if a directory exists.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isModuleDir checks if a path is a module directory (contains main.star or <name>.star).
func isModuleDir(path string) bool {
	return dirExists(path) &&
		(fileExists(filepath.Join(path, "main.star")) ||
			fileExists(filepath.Join(path, filepath.Base(path)+".star")))
}
