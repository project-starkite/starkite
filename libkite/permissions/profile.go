// Package permissions resolves --permissions values and per-script
// frontmatter into libkite.PermissionConfig instances.
//
// Resolution order for a value:
//
//  1. Built-in name: "allow-all", "strict", "deny-all".
//  2. Inline rules: leading "allow:" or "deny:" — e.g. "allow:fs.read,deny:os.exec".
//  3. File path: contains '/' or ends in .yaml/.yml; optional "#name" fragment
//     selects a profile when the file holds more than one.
//  4. Named user profile: looked up under "permissions.<name>" in
//     ~/.starkite/security.yaml.
//
// An empty value returns (nil, nil) — caller treats nil as trust mode.
package permissions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

// Built-in profile names.
const (
	ProfileAllowAll = "allow-all"
	ProfileStrict   = "strict"
	ProfileDenyAll  = "deny-all"
)

// UserSecurityFile is the default location for user permission profiles,
// relative to the user's home directory.
const UserSecurityFile = ".starkite/security.yaml"

// SecurityFile is the on-disk schema for ~/.starkite/security.yaml. The
// Sandbox section is reserved for sandbox-isolation Phase 4+ and is parsed
// but otherwise unused here.
type SecurityFile struct {
	Permissions map[string]ProfileSpec `yaml:"permissions"`
	Sandbox     map[string]any         `yaml:"sandbox,omitempty"`
}

// ProfileSpec is a single permissions profile in YAML form.
type ProfileSpec struct {
	Default string   `yaml:"default"`
	Allow   []string `yaml:"allow,omitempty"`
	Deny    []string `yaml:"deny,omitempty"`
}

func (p ProfileSpec) toConfig() (*libkite.PermissionConfig, error) {
	cfg := &libkite.PermissionConfig{Allow: p.Allow, Deny: p.Deny}
	switch p.Default {
	case "allow":
		cfg.Default = libkite.DefaultAllow
	case "deny", "":
		cfg.Default = libkite.DefaultDeny
	default:
		return nil, fmt.Errorf("invalid default %q (want \"allow\" or \"deny\")", p.Default)
	}
	return cfg, nil
}

// LoadProfile resolves a --permissions value to a PermissionConfig. See the
// package documentation for the resolution order.
func LoadProfile(value string) (*libkite.PermissionConfig, error) {
	if value == "" {
		return nil, nil
	}

	// (1) Built-ins
	switch value {
	case ProfileAllowAll:
		return libkite.AllowAllPermissions(), nil
	case ProfileStrict:
		return libkite.StrictPermissions(), nil
	case ProfileDenyAll:
		return libkite.DenyAllPermissions(), nil
	}

	// (2) Inline rules
	if isInline(value) {
		return parseInline(value)
	}

	// (3) File path (with optional fragment)
	if isFilePath(value) {
		return loadFromFile(value)
	}

	// (4) Named user profile
	return loadNamed(value)
}

func isInline(value string) bool {
	return strings.HasPrefix(value, "allow:") || strings.HasPrefix(value, "deny:")
}

func isFilePath(value string) bool {
	if strings.ContainsAny(value, "/\\") {
		return true
	}
	base := value
	if i := strings.Index(value, "#"); i >= 0 {
		base = value[:i]
	}
	return strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".yml")
}

// parseInline parses semicolon-separated allow:/deny: clauses.
//
// Grammar:
//
//	inline := clause (";" clause)*
//	clause := ("allow"|"deny") ":" rule ("," rule)*
//
// Whitespace around clauses, kinds, and rules is tolerated and trimmed. The
// resulting config has Default=DefaultDeny.
func parseInline(value string) (*libkite.PermissionConfig, error) {
	cfg := &libkite.PermissionConfig{Default: libkite.DefaultDeny}

	clauses := strings.Split(value, ";")
	for _, raw := range clauses {
		c := strings.TrimSpace(raw)
		if c == "" {
			return nil, fmt.Errorf("inline rules: empty clause in %q", value)
		}
		colon := strings.Index(c, ":")
		if colon < 0 {
			return nil, fmt.Errorf("inline rules: clause %q missing ':' (want allow:rule or deny:rule)", c)
		}
		kind := strings.TrimSpace(c[:colon])
		body := c[colon+1:]
		if strings.TrimSpace(body) == "" {
			return nil, fmt.Errorf("inline rules: clause %q has empty rule list", c)
		}
		rules := strings.Split(body, ",")
		for i, r := range rules {
			rules[i] = strings.TrimSpace(r)
			if rules[i] == "" {
				return nil, fmt.Errorf("inline rules: clause %q has empty rule entry", c)
			}
		}
		switch kind {
		case "allow":
			cfg.Allow = append(cfg.Allow, rules...)
		case "deny":
			cfg.Deny = append(cfg.Deny, rules...)
		default:
			return nil, fmt.Errorf("inline rules: clause kind %q must be \"allow\" or \"deny\"", kind)
		}
	}
	return cfg, nil
}

func loadFromFile(value string) (*libkite.PermissionConfig, error) {
	path := value
	fragment := ""
	if i := strings.Index(value, "#"); i >= 0 {
		path = value[:i]
		fragment = value[i+1:]
	}

	sf, err := readSecurityFile(path)
	if err != nil {
		return nil, err
	}
	return pickFromFile(sf, fragment, path)
}

func readSecurityFile(path string) (*SecurityFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("permissions: %w", err)
	}
	defer f.Close()

	var sf SecurityFile
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&sf); err != nil {
		return nil, fmt.Errorf("permissions: parse %s: %w", path, err)
	}
	return &sf, nil
}

func pickFromFile(sf *SecurityFile, fragment, path string) (*libkite.PermissionConfig, error) {
	if len(sf.Permissions) == 0 {
		return nil, fmt.Errorf("permissions: %s contains no permissions profiles", path)
	}
	if fragment != "" {
		spec, ok := sf.Permissions[fragment]
		if !ok {
			return nil, fmt.Errorf("permissions: profile %q not found in %s", fragment, path)
		}
		return spec.toConfig()
	}
	if len(sf.Permissions) == 1 {
		for _, spec := range sf.Permissions {
			return spec.toConfig()
		}
	}
	names := make([]string, 0, len(sf.Permissions))
	for n := range sf.Permissions {
		names = append(names, n)
	}
	return nil, fmt.Errorf("permissions: %s has multiple profiles %v; specify with %s#<name>", path, names, path)
}

func loadNamed(name string) (*libkite.PermissionConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("permissions: cannot resolve home directory: %w", err)
	}
	path := filepath.Join(home, UserSecurityFile)

	sf, err := readSecurityFile(path)
	if err != nil {
		// Surface a nicer message when the file simply doesn't exist.
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
			return nil, fmt.Errorf("permissions: unknown profile %q (built-ins: %s, %s, %s; %s does not exist)",
				name, ProfileAllowAll, ProfileStrict, ProfileDenyAll, path)
		}
		return nil, err
	}

	spec, ok := sf.Permissions[name]
	if !ok {
		names := make([]string, 0, len(sf.Permissions))
		for n := range sf.Permissions {
			names = append(names, n)
		}
		return nil, fmt.Errorf("permissions: profile %q not found in %s (defined: %v; built-ins: %s, %s, %s)",
			name, path, names, ProfileAllowAll, ProfileStrict, ProfileDenyAll)
	}
	return spec.toConfig()
}
