package libkite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

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
				Hint:     SuggestCapabilityHint(module, category, function, resource),
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
		Hint:     SuggestCapabilityHint(module, category, function, resource),
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

// sandboxPathRules are the capabilities granted at Level 1 (sandbox-path):
// workspace filesystem read/write/delete, interactive prompt, environment read,
// and local SQLite databases under $CWD.
var sandboxPathRules = []string{
	"fs.read($CWD/**)",
	"fs.write($CWD/**)",
	"fs.delete($CWD/**)",
	"os.env",
	"io.prompt",
	"sql.open(sqlite:$CWD/**)",
	"sql.open(sqlite::memory:)",
}

// sandboxExecRules are the capabilities added at Level 2 (sandbox-exec):
// process and shell execution.
var sandboxExecRules = []string{
	"os.exec",
	"os.shell",
}

// sandboxNetRules are the capabilities added at Level 3 (sandbox-net):
// outbound/inbound HTTP, SSH, networked SQL drivers, Kubernetes, MCP, and containers.
var sandboxNetRules = []string{
	"http.client",
	"http.server",
	"ssh.connect",
	"ssh.transfer",
	"sql.open(postgres:**)",
	"sql.open(mysql:**)",
	"k8s.read",
	"k8s.write",
	"k8s.config",
	"mcp.client",
	"mcp.server",
	"containers.*",
}

// SandboxPathPermissions returns the Level 1 sandbox-path profile.
func SandboxPathPermissions() *PermissionConfig {
	return &PermissionConfig{Allow: slices.Clone(sandboxPathRules), Default: DefaultDeny}
}

// SandboxExecPermissions returns the Level 2 sandbox-exec profile.
func SandboxExecPermissions() *PermissionConfig {
	return &PermissionConfig{Allow: slices.Concat(sandboxPathRules, sandboxExecRules), Default: DefaultDeny}
}

// SandboxNetPermissions returns the Level 3 sandbox-net profile.
func SandboxNetPermissions() *PermissionConfig {
	return &PermissionConfig{Allow: slices.Concat(sandboxPathRules, sandboxExecRules, sandboxNetRules), Default: DefaultDeny}
}

// AllowFSPermissions returns the sandbox-path profile.
func AllowFSPermissions() *PermissionConfig {
	return SandboxPathPermissions()
}

// AllowLocalPermissions returns the sandbox-exec profile.
func AllowLocalPermissions() *PermissionConfig {
	return SandboxExecPermissions()
}

// AllowNetPermissions returns the sandbox-net profile.
func AllowNetPermissions() *PermissionConfig {
	return SandboxNetPermissions()
}

// PathReadRules generates fs.read and sqlite read rules for the provided paths.
func PathReadRules(paths []string) []string {
	var rules []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "." || p == "./" {
			p = "$CWD"
		} else if after, ok := strings.CutPrefix(p, "./"); ok {
			p = "$CWD/" + after
		}
		if p != "/" {
			p = strings.TrimRight(p, "/")
		}
		if p == "/" {
			rules = append(rules, "fs.read(/**)", "sql.open(sqlite:/**)")
		} else {
			rules = append(rules,
				fmt.Sprintf("fs.read(%s/**)", p),
				fmt.Sprintf("fs.read(%s)", p),
				fmt.Sprintf("sql.open(sqlite:%s/**)", p),
			)
		}
	}
	return rules
}

// PathWriteRules generates fs.read, fs.write, fs.delete, and sqlite rules for the provided paths.
func PathWriteRules(paths []string) []string {
	var rules []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "." || p == "./" {
			p = "$CWD"
		} else if after, ok := strings.CutPrefix(p, "./"); ok {
			p = "$CWD/" + after
		}
		if p != "/" {
			p = strings.TrimRight(p, "/")
		}
		rules = append(rules, PathReadRules([]string{p})...)
		if p == "/" {
			rules = append(rules,
				"fs.write(/**)",
				"fs.delete(/**)",
				"sql.open(sqlite::memory:)",
			)
		} else {
			rules = append(rules,
				fmt.Sprintf("fs.write(%s/**)", p),
				fmt.Sprintf("fs.write(%s)", p),
				fmt.Sprintf("fs.delete(%s/**)", p),
				fmt.Sprintf("fs.delete(%s)", p),
				"sql.open(sqlite::memory:)",
			)
		}
	}
	return rules
}

