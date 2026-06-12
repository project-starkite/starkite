package sandbox

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/default.yaml profiles/strict.yaml
var builtinProfiles embed.FS

// UserConfigFile is the default location for user-defined sandbox
// profiles, relative to the user's home directory. The "sandbox:" section
// of the unified config file holds a map of profile name → profileSpec.
const UserConfigFile = ".starkite/config.yaml"

// securityFile is the on-disk schema of the sections this package consumes
// from config.yaml. Other
// sections (config:, permissions:) are parsed elsewhere and ignored here.
type securityFile struct {
	Sandbox map[string]profileSpec `yaml:"sandbox"`
	// Permissions is parsed but unused — declared so KnownFields(true)
	// doesn't reject documents that include it.
	Permissions map[string]any `yaml:"permissions,omitempty"`
}

// profileSpec is the YAML schema for a sandbox profile. The shape
// matches what users write under config.yaml's "sandbox:" map, so the
// same decoder parses both embedded and user-defined profiles.
type profileSpec struct {
	Network string      `yaml:"network"`
	Mounts  []mountSpec `yaml:"mounts,omitempty"`
}

// mountSpec is the YAML form of a single mount entry. Defaults applied
// during decode-to-Mount: Type=bind, Mode=ro.
type mountSpec struct {
	Source      string `yaml:"source,omitempty"`
	Destination string `yaml:"destination"`
	Type        string `yaml:"type,omitempty"`
	Mode        string `yaml:"mode,omitempty"`
	Optional    bool   `yaml:"optional,omitempty"`
}

// loadBuiltin reads an embedded profile YAML and returns a fully-validated
// Profile. The name must match a basename under profiles/.
func loadBuiltin(name string) (Profile, error) {
	data, err := builtinProfiles.ReadFile("profiles/" + name + ".yaml")
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: built-in profile %q: %w", name, err)
	}
	return decodeProfile(name, data, "")
}

// decodeProfile parses YAML bytes into a Profile, applying defaults,
// expanding $CWD in mount paths, and validating the result.
//
// origin is a short label used in error messages: "(built-in)", a file
// path, etc. Empty origin omits the suffix.
func decodeProfile(name string, data []byte, origin string) (Profile, error) {
	var spec profileSpec
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&spec); err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: %w", name, errWithOrigin(err, origin))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: getwd: %w", name, err)
	}

	p := Profile{Name: name}
	switch spec.Network {
	case string(NetworkHost):
		p.Network = NetworkHost
	case string(NetworkSandboxLoopback):
		p.Network = NetworkSandboxLoopback
	case "":
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: network mode is required (one of %q, %q)",
			name, fmtOrigin(origin), NetworkHost, NetworkSandboxLoopback)
	default:
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: unknown network mode %q (want %q or %q)",
			name, fmtOrigin(origin), spec.Network, NetworkHost, NetworkSandboxLoopback)
	}

	for i, m := range spec.Mounts {
		mount, err := decodeMount(m, cwd)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: mount[%d]: %w", name, fmtOrigin(origin), i, err)
		}
		p.Mounts = append(p.Mounts, mount)
	}

	return p, nil
}

func decodeMount(m mountSpec, cwd string) (Mount, error) {
	out := Mount{
		Source:      expandPath(m.Source, cwd),
		Destination: expandPath(m.Destination, cwd),
		Optional:    m.Optional,
	}

	if out.Destination == "" {
		return Mount{}, fmt.Errorf("destination is required")
	}

	switch m.Type {
	case "", string(MountBind):
		out.Type = MountBind
		if out.Source == "" {
			return Mount{}, fmt.Errorf("bind mount requires source")
		}
	case string(MountTmpfs):
		out.Type = MountTmpfs
		if m.Source != "" {
			return Mount{}, fmt.Errorf("tmpfs mount must not specify source (got %q)", m.Source)
		}
		if m.Optional {
			return Mount{}, fmt.Errorf("tmpfs mount cannot be optional")
		}
	default:
		return Mount{}, fmt.Errorf("unknown mount type %q (want %q or %q)", m.Type, MountBind, MountTmpfs)
	}

	switch m.Mode {
	case string(MountRO):
		out.Mode = MountRO
	case string(MountRW):
		out.Mode = MountRW
	case "":
		// Default: ro for binds (safer; user must opt into writes), rw
		// for tmpfs (a read-only tmpfs is useless).
		if out.Type == MountTmpfs {
			out.Mode = MountRW
		} else {
			out.Mode = MountRO
		}
	default:
		return Mount{}, fmt.Errorf("unknown mode %q (want %q or %q)", m.Mode, MountRO, MountRW)
	}

	if !filepath.IsAbs(out.Destination) {
		return Mount{}, fmt.Errorf("destination %q must be an absolute path", out.Destination)
	}
	if out.Type == MountBind && !filepath.IsAbs(out.Source) {
		return Mount{}, fmt.Errorf("source %q must be an absolute path", out.Source)
	}

	return out, nil
}

