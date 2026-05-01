package libkite

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"go.starlark.net/starlark"
)

// permissionKey is the thread.Local key for the permission checker.
const permissionKey = "libkite.permissions"

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

// NewPermissionChecker creates a checker from the given config.
func NewPermissionChecker(config *PermissionConfig) (*PermissionChecker, error) {
	if config == nil {
		return nil, nil
	}

	checker := &PermissionChecker{
		default_: config.Default,
	}

	// Parse allow rules
	for _, pattern := range config.Allow {
		rule, err := ParseRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid allow rule %q: %w", pattern, err)
		}
		checker.allow = append(checker.allow, *rule)
	}

	// Parse deny rules
	for _, pattern := range config.Deny {
		rule, err := ParseRule(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid deny rule %q: %w", pattern, err)
		}
		checker.deny = append(checker.deny, *rule)
	}

	return checker, nil
}

// Check validates if module.function(resource) is permitted.
func (c *PermissionChecker) Check(module, function, resource string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check deny rules first (deny takes precedence)
	for _, rule := range c.deny {
		if rule.Matches(module, function, resource) {
			return &PermissionError{
				Module:   module,
				Function: function,
				Resource: resource,
				Reason:   fmt.Sprintf("blocked by deny rule: %s", rule.Raw),
			}
		}
	}

	// Check allow rules
	for _, rule := range c.allow {
		if rule.Matches(module, function, resource) {
			return nil // Allowed
		}
	}

	// No rule matched, use default
	if c.default_ == DefaultAllow {
		return nil
	}

	return &PermissionError{
		Module:   module,
		Function: function,
		Resource: resource,
		Reason:   "no matching allow rule",
		Allowed:  c.allowedPatterns(),
	}
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
	Module   string // Module name or "*"
	Function string // Function name or "*"
	Resource string // Resource glob pattern or ""
	Raw      string // Original pattern string
}

// ParseRule parses a permission pattern string.
//
// Pattern formats:
//
//	"module.*"                  → module=module, function=*, resource=""
//	"module.function"           → module=module, function=function, resource=""
//	"module.function(resource)" → module=module, function=function, resource=glob
//	"*.*"                       → module=*, function=*, resource=""
func ParseRule(pattern string) (*Rule, error) {
	rule := &Rule{Raw: pattern}

	// Check for resource pattern: module.function(resource)
	if idx := strings.Index(pattern, "("); idx != -1 {
		if !strings.HasSuffix(pattern, ")") {
			return nil, fmt.Errorf("unclosed parenthesis in pattern")
		}
		rule.Resource = pattern[idx+1 : len(pattern)-1]
		pattern = pattern[:idx]
	}

	// Split module.function
	parts := strings.SplitN(pattern, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("pattern must be module.function, got %q", pattern)
	}

	rule.Module = parts[0]
	rule.Function = parts[1]

	if rule.Module == "" || rule.Function == "" {
		return nil, fmt.Errorf("module and function cannot be empty")
	}

	return rule, nil
}

// Matches checks if the rule matches the given operation.
func (r *Rule) Matches(module, function, resource string) bool {
	// Match module
	if r.Module != "*" && r.Module != module {
		return false
	}

	// Match function
	if r.Function != "*" && r.Function != function {
		return false
	}

	// Match resource (if pattern specified)
	if r.Resource != "" && resource != "" {
		// Expand common variables
		pattern := r.Resource
		pattern = expandPathVariables(pattern)

		// Use filepath.Match for glob matching
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

// expandPathVariables expands common path variables in patterns.
func expandPathVariables(pattern string) string {
	// These would be set at runtime based on context
	// For now, we'll handle common cases
	if strings.HasPrefix(pattern, "$CWD/") || pattern == "$CWD" {
		// $CWD is handled at check time
	}
	if strings.HasPrefix(pattern, "$HOME/") || pattern == "$HOME" {
		// $HOME is handled at check time
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

// Check validates module.function(resource) against the permission rules
// stored in the thread's local storage. Returns nil if no checker is set
// (trusted mode / backward compatible).
//
// This is the main entry point for modules to check permissions.
func Check(thread *starlark.Thread, module, function, resource string) error {
	if thread == nil {
		return nil // No thread = trusted mode
	}

	checker, _ := thread.Local(permissionKey).(*PermissionChecker)
	if checker == nil {
		return nil // No checker = trusted mode (backward compatible)
	}

	return checker.Check(module, function, resource)
}

// SetPermissions stores the permission checker in thread.Local.
// Called by Runtime before executing each script.
func SetPermissions(thread *starlark.Thread, checker *PermissionChecker) {
	if thread != nil && checker != nil {
		thread.SetLocal(permissionKey, checker)
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

// TrustedPermissions returns a config that allows all operations.
func TrustedPermissions() *PermissionConfig {
	return &PermissionConfig{
		Allow:   []string{"*.*"},
		Default: DefaultAllow,
	}
}

// SandboxedPermissions returns a config with only safe modules allowed.
// This blocks all I/O operations (fs, os, http, ssh).
func SandboxedPermissions() *PermissionConfig {
	return &PermissionConfig{
		Allow: []string{
			// Pure utility modules (no I/O, no side effects)
			"strings.*", "regexp.*", "json.*", "yaml.*", "csv.*",
			"base64.*", "hash.*", "uuid.*", "gzip.*", "zip.*",
			"time.*", "template.*", "table.*", "log.*",
			"fmt.*", "concur.*", "retry.*",
			// Safe fs functions (path manipulation only, no I/O)
			"fs.join", "fs.split", "fs.dir", "fs.base",
			"fs.ext", "fs.abs", "fs.rel", "fs.clean", "fs.match",
		},
		Default: DefaultDeny,
	}
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
