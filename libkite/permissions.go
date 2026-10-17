package libkite

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"go.starlark.net/starlark"
)

// permissionKey is the thread.Local key for the permission checker.
const permissionKey = "libkite.permissions"

// moduleOriginKey is the thread.Local key holding the label of the loaded
// module whose code the thread runs, for denial attribution.
const moduleOriginKey = "libkite.moduleOrigin"

// PermissionDefault specifies default behavior when no rule matches.
type PermissionDefault int

const (
	// DefaultDeny denies operations when no rule matches (secure).
	DefaultDeny PermissionDefault = iota
	// DefaultAllow allows operations when no rule matches (permissive).
	DefaultAllow
)

// PermissionConfig defines the permission policy for a runtime.
type PermissionConfig struct {
	// Allow rules - patterns that grant access
	Allow []string

	// Deny rules - patterns that block access (evaluated first)
	Deny []string

	// Default behavior when no rules match
	Default PermissionDefault
}

// PermissionChecker validates operations against allow/deny rules.
type PermissionChecker struct {
	allow    []Rule
	deny     []Rule
	default_ PermissionDefault
	mu       sync.RWMutex
}

// NewPermissionChecker creates a checker from the given config. Path
// variables ($CWD, $HOME) in resource patterns are expanded once here using
// the process's current working directory and the user's home directory.
func NewPermissionChecker(config *PermissionConfig) (*PermissionChecker, error) {
	if config == nil {
		return nil, nil
	}

	cwd, _ := os.Getwd()
	if cwd != "" {
		cwd = CanonicalizePath(cwd)
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		home = CanonicalizePath(home)
	}

	checker := &PermissionChecker{
		default_: config.Default,
	}

	for _, pattern := range config.Allow {
		rule, err := ParseRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid allow rule %q: %w", pattern, err)
		}
		rule.setResource(canonicalizePattern(expandPathVariables(rule.Resource, cwd, home)))
		checker.allow = append(checker.allow, *rule)
	}

	for _, pattern := range config.Deny {
		rule, err := ParseRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid deny rule %q: %w", pattern, err)
		}
		rule.setResource(canonicalizePattern(expandPathVariables(rule.Resource, cwd, home)))
		checker.deny = append(checker.deny, *rule)
	}

	return checker, nil
}

// Check validates if module.category.function(resource) is permitted.
func (c *PermissionChecker) Check(module, category, function, resource string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.deny {
		if rule.Matches(module, category, function, resource) {
			return &PermissionError{
				Module:   module,
				Category: category,
				Function: function,
				Resource: resource,
				Reason:   fmt.Sprintf("blocked by deny rule: %s", rule.Raw),
			}
		}
	}

	for _, rule := range c.allow {
		if rule.Matches(module, category, function, resource) {
			return nil
		}
	}

	if c.default_ == DefaultAllow {
		return nil
	}

	return &PermissionError{
		Module:   module,
		Category: category,
		Function: function,
		Resource: resource,
		Reason:   "no matching allow rule",
		Allowed:  c.allowedPatterns(),
		Suggest:  suggestProfile(module, category, function, resource),
	}
}

// allows reports whether the operation is permitted, without constructing a
// PermissionError. It is used to compute opt-up suggestions against the
// built-in ladder profiles, avoiding the recursion that calling Check would
// trigger (Check builds the suggestion, which would call Check again).
func (c *PermissionChecker) allows(module, category, function, resource string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, rule := range c.deny {
		if rule.Matches(module, category, function, resource) {
			return false
		}
	}
	for _, rule := range c.allow {
		if rule.Matches(module, category, function, resource) {
			return true
		}
	}
	return c.default_ == DefaultAllow
}

// allowedPatterns returns the raw allow patterns for error messages.
func (c *PermissionChecker) allowedPatterns() []string {
	patterns := make([]string, len(c.allow))
	for i, rule := range c.allow {
		patterns[i] = rule.Raw
	}
	return patterns
}