// expandPath resolves $CWD substitutions. Sources/destinations may use
// "$CWD" or "$CWD/sub" to refer to the current working directory at the
// time LoadProfile runs — i.e. where the user invoked `kite`. Other
// shell-style expansions (~, $HOME, env vars) are intentionally NOT
// supported here: $HOME would expose user-data risk, and broad env-var
// expansion makes profiles harder to audit.
func expandPath(p, cwd string) string {
	if p == "" {
		return ""
	}
	if p == "$CWD" {
		return cwd
	}
	if rest, ok := strings.CutPrefix(p, "$CWD/"); ok {
		return filepath.Join(cwd, rest)
	}
	return p
}

func fmtOrigin(origin string) string {
	if origin == "" {
		return ""
	}
	return " (" + origin + ")"
}

func errWithOrigin(err error, origin string) error {
	if origin == "" {
		return err
	}
	return fmt.Errorf("%s: %w", origin, err)
}

// isFilePath reports whether a --sandbox value should be resolved as a
// file path rather than a built-in or named user profile. A path is
// recognized by an explicit separator or a .yaml/.yml suffix on the
// portion before any "#name" fragment.
// loadNamed resolves a profile name against the "sandbox:" maps of
// ./config.yaml and ~/.starkite/config.yaml; the project-local file wins.
func loadNamed(name string) (Profile, error) {
	paths := []string{"config.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, UserConfigFile))
	}

	var defined []string
	var searched []string
	for _, path := range paths {
		sf, _, err := readSecurityFile(path)
		if err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
				continue
			}
			return Profile{}, err
		}
		searched = append(searched, path)
		if spec, ok := sf.Sandbox[name]; ok {
			return profileFromSpec(name, spec, path)
		}
		for n := range sf.Sandbox {
			defined = append(defined, n)
		}
	}

	if len(searched) == 0 {
		return Profile{}, fmt.Errorf(
			"sandbox: unknown profile %q (built-ins: %s, %s; no config.yaml found)",
			name, ProfileDefault, ProfileStrict)
	}
	return Profile{}, fmt.Errorf(
		"sandbox: profile %q not found in %s (defined: %v; built-ins: %s, %s)",
		name, strings.Join(searched, ", "), defined, ProfileDefault, ProfileStrict)
}

// readSecurityFile parses path as a security file (containing a
// "sandbox:" section). It also returns the raw bytes, so the caller can
// fall back to top-level-profileSpec parsing when "sandbox:" is empty.
func readSecurityFile(path string) (*securityFile, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("sandbox: %w", err)
	}
	var sf securityFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// Note: KnownFields off here. The file may legitimately contain a
	// "permissions:" section the sandbox loader doesn't recognize fully,
	// or other forward-compatible keys. Schema strictness is applied
	// per-profile via decodeProfile / profileFromSpec.
	if err := dec.Decode(&sf); err != nil {
		// Empty file is fine; treat as no sandbox section.
		if errors.Is(err, errors.New("EOF")) || strings.Contains(err.Error(), "EOF") {
			return &sf, data, nil
		}
		return nil, nil, fmt.Errorf("sandbox: parse %s: %w", path, err)
	}
	return &sf, data, nil
}

// profileFromSpec builds a Profile from an already-decoded profileSpec.
// The spec was decoded without KnownFields(true) (the parent file may
// have other sections), so re-marshal here and pass through decodeProfile
// for the same strict validation applied to embedded built-ins.
func profileFromSpec(name string, spec profileSpec, origin string) (Profile, error) {
	raw, err := yaml.Marshal(spec)
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: re-marshal: %w", name, err)
	}
	return decodeProfile(name, raw, origin)
}
