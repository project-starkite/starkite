// Package permissions resolves --permissions values into
// libkite.PermissionConfig instances.
//
// A value resolves, in order, to:
//
//  1. A built-in profile name forming the capability ladder, each a superset of
//     the prior: "deny-all", "allow-fs", "allow-net", "allow-local", "allow-all".
//  2. Inline rules: leading "allow:" or "deny:" — e.g. "allow:fs.read;deny:os.exec".
//     These layer on a deny-all baseline.
//  3. A file path: contains '/' or ends in .yaml/.yml; an optional "#name"
//     fragment selects a profile when the file holds more than one.
//  4. A named profile from config.yaml's `permissions:` map (passed to Resolve).
//
// Resolve treats an empty value as deny-all, unless config.yaml defines a
// profile named "default", which then becomes the implicit profile.
package permissions

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

// Built-in profile names — the capability ladder, each a superset of the prior.
const (
	ProfileDenyAll    = "deny-all"
	ProfileAllowFS    = "allow-fs"
	ProfileAllowNet   = "allow-net"
	ProfileAllowLocal = "allow-local"
	ProfileAllowAll   = "allow-all"
)

// builtinProfiles lists the built-in names for error messages.
var builtinProfiles = []string{
	ProfileDenyAll, ProfileAllowFS, ProfileAllowNet, ProfileAllowLocal, ProfileAllowAll,
}

// profileFile is the on-disk schema for a standalone permissions file passed
// as --permissions=<path>. It holds a map of named profiles under a top-level
// `permissions:` key, mirroring the config.yaml section.
type profileFile struct {
	Permissions map[string]ProfileSpec `yaml:"permissions"`
}

// DefaultProfileName is the reserved profile name in config.permissions that,
// when defined, becomes the implicit profile for an unspecified --permissions.
const DefaultProfileName = "default"

// ProfileSpec is a single permissions profile defined in config.yaml. It is
// allow-list only: a capability is granted only if it appears in Allow (and not
// in Deny). There is no default-allow mode — unmatched calls are denied.
type ProfileSpec struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

func (p ProfileSpec) toConfig() (*libkite.PermissionConfig, error) {
	return &libkite.PermissionConfig{
		Allow:   p.Allow,
		Deny:    p.Deny,
		Default: libkite.DefaultDeny,
	}, nil
}

// builtin returns the PermissionConfig for a built-in profile name, or nil if
// the name is not a built-in.
func builtin(value string) *libkite.PermissionConfig {
	switch value {
	case ProfileDenyAll:
		return libkite.DenyAllPermissions()
	case ProfileAllowFS:
		return libkite.AllowFSPermissions()
	case ProfileAllowNet:
		return libkite.AllowNetPermissions()
	case ProfileAllowLocal:
		return libkite.AllowLocalPermissions()
	case ProfileAllowAll:
		return libkite.AllowAllPermissions()
	}
	return nil
}

// Resolve turns a --permissions value into a PermissionConfig, using the
// user-defined profiles from config.yaml's `permissions:` map.
//
// Rules:
//   - "" (unspecified): the "default" profile from config if defined, else deny-all.
//   - a built-in name (deny-all/allow-fs/allow-net/allow-local/allow-all).
//   - inline rules ("allow:..."/"deny:...") layered on a deny-all baseline.
//   - a file path (with optional #name fragment).
//   - a named profile from config; an undefined name (including "default") errors.
func Resolve(value string, defined map[string]ProfileSpec) (*libkite.PermissionConfig, error) {
	if value == "" {
		// Unspecified → the configured default profile, else deny-all.
		if spec, ok := defined[DefaultProfileName]; ok {
			return spec.toConfig()
		}
		return libkite.DenyAllPermissions(), nil
	}

	if cfg := builtin(value); cfg != nil {
		return cfg, nil
	}
	if isInline(value) {
		return parseInline(value)
	}
	if isFilePath(value) {
		return loadFromFile(value)
	}

	// Named profile from config.
	if spec, ok := defined[value]; ok {
		return spec.toConfig()
	}
	names := make([]string, 0, len(defined))
	for n := range defined {
		names = append(names, n)
	}
	return nil, fmt.Errorf("permissions: unknown profile %q (built-ins: %s; config-defined: %v)",
		value, strings.Join(builtinProfiles, ", "), names)
}

// LoadProfile resolves a value against built-ins, inline rules, and file paths
// only (no config-defined profiles). Used where no config map is available.
func LoadProfile(value string) (*libkite.PermissionConfig, error) {
	if value == "" {
		return nil, nil
	}
	if cfg := builtin(value); cfg != nil {
		return cfg, nil
	}
	if isInline(value) {
		return parseInline(value)
	}
	if isFilePath(value) {
		return loadFromFile(value)
	}
	return nil, fmt.Errorf("permissions: unknown profile %q (built-ins: %s)",
		value, strings.Join(builtinProfiles, ", "))
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

	pf, err := readProfileFile(path)
	if err != nil {
		return nil, err
	}
	return pickFromFile(pf, fragment, path)
}

func readProfileFile(path string) (*profileFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("permissions: %w", err)
	}
	defer f.Close()

	var pf profileFile
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&pf); err != nil {
		return nil, fmt.Errorf("permissions: parse %s: %w", path, err)
	}
	return &pf, nil
}

func pickFromFile(sf *profileFile, fragment, path string) (*libkite.PermissionConfig, error) {
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