// ExecAllowRules generates os.exec and os.shell allow rules for the given binaries.
func ExecAllowRules(bins []string) []string {
	var rules []string
	for _, b := range bins {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if b == "*" {
			rules = append(rules, "os.exec", "os.shell")
		} else {
			rules = append(rules, fmt.Sprintf("os.exec(%s)", b))
			if !strings.HasPrefix(b, "/") && !strings.HasPrefix(b, ".") && !strings.HasPrefix(b, "$") {
				rules = append(rules, fmt.Sprintf("os.exec(/**/%s)", b))
			}
		}
	}
	return rules
}

// ExecDenyRules generates os.exec and os.shell deny rules for the given binaries.
func ExecDenyRules(bins []string) []string {
	var rules []string
	for _, b := range bins {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		if b == "*" {
			rules = append(rules, "os.exec", "os.shell")
		} else {
			rules = append(rules, fmt.Sprintf("os.exec(%s)", b))
			if !strings.HasPrefix(b, "/") && !strings.HasPrefix(b, ".") && !strings.HasPrefix(b, "$") {
				rules = append(rules, fmt.Sprintf("os.exec(/**/%s)", b))
			}
		}
	}
	return rules
}

// NetworkAllowRules returns the network capability rules for the given mode and allowed endpoints.
func NetworkAllowRules(mode string, allowEndpoints []string) []string {
	if mode == "none" && len(allowEndpoints) == 0 {
		return nil
	}
	if mode == "loopback" && len(allowEndpoints) == 0 {
		return []string{
			"http.client(http://localhost*)",
			"http.client(http://127.0.0.1*)",
			"http.client(http://[::1]*)",
			"http.server",
		}
	}
	if mode == "host" || len(allowEndpoints) > 0 {
		return slices.Clone(sandboxNetRules)
	}
	return nil
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
		{"sandbox-path", SandboxPathPermissions()},
		{"sandbox-exec", SandboxExecPermissions()},
		{"sandbox-net", SandboxNetPermissions()},
		{"unconfined", AllowAllPermissions()},
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
// messages.
func suggestProfile(module, category, function, resource string) string {
	for _, entry := range getSuggestLadder() {
		if entry.checker.allows(module, category, function, resource) {
			return entry.name
		}
	}
	return "unconfined"
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

// SuggestCapabilityHint returns an actionable remediation hint naming the exact
// CLI flag and config.yaml directive required to authorize a denied capability.
func SuggestCapabilityHint(module, category, function, resource string) string {
	switch module {
	case "fs":
		target := suggestPathForResource(resource)
		if category == "read" {
			return fmt.Sprintf("pass --sandbox-path-read=%s on CLI, or add %q to sandbox.<profile>.paths.read in config.yaml", target, target)
		}
		return fmt.Sprintf("pass --sandbox-path-write=%s on CLI, or add %q to sandbox.<profile>.paths.write in config.yaml", target, target)

	case "sql":
		if rest, ok := strings.CutPrefix(resource, "sqlite:"); ok {
			if rest == ":memory:" || rest == "" {
				return "pass --sandbox-path-write=. on CLI, or add \".\" to sandbox.<profile>.paths.write in config.yaml"
			}
			target := suggestPathForResource(rest)
			return fmt.Sprintf("pass --sandbox-path-write=%s on CLI, or add %q to sandbox.<profile>.paths.write in config.yaml", target, target)
		}
		return "pass --sandbox-net on CLI, or set sandbox.<profile>.network: host in config.yaml"

	case "os":
		if category == "exec" || category == "shell" {
			if category == "shell" || function == "shell" || function == "sh" ||
				function == "bash" || function == "zsh" || function == "cmdexe" ||
				function == "powershell" || resource == "" || resource == "*" {
				return "pass --sandbox-exec-allow=* on CLI, or add \"*\" to sandbox.<profile>.exec.allow in config.yaml"
			}
			bin := suggestExecBinary(resource)
			return fmt.Sprintf("pass --sandbox-exec-allow=%s on CLI, or add %q to sandbox.<profile>.exec.allow in config.yaml", bin, bin)
		}
		callTarget := module + "." + category
		return fmt.Sprintf("pass --sandbox-call-allow=%s on CLI, or add %q to sandbox.<profile>.calls.allow in config.yaml", callTarget, callTarget)

	case "http", "ssh", "k8s", "mcp", "containers", "net":
		return "pass --sandbox-net on CLI, or set sandbox.<profile>.network: host in config.yaml"

	default:
		callTarget := module + "." + category
		if category == "" {
			callTarget = module + ".*"
		}
		return fmt.Sprintf("pass --sandbox-call-allow=%s on CLI, or add %q to sandbox.<profile>.calls.allow in config.yaml", callTarget, callTarget)
	}
}

// suggestPathForResource normalizes a denied file/directory resource into a
// concise CLI path argument ("." when inside $CWD, or the cleaned path outside).
func suggestPathForResource(resource string) string {
	r := strings.TrimSpace(resource)
	if r == "" || r == "." || r == "./" || r == "$CWD" || strings.HasPrefix(r, "$CWD/") {
		return "."
	}
	if !filepath.IsAbs(r) && !strings.HasPrefix(r, "/") && !strings.HasPrefix(r, "$HOME") && !strings.HasPrefix(r, "~") {
		cleanRel := filepath.ToSlash(filepath.Clean(r))
		if cleanRel == "." || !strings.HasPrefix(cleanRel, "..") {
			return "."
		}
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		cleanCwd := filepath.ToSlash(filepath.Clean(cwd))
		canonCwd := filepath.ToSlash(CanonicalizePath(cwd))
		cleanRes := filepath.ToSlash(filepath.Clean(r))
		canonRes := filepath.ToSlash(CanonicalizePath(r))
		if cleanRes == cleanCwd || strings.HasPrefix(cleanRes, cleanCwd+"/") ||
			canonRes == canonCwd || strings.HasPrefix(canonRes, canonCwd+"/") {
			return "."
		}
	}
	return filepath.ToSlash(filepath.Clean(r))
}

// suggestExecBinary extracts a clean binary name from an exec resource string.
func suggestExecBinary(resource string) string {
	r := strings.TrimSpace(resource)
	if r == "" || r == "*" {
		return "*"
	}
	if strings.HasPrefix(r, `"`) {
		if end := strings.Index(r[1:], `"`); end != -1 {
			r = r[1 : end+1]
		}
	} else if strings.ContainsRune(r, filepath.Separator) || strings.ContainsRune(r, '/') {
		if !fileExists(r) {
			lower := strings.ToLower(r)
			for _, ext := range []string{".exe ", ".cmd ", ".bat ", ".ps1 "} {
				if idx := strings.Index(lower, ext); idx != -1 {
					r = r[:idx+len(ext)-1]
					break
				}
			}
			if !fileExists(r) {
				fields := strings.Fields(r)
				for i := len(fields) - 1; i >= 1; i-- {
					cand := strings.Join(fields[:i], " ")
					if fileExists(cand) {
						r = cand
						break
					}
				}
			}
		}
	} else {
		fields := strings.Fields(r)
		if len(fields) > 0 {
			r = fields[0]
		}
	}
	base := filepath.Base(filepath.ToSlash(r))
	if base == "" || base == "." || base == "/" {
		return "*"
	}
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".EXE", ".CMD", ".BAT", ".PS1"} {
		if strings.HasSuffix(base, ext) {
			base = strings.TrimSuffix(base, ext)
			break
		}
	}
	return base
}

// FormatEnvScrubbedDiagnostic formats a debug warning when a script requests an
// environment variable via os.env() that exists on the host but was scrubbed by
// zero-trust environment sanitization.
func FormatEnvScrubbedDiagnostic(varName string) string {
	return fmt.Sprintf(
		"[DEBUG] env %q requested by script is present on host but scrubbed by sandbox.\n  Hint: pass --env %s on CLI, or declare \"%s:\" in sandbox.<profile>.env in config.yaml",
		varName, varName, varName,
	)
}

// WrapOSPermissionError inspects an OS/syscall error and, if it represents a
// kernel-level permission or socket denial (EACCES, EPERM, os.ErrPermission),
// appends an actionable remediation hint for the corresponding module capability.
func WrapOSPermissionError(module, category, resource string, err error) error {
	if err == nil {
		return nil
	}
	if IsPermissionError(err) {
		return err
	}
	isPerm := errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
	if !isPerm {
		lower := strings.ToLower(err.Error())
		isPerm = strings.Contains(lower, "operation not permitted") ||
			strings.Contains(lower, "permission denied")
	}
	if !isPerm {
		return err
	}
	if strings.Contains(err.Error(), "Hint:") {
		return err
	}
	hint := SuggestCapabilityHint(module, category, "", resource)
	return fmt.Errorf("%w\n  Hint: %s", err, hint)
}