// Rule represents a parsed permission pattern.
type Rule struct {
	Module    string   // Module name or "*"
	Category  string   // Category name or "*"
	Functions []string // Optional function-name filter; nil = any function in category
	Resource  string   // Resource glob pattern or ""
	Raw       string   // Original pattern string

	// Pre-computed fields for fast resource matching when Resource has not been mutated after setResource.
	compiledSrc   string
	slashPattern  string
	isAbsPattern  bool
	hasDoublestar bool
	patSegs       []string
}

func (r *Rule) setResource(res string) {
	r.Resource = res
	if res == "" {
		r.compiledSrc = ""
		r.slashPattern = ""
		r.isAbsPattern = false
		r.hasDoublestar = false
		r.patSegs = nil
		return
	}
	p := filepath.ToSlash(res)
	isAbs := filepath.IsAbs(p)
	if isAbs {
		p = canonicalizePattern(p)
	}
	hasDS := strings.Contains(p, "**")
	var segs []string
	if hasDS && p != "**" {
		segs = strings.Split(p, "/")
	}
	r.compiledSrc = res
	r.slashPattern = p
	r.isAbsPattern = isAbs
	r.hasDoublestar = hasDS
	r.patSegs = segs
}

// ParseRule parses a permission pattern string.
//
// Pattern grammar:
//
//	rule     := module "." category [ "(" [ funclist ":" ] resource ")" ]
//	funclist := func_name ("," func_name)*    bare names, no wildcards
//
// Examples:
//
//	"module.*"                                → any category, any function, any resource
//	"fs.read"                                 → category, any function, any resource
//	"fs.read(/etc/**)"                        → category, any function, scoped resource
//	"fs.read(read_file:*)"                    → single function, any resource
//	"fs.read(read_file,read_bytes:/etc/**)"   → multi-function, scoped resource
//	"*.*"                                     → catch-all
//
// Disambiguation: contents before the first ":" are a function list only if
// they match `ident(,ident)*`; otherwise the parenthesised content is a
// resource literal.
func ParseRule(pattern string) (*Rule, error) {
	rule := &Rule{Raw: pattern}

	if idx := strings.Index(pattern, "("); idx != -1 {
		if !strings.HasSuffix(pattern, ")") {
			return nil, fmt.Errorf("unclosed parenthesis in pattern")
		}
		inner := pattern[idx+1 : len(pattern)-1]
		pattern = pattern[:idx]

		funcs, resource := splitFuncsAndResource(inner)
		rule.Functions = funcs
		rule.setResource(resource)
	}

	parts := strings.SplitN(pattern, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("pattern must be module.category, got %q", pattern)
	}

	rule.Module = parts[0]
	rule.Category = parts[1]

	if rule.Module == "" || rule.Category == "" {
		return nil, fmt.Errorf("module and category cannot be empty")
	}

	return rule, nil
}

// splitFuncsAndResource splits paren contents into an optional function list
// and a resource. The colon is the separator; function lists must consist of
// bare identifiers separated by commas. Returns (nil, inner) when the contents
// don't contain a valid function-list prefix.
func splitFuncsAndResource(inner string) ([]string, string) {
	before, after, ok := strings.Cut(inner, ":")
	if !ok {
		return nil, inner
	}
	if len(before) == 1 && len(after) > 0 && (after[0] == '/' || after[0] == '\\') {
		return nil, inner
	}
	prefix := before
	if !isFuncList(prefix) {
		return nil, inner
	}
	return strings.Split(prefix, ","), after
}

// isFuncList reports whether s matches `ident(,ident)*` with non-empty idents.
func isFuncList(s string) bool {
	if s == "" {
		return false
	}
	for name := range strings.SplitSeq(s, ",") {
		if !isIdent(name) {
			return false
		}
	}
	return true
}

