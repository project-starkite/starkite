// Package permissions resolves --permissions values into
// libkite.PermissionConfig instances.
//
// A --permissions value is a profile name only:
//
//  1. A built-in profile forming the capability ladder, each a superset of the
//     prior: "deny-all", "allow-fs", "allow-net", "allow-local", "allow-all".
//  2. A profile defined in config.yaml's `permissions:` map (passed to Resolve).
//
// Resolve treats an empty value as deny-all, unless config.yaml defines a
// profile named "default", which then becomes the implicit profile.
package permissions

import (
	"fmt"
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

// DefaultProfileName is the reserved profile name in config.permissions that,
// when defined, becomes the implicit profile for an unspecified --permissions.
const DefaultProfileName = "default"

// ProfileSpec is a single permissions profile defined in config.yaml.
//
// Two YAML forms are accepted:
//
//	ci: { allow: [...], deny: [...] }   # full spec — allow-list only; a
//	                                    # capability is granted only if it
//	                                    # appears in Allow and not in Deny
//	default: allow-fs                   # alias — the profile is the named
//	                                    # built-in
type ProfileSpec struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`

	// Alias names a built-in profile when the YAML value is a scalar string
	// instead of an allow/deny map. Mutually exclusive with Allow/Deny.
	Alias string `yaml:"-"`
}

// UnmarshalYAML accepts either a scalar built-in alias or an allow/deny map.
func (p *ProfileSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var alias string
		if err := node.Decode(&alias); err != nil {
			return err
		}
		p.Alias = alias
		return nil
	}
	type plain struct {
		Allow []string `yaml:"allow,omitempty"`
		Deny  []string `yaml:"deny,omitempty"`
	}
	var sp plain
	if err := node.Decode(&sp); err != nil {
		return err
	}
	p.Allow, p.Deny = sp.Allow, sp.Deny
	return nil
}

func (p ProfileSpec) toConfig() (*libkite.PermissionConfig, error) {
	if p.Alias != "" {
		cfg := builtin(p.Alias)
		if cfg == nil {
			return nil, fmt.Errorf("permissions: alias %q must name a built-in profile (%s)",
				p.Alias, strings.Join(builtinProfiles, ", "))
		}
		return cfg, nil
	}
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
//   - a profile defined in config; an undefined name (including "default") errors.
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

// LoadProfile resolves a built-in profile name only (no config-defined
// profiles). Used where no config map is available.
func LoadProfile(value string) (*libkite.PermissionConfig, error) {
	if value == "" {
		return nil, nil
	}
	if cfg := builtin(value); cfg != nil {
		return cfg, nil
	}
	return nil, fmt.Errorf("permissions: unknown profile %q (built-ins: %s)",
		value, strings.Join(builtinProfiles, ", "))
}
