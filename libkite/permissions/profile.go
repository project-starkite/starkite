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
	ProfileDenyAll       = "deny-all"
	ProfileAllowFS       = "allow-fs"
	ProfileAllowNet      = "allow-net"
	ProfileAllowLocal    = "allow-local"
	ProfileAllowAll      = "allow-all"
	ProfileAllowAllShell = "allow-all-shell"
)

// builtinProfiles lists the built-in names for error messages.
var builtinProfiles = []string{
	ProfileDenyAll, ProfileAllowFS, ProfileAllowNet, ProfileAllowLocal, ProfileAllowAll, ProfileAllowAllShell,
}

// DefaultProfileName is the reserved profile name in config.permissions that,
// when defined, becomes the implicit profile for an unspecified --permissions.
const DefaultProfileName = "default"

// ProfileSpec is a single permissions profile defined in config.yaml.
//
// Three YAML forms are accepted:
//
//	ci: { allow: [...], deny: [...] }   # self-contained spec — allow-list
//	                                    # only; a capability is granted only
//	                                    # if it appears in Allow and not Deny
//	deploy:                             # composed spec — start from a
//	  base: allow-fs                    # built-in profile, append rules
//	  allow: [k8s.write]
//	  deny: [fs.delete]
//	default: allow-fs                   # scalar — shorthand for
//	                                    # { base: allow-fs }
type ProfileSpec struct {
	Base  string   `yaml:"base,omitempty"`
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// UnmarshalYAML accepts either a scalar (shorthand for {base: <name>}) or a
// spec map.
func (p *ProfileSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var base string
		if err := node.Decode(&base); err != nil {
			return err
		}
		p.Base = base
		return nil
	}
	type plain struct {
		Base  string   `yaml:"base,omitempty"`
		Allow []string `yaml:"allow,omitempty"`
		Deny  []string `yaml:"deny,omitempty"`
	}
	var sp plain
	if err := node.Decode(&sp); err != nil {
		return err
	}
	p.Base, p.Allow, p.Deny = sp.Base, sp.Allow, sp.Deny
	return nil
}

// toConfig resolves the spec to a PermissionConfig. With a base, the result
// is the built-in's rules plus the spec's appended allow/deny lists, keeping
// the built-in's default; deny-first evaluation is unchanged, so an appended
// deny carves an exception out of the base's allows, and a base deny cannot
// be re-allowed.
func (p ProfileSpec) toConfig() (*libkite.PermissionConfig, error) {
	if p.Base == "" {
		return &libkite.PermissionConfig{
			Allow:   p.Allow,
			Deny:    p.Deny,
			Default: libkite.DefaultDeny,
		}, nil
	}

	base := builtin(p.Base)
	if base == nil {
		return nil, fmt.Errorf("permissions: base %q must name a built-in profile (%s)",
			p.Base, strings.Join(builtinProfiles, ", "))
	}
	allow, deny := base.Allow, base.Deny
	if len(p.Allow) > 0 {
		allow = append(append([]string{}, base.Allow...), p.Allow...)
	}
	if len(p.Deny) > 0 {
		deny = append(append([]string{}, base.Deny...), p.Deny...)
	}
	return &libkite.PermissionConfig{
		Allow:   allow,
		Deny:    deny,
		Default: base.Default,
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
	case ProfileAllowAllShell:
		return libkite.AllowAllShellPermissions()
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