// isIdent reports whether s is a non-empty identifier of letters, digits, and
// underscore (no leading digit).
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// Matches checks if the rule matches the given operation.
func (r *Rule) Matches(module, category, function, resource string) bool {
	if r.Module != "*" && r.Module != module {
		return false
	}

	if r.Category != "*" && r.Category != category {
		return false
	}

	if r.Functions != nil {
		found := slices.Contains(r.Functions, function)
		if !found {
			return false
		}
	}

	if r.Resource != "" {
		if resource == "" {
			return false
		}
		var pattern string
		var isAbs, hasDS bool
		var patSegs []string
		if r.compiledSrc == r.Resource && r.slashPattern != "" {
			pattern = r.slashPattern
			isAbs = r.isAbsPattern
			hasDS = r.hasDoublestar
			patSegs = r.patSegs
		} else {
			pattern = filepath.ToSlash(r.Resource)
			isAbs = filepath.IsAbs(pattern)
			if isAbs {
				pattern = canonicalizePattern(pattern)
			}
			hasDS = strings.Contains(pattern, "**")
		}

		// Bare "*" or "**" matches any non-empty resource (filepath.Match
		// treats "*" as not crossing path separators, which surprises users).
		if pattern == "*" || pattern == "**" {
			return true
		}
		res := filepath.ToSlash(resource)
		if isAbs {
			res = filepath.ToSlash(CanonicalizePath(resource))
		}
		matched, err := filepath.Match(pattern, res)
		if err != nil {
			if before, ok := strings.CutSuffix(pattern, "**"); ok {
				return strings.HasPrefix(res, before)
			}
			return false
		}

		// Handle ** patterns (match any directory depth)
		if !matched && hasDS {
			if patSegs != nil {
				matched = matchDoublestarPrecompiled(pattern, patSegs, res)
			} else {
				matched = matchDoublestar(pattern, res)
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

// CanonicalizePath resolves a path to an absolute, cleaned physical path by
// evaluating symlinks on the longest existing ancestor prefix. This prevents
// lexical ".." traversal and symlink escapes out of $CWD- or directory-scoped
// permission rules, even when the leaf target does not yet exist on disk.
func CanonicalizePath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	return evalSymlinksLongestPrefix(abs)
}

func evalSymlinksLongestPrefix(absPath string) string {
	curr := filepath.Clean(absPath)
	var tail []string
	for {
		if resolved, err := filepath.EvalSymlinks(curr); err == nil {
			for _, t := range slices.Backward(tail) {
				resolved = filepath.Join(resolved, t)
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			return filepath.Clean(absPath)
		}
		tail = append(tail, filepath.Base(curr))
		curr = parent
	}
}

func canonicalizePattern(pattern string) string {
	if !filepath.IsAbs(pattern) {
		return pattern
	}
	slashed := filepath.ToSlash(pattern)
	idx := strings.IndexAny(slashed, "*?[")
	if idx == -1 {
		return filepath.ToSlash(CanonicalizePath(pattern))
	}
	slashIdx := strings.LastIndex(slashed[:idx], "/")
	if slashIdx <= 0 {
		return slashed
	}
	prefix := CanonicalizePath(slashed[:slashIdx])
	return filepath.ToSlash(prefix) + slashed[slashIdx:]
}

// expandPathVariables substitutes $CWD and $HOME in a resource pattern. An
// empty replacement leaves the variable in place (so a misconfigured rule
// fails closed rather than matching the empty path).
func expandPathVariables(pattern, cwd, home string) string {
	if cwd != "" {
		pattern = strings.ReplaceAll(pattern, "$CWD", filepath.ToSlash(cwd))
	}
	if home != "" {
		pattern = strings.ReplaceAll(pattern, "$HOME", filepath.ToSlash(home))
	}
	return pattern
}

// matchDoublestar handles ** glob patterns that match any directory depth.
func matchDoublestar(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	if pattern == "**" {
		return true
	}

	// Fast path for non-slash prefix wildcard (e.g. "sqlite:**", "postgres:**").
	if before, ok := strings.CutSuffix(pattern, "**"); ok && !strings.HasSuffix(before, "/") && !strings.ContainsAny(before, "*?[") {
		return strings.HasPrefix(path, before)
	}

	patSegs := strings.Split(pattern, "/")
	pathSegs := strings.Split(path, "/")
	return matchSegments(patSegs, pathSegs)
}

func matchDoublestarPrecompiled(pattern string, patSegs []string, path string) bool {
	if before, ok := strings.CutSuffix(pattern, "**"); ok && !strings.HasSuffix(before, "/") && !strings.ContainsAny(before, "*?[") {
		return strings.HasPrefix(path, before)
	}
	pathSegs := strings.Split(path, "/")
	return matchSegments(patSegs, pathSegs)
}

func matchSegments(patSegs, pathSegs []string) bool {
	for len(patSegs) > 0 {
		if patSegs[0] == "**" {
			for len(patSegs) > 1 && patSegs[1] == "**" {
				patSegs = patSegs[1:]
			}
			if len(patSegs) == 1 {
				return true
			}
			for i := 0; i <= len(pathSegs); i++ {
				if matchSegments(patSegs[1:], pathSegs[i:]) {
					return true
				}
			}
			return false
		}
		if len(pathSegs) == 0 {
			return false
		}
		segPat := strings.ReplaceAll(patSegs[0], "**", "*")
		matched, err := filepath.Match(segPat, pathSegs[0])
		if err != nil || !matched {
			return false
		}
		patSegs = patSegs[1:]
		pathSegs = pathSegs[1:]
	}
	return len(pathSegs) == 0
}

// Check validates module.category.function(resource) against the permission
// rules stored in the thread's local storage. Returns nil if no checker is
// set (trusted mode).
//
// This is the main entry point for modules to check permissions.
func Check(thread *starlark.Thread, module, category, function, resource string) error {
	if thread == nil {
		return nil
	}

	checker, _ := thread.Local(permissionKey).(*PermissionChecker)
	if checker == nil {
		return nil
	}

	err := checker.Check(module, category, function, resource)
	// Attribute a denial to the loaded module whose code is running, if any.
	if pe, ok := err.(*PermissionError); ok {
		pe.Origin = attributeModule(thread)
	}
	return err
}

// attributeModule identifies the loaded module responsible for the current
// call, for denial attribution. A module's exported function executes on the
// caller's thread, so the call stack is the reliable signal: the innermost
// frame whose source file differs from the entry script belongs to a loaded
// module. The module-origin thread local is the fallback for denials raised
// while a module's top-level code runs (during load). Returns "" for calls
// made directly by the entry script.
func attributeModule(thread *starlark.Thread) string {
	if stack := thread.CallStack(); len(stack) > 0 {
		entryFile := stack[0].Pos.Filename()
		for _, s := range slices.Backward(stack) {
			f := s.Pos.Filename()
			// Skip non-file frames (builtins are "<builtin>") and the entry script.
			if !strings.HasSuffix(f, ".star") || f == entryFile {
				continue
			}
			return moduleLabelFromFile(f)
		}
	}
	origin, _ := thread.Local(moduleOriginKey).(string)
	return origin
}

// moduleLabelFromFile derives a module label from a source file path: the
// directory name for a multi-file module's main.star, otherwise the file's base
// name without the .star suffix.
func moduleLabelFromFile(path string) string {
	base := filepath.Base(path)
	if base == EntryFile {
		return filepath.Base(filepath.Dir(path))
	}
	return strings.TrimSuffix(base, ".star")
}

// SetPermissions stores the permission checker in thread.Local.
// Called by Runtime before executing each script.
func SetPermissions(thread *starlark.Thread, checker *PermissionChecker) {
	if thread != nil && checker != nil {
		thread.SetLocal(permissionKey, checker)
	}
}

// SetModuleOrigin labels a thread with the loaded module whose code it runs, so
// a denial raised on that thread can be attributed to the module. The entry
// script's thread carries no origin.
func SetModuleOrigin(thread *starlark.Thread, origin string) {
	if thread != nil && origin != "" {
		thread.SetLocal(moduleOriginKey, origin)
	}
}

// GetPermissions retrieves the permission checker from thread.Local.
func GetPermissions(thread *starlark.Thread) *PermissionChecker {
	if thread == nil {
		return nil
	}
	checker, _ := thread.Local(permissionKey).(*PermissionChecker)
	return checker
}

// AllowAllPermissions returns a config that grants every gated operation.
func AllowAllPermissions() *PermissionConfig {
	return &PermissionConfig{
		Allow:   []string{"*.*"},
		Default: DefaultAllow,
	}
}

// DenyAllPermissions returns a config that denies every gated operation.
// Pure utility modules (strings, json, yaml, …) bypass the permission system,
// so they remain available; any module that calls Check (fs, os, http, ssh,
// k8s, mcp, io) is blocked.
func DenyAllPermissions() *PermissionConfig {
	return &PermissionConfig{
		Default: DefaultDeny,
	}
}

// The five built-in profiles form a strict capability ladder; each is a
// superset of the one below. All use DefaultDeny (allow-list semantics) — a
// capability is granted only if it appears in the allow set.

// allow-fs: read any file; create/modify/delete only within the working tree
// ($CWD); environment and interactive prompts. No network or exec.
var allowFSRules = []string{
	"fs.read",
	"fs.write($CWD/**)",
	"fs.delete($CWD/**)",
	"os.env",
	"io.prompt",
	"sql.open(sqlite:**)", // SQLite is file I/O
}

// allow-net: allow-fs plus low-level protocol networking.
var allowNetRules = append(append([]string{}, allowFSRules...),
	"http.client",
	"ssh.connect", "ssh.transfer",
	"sql.open(postgres:**)", "sql.open(mysql:**)", // networked databases
)

// allow-local: allow-net plus serve, $CWD-scoped exec, and higher-level
// networked services (k8s, mcp). Withholds unrestricted exec, k8s.exec,
// and process control — that is the line to allow-all.
var allowLocalRules = append(append([]string{}, allowNetRules...),
	"http.server",
	"os.exec($CWD/**)",
	"k8s.read", "k8s.write", "k8s.config",
	"mcp.client", "mcp.server",
	"containers.connect", "containers.read", "containers.write", "containers.manage",
)

// AllowFSPermissions returns the allow-fs profile.
func AllowFSPermissions() *PermissionConfig {
	return &PermissionConfig{Allow: append([]string{}, allowFSRules...), Default: DefaultDeny}
}

// AllowNetPermissions returns the allow-net profile.
func AllowNetPermissions() *PermissionConfig {
	return &PermissionConfig{Allow: append([]string{}, allowNetRules...), Default: DefaultDeny}
}

// AllowLocalPermissions returns the allow-local profile.
func AllowLocalPermissions() *PermissionConfig {
	return &PermissionConfig{Allow: append([]string{}, allowLocalRules...), Default: DefaultDeny}
}

type ladderEntry struct {
	name    string
	checker *PermissionChecker
}

var (
	suggestCacheMu      sync.RWMutex
	suggestCacheCwd     string
	suggestCacheEntries []ladderEntry
)

func getSuggestLadder() []ladderEntry {
	cwd, _ := os.Getwd()

	suggestCacheMu.RLock()
	if len(suggestCacheEntries) > 0 && suggestCacheCwd == cwd {
		entries := suggestCacheEntries
		suggestCacheMu.RUnlock()
		return entries
	}
	suggestCacheMu.RUnlock()

	profiles := []struct {
		name string
		cfg  *PermissionConfig
	}{
		{"allow-fs", AllowFSPermissions()},
		{"allow-net", AllowNetPermissions()},
		{"allow-local", AllowLocalPermissions()},
		{"allow-all", AllowAllPermissions()},
	}

	built := make([]ladderEntry, 0, len(profiles))
	for _, p := range profiles {
		if c, err := NewPermissionChecker(p.cfg); err == nil {
			built = append(built, ladderEntry{name: p.name, checker: c})
		}
	}

	suggestCacheMu.Lock()
	suggestCacheCwd = cwd
	suggestCacheEntries = built
	suggestCacheMu.Unlock()

	return built
}

// suggestProfile returns the lowest built-in profile on the capability ladder
// that would actually grant this specific call, for prescriptive denial
// messages. It tests the real profiles (resource-scoped rules included), so
// e.g. exec of a $CWD binary suggests allow-local while exec of a system
// binary suggests allow-all. The names match the built-in profile constants in
// libkite/permissions.
func suggestProfile(module, category, function, resource string) string {
	for _, entry := range getSuggestLadder() {
		if entry.checker.allows(module, category, function, resource) {
			return entry.name
		}
	}
	return "allow-all"
}

// AllowPermissions creates a permissive config with specific denials.
// This is easier than listing everything you want to allow.
func AllowPermissions(deny ...string) *PermissionConfig {
	return &PermissionConfig{
		Allow:   []string{"*.*"},
		Deny:    deny,
		Default: DefaultAllow,
	}
}
