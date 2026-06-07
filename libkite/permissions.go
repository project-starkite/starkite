package libkite

import (
	"fmt"
	"os"
	"path/filepath"
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
	home, _ := os.UserHomeDir()

	checker := &PermissionChecker{
		default_: config.Default,
	}

	for _, pattern := range config.Allow {
		rule, err := ParseRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid allow rule %q: %w", pattern, err)
		}
		rule.Resource = expandPathVariables(rule.Resource, cwd, home)
		checker.allow = append(checker.allow, *rule)
	}

	for _, pattern := range config.Deny {
		rule, err := ParseRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid deny rule %q: %w", pattern, err)
		}
		rule.Resource = expandPathVariables(rule.Resource, cwd, home)
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
		rule.Resource = resource
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
	colon := strings.Index(inner, ":")
	if colon < 0 {
		return nil, inner
	}
	prefix := inner[:colon]
	if !isFuncList(prefix) {
		return nil, inner
	}
	return strings.Split(prefix, ","), inner[colon+1:]
}

// isFuncList reports whether s matches `ident(,ident)*` with non-empty idents.
func isFuncList(s string) bool {
	if s == "" {
		return false
	}
	for _, name := range strings.Split(s, ",") {
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
		found := false
		for _, fn := range r.Functions {
			if fn == function {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if r.Resource != "" && resource != "" {
		pattern := r.Resource
		// Bare "*" or "**" matches any non-empty resource (filepath.Match
		// treats "*" as not crossing path separators, which surprises users).
		if pattern == "*" || pattern == "**" {
			return true
		}
		matched, err := filepath.Match(pattern, resource)
		if err != nil {
			// Try as a prefix match for directory patterns
			if strings.HasSuffix(pattern, "**") {
				prefix := strings.TrimSuffix(pattern, "**")
				return strings.HasPrefix(resource, prefix)
			}
			return false
		}

		// Also handle ** patterns (match any depth)
		if !matched && strings.Contains(pattern, "**") {
			matched = matchDoublestar(pattern, resource)
		}

		if !matched {
			return false
		}
	}

	return true
}

// expandPathVariables substitutes $CWD and $HOME in a resource pattern. An
// empty replacement leaves the variable in place (so a misconfigured rule
// fails closed rather than matching the empty path).
func expandPathVariables(pattern, cwd, home string) string {
	if cwd != "" {
		pattern = strings.ReplaceAll(pattern, "$CWD", cwd)
	}
	if home != "" {
		pattern = strings.ReplaceAll(pattern, "$HOME", home)
	}
	return pattern
}

// matchDoublestar handles ** glob patterns that match any directory depth.
func matchDoublestar(pattern, path string) bool {
	// Simple implementation: ** matches any number of path segments
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0]
	suffix := parts[1]

	if prefix != "" && !strings.HasPrefix(path, prefix) {
		return false
	}

	if suffix != "" && !strings.HasSuffix(path, suffix) {
		return false
	}

	return true
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
		for i := len(stack) - 1; i >= 0; i-- {
			f := stack[i].Pos.Filename()
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
// Equivalent to "no permission system" — any Check call returns nil.
func AllowAllPermissions() *PermissionConfig {
	return &PermissionConfig{
		Allow:   []string{"*.*"},
		Default: DefaultAllow,
	}
}

// DenyAllPermissions returns a config that denies every gated operation.
// Pure utility modules (strings, json, yaml, …) bypass the permission system,
// so they remain available; any module that calls Check (fs, os, http, ssh,
// k8s, ai, mcp, io) is blocked.
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
}

// allow-net: allow-fs plus low-level protocol networking.
var allowNetRules = append(append([]string{}, allowFSRules...),
	"http.client",
	"ssh.connect", "ssh.transfer",
)

// allow-local: allow-net plus serve, $CWD-scoped exec, and higher-level
// networked services (ai, k8s, mcp). Withholds unrestricted exec, k8s.exec,
// and process control — that is the line to allow-all.
var allowLocalRules = append(append([]string{}, allowNetRules...),
	"http.server",
	"os.exec($CWD/**)",
	"ai.generate",
	"k8s.read", "k8s.write", "k8s.config",
	"mcp.client", "mcp.server",
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

// suggestProfile returns the lowest built-in profile on the capability ladder
// that would actually grant this specific call, for prescriptive denial
// messages. It tests the real profiles (resource-scoped rules included), so
// e.g. exec of a $CWD binary suggests allow-local while exec of a system
// binary suggests allow-all. The names match the built-in profile constants in
// libkite/permissions.
func suggestProfile(module, category, function, resource string) string {
	ladder := []struct {
		name string
		cfg  *PermissionConfig
	}{
		{"allow-fs", AllowFSPermissions()},
		{"allow-net", AllowNetPermissions()},
		{"allow-local", AllowLocalPermissions()},
		{"allow-all", AllowAllPermissions()},
	}
	for _, p := range ladder {
		c, err := NewPermissionChecker(p.cfg)
		if err == nil && c.allows(module, category, function, resource) {
			return p.name
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
