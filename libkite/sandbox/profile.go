package sandbox

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

//go:embed profiles/opaque.yaml profiles/net-access.yaml profiles/host.yaml
var builtinProfiles embed.FS

// UserConfigFile is the default location for user-defined sandbox
// profiles, relative to the user's home directory. The "sandbox:" section
// of the unified config file holds a map of profile name → profile.
const UserConfigFile = ".starkite/config.yaml"

// errProfileNotFound reports that a named profile is not defined in any
// config file (as opposed to a parse or validation error). LoadProfile
// uses it to fall back to the opaque rung for the reserved "default" name.
var errProfileNotFound = errors.New("sandbox: profile not found")

// securityFile is the on-disk schema of the sections this package consumes
// from config.yaml. Other sections (config:, permissions:) are parsed
// elsewhere and ignored here.
type securityFile struct {
	Sandbox map[string]profileSpec `yaml:"sandbox"`
	// Permissions is parsed but unused — declared so strict decoders
	// don't reject documents that include it.
	Permissions map[string]any `yaml:"permissions,omitempty"`
}

// profileSpec is one value in config.yaml's "sandbox:" map. Three YAML
// forms are accepted:
//
//	ci: net-access                      # scalar — shorthand for {base: net-access}
//	ci: { network: ..., mounts: ... }   # self-contained body
//	ci: { base: host, mounts: ... }     # composed body — start from a rung
//
// The map form is captured as a raw node and re-decoded strictly in
// profileFromSpec, so unknown fields inside a profile error loudly even
// though the outer config file is parsed leniently.
type profileSpec struct {
	Base string
	Node yaml.Node
}

// UnmarshalYAML accepts either a scalar (shorthand for {base: <rung>}) or a
// profile map.
func (ps *profileSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&ps.Base)
	}
	ps.Node = *node
	return nil
}

// profileBody is the strict YAML schema of a full profile definition. The
// shape matches what users write under config.yaml's "sandbox:" map, so
// the same decoder parses both embedded and user-defined profiles. Base,
// when set, names a built-in rung the body composes on top of.
type profileBody struct {
	Base      string      `yaml:"base,omitempty"`
	Driver    string      `yaml:"driver,omitempty"`
	Image     string      `yaml:"image,omitempty"`
	Network   string      `yaml:"network,omitempty"`
	MaxMemory string      `yaml:"max_memory,omitempty"`
	Timeout   string      `yaml:"timeout,omitempty"`
	Mounts    []mountSpec `yaml:"mounts,omitempty"`
}

// mountSpec is the YAML form of a single mount entry. Defaults applied
// during decode-to-Mount: Type=bind, Mode=ro (rw for tmpfs).
type mountSpec struct {
	Source      string `yaml:"source,omitempty"`
	Destination string `yaml:"destination"`
	Type        string `yaml:"type,omitempty"`
	Mode        string `yaml:"mode,omitempty"`
}

// loadBuiltin reads an embedded rung YAML and returns a fully-validated
// Profile. Bind mounts whose host source is absent are dropped — the
// rungs reference host-support files (/etc/ssl/certs, /lib, …) that
// minimal hosts may not have, and a built-in must work everywhere.
func loadBuiltin(name string) (Profile, error) {
	data, err := builtinProfiles.ReadFile("profiles/" + name + ".yaml")
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: built-in profile %q: %w", name, err)
	}
	p, err := decodeProfile(name, data, "")
	if err != nil {
		return Profile{}, err
	}
	kept := p.Mounts[:0]
	for _, m := range p.Mounts {
		if m.Type == MountBind {
			if _, err := os.Stat(m.Source); err != nil {
				continue
			}
		}
		kept = append(kept, m)
	}
	p.Mounts = kept
	return p, nil
}

// decodeProfile parses YAML bytes into a Profile, applying defaults,
// expanding $CWD and $HOME in mount paths, and validating the result.
//
// origin is a short label used in error messages: a file path, etc.
// Empty origin omits the suffix.
func decodeProfile(name string, data []byte, origin string) (Profile, error) {
	var body profileBody
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&body); err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: %w", name, errWithOrigin(err, origin))
	}
	if body.Base != "" {
		return composeProfile(name, body, origin)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: getwd: %w", name, err)
	}
	home, _ := os.UserHomeDir() // empty home leaves $HOME unexpanded → abs-path validation fails closed

	p := Profile{
		Name:   name,
		Driver: body.Driver,
		Image:  body.Image,
	}
	if p.Driver == "" {
		p.Driver = DriverDefault
	}

	if body.MaxMemory != "" {
		mem, err := parseMemoryMB(body.MaxMemory)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: %w", name, fmtOrigin(origin), err)
		}
		p.MaxMemoryMB = mem
	}

	if body.Timeout != "" {
		dur, err := parseDuration(body.Timeout)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: %w", name, fmtOrigin(origin), err)
		}
		p.Timeout = dur
	}

	switch body.Network {
	case string(NetworkHost):
		p.Network = NetworkHost
	case string(NetworkLoopback):
		p.Network = NetworkLoopback
	case string(NetworkNone):
		p.Network = NetworkNone
	case "":
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: network mode is required (one of %q, %q)",
			name, fmtOrigin(origin), NetworkHost, NetworkLoopback)
	default:
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: unknown network mode %q (want %q or %q)",
			name, fmtOrigin(origin), body.Network, NetworkHost, NetworkLoopback)
	}

	for i, m := range body.Mounts {
		mount, err := decodeMount(m, cwd, home)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: mount[%d]: %w", name, fmtOrigin(origin), i, err)
		}
		p.Mounts = append(p.Mounts, mount)
	}

	return p, nil
}

func decodeMount(m mountSpec, cwd, home string) (Mount, error) {
	out := Mount{
		Source:      expandPath(m.Source, cwd, home),
		Destination: expandPath(m.Destination, cwd, home),
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

	if !isContainerAbs(out.Destination) {
		return Mount{}, fmt.Errorf("destination %q must be an absolute path", out.Destination)
	}
	if out.Type == MountBind && !isHostAbs(out.Source) {
		return Mount{}, fmt.Errorf("source %q must be an absolute path", out.Source)
	}

	return out, nil
}

func isContainerAbs(p string) bool {
	return strings.HasPrefix(p, "/") || filepath.IsAbs(p)
}

func isHostAbs(p string) bool {
	return filepath.IsAbs(p) || filepath.VolumeName(p) != "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\")
}

// composeProfile builds a profile on top of a built-in rung: the rung's
// network unless the body overrides it, and the rung's mounts with the
// body's mounts appended — a body mount replaces a rung mount with the
// same destination. Composition can widen OR narrow a rung (e.g. remount
// $HOME rw over host's ro); the result is the author's responsibility,
// like any user profile.
func composeProfile(name string, body profileBody, origin string) (Profile, error) {
	switch body.Base {
	case ProfileOpaque, ProfileNetAccess, ProfileHost:
	default:
		return Profile{}, fmt.Errorf(
			"sandbox: profile %q%s: base %q must name a built-in profile (%s, %s, %s)",
			name, fmtOrigin(origin), body.Base, ProfileOpaque, ProfileNetAccess, ProfileHost)
	}
	base, err := loadBuiltin(body.Base)
	if err != nil {
		return Profile{}, err
	}

	p := Profile{
		Name:        name,
		Driver:      base.Driver,
		Image:       base.Image,
		Network:     base.Network,
		MaxMemoryMB: base.MaxMemoryMB,
		Timeout:     base.Timeout,
	}

	if body.Driver != "" {
		p.Driver = body.Driver
	}
	if p.Driver == "" {
		p.Driver = DriverDefault
	}
	if body.Image != "" {
		p.Image = body.Image
	}
	if body.MaxMemory != "" {
		mem, err := parseMemoryMB(body.MaxMemory)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: %w", name, fmtOrigin(origin), err)
		}
		p.MaxMemoryMB = mem
	}
	if body.Timeout != "" {
		dur, err := parseDuration(body.Timeout)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: %w", name, fmtOrigin(origin), err)
		}
		p.Timeout = dur
	}

	switch body.Network {
	case "":
	case string(NetworkHost):
		p.Network = NetworkHost
	case string(NetworkLoopback):
		p.Network = NetworkLoopback
	case string(NetworkNone):
		p.Network = NetworkNone
	default:
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: unknown network mode %q (want %q or %q)",
			name, fmtOrigin(origin), body.Network, NetworkHost, NetworkLoopback)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: getwd: %w", name, err)
	}
	home, _ := os.UserHomeDir()

	var added []Mount
	for i, m := range body.Mounts {
		mount, err := decodeMount(m, cwd, home)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: mount[%d]: %w", name, fmtOrigin(origin), i, err)
		}
		added = append(added, mount)
	}
	// User-authored mounts must exist on the host (the rung's own mounts
	// were already filtered at loadBuiltin).
	if err := validateSources(Profile{Name: name, Mounts: added}, origin); err != nil {
		return Profile{}, err
	}

	override := make(map[string]int, len(added))
	for i, m := range added {
		override[m.Destination] = i
	}
	used := make(map[string]bool, len(added))
	for _, m := range base.Mounts {
		if i, ok := override[m.Destination]; ok {
			p.Mounts = append(p.Mounts, added[i])
			used[m.Destination] = true
			continue
		}
		p.Mounts = append(p.Mounts, m)
	}
	for _, m := range added {
		if !used[m.Destination] {
			p.Mounts = append(p.Mounts, m)
		}
	}
	return p, nil
}

func parseMemoryMB(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	upper := strings.ToUpper(s)
	if strings.HasSuffix(upper, "GB") || strings.HasSuffix(upper, "G") {
		multiplier = 1024
		s = strings.TrimRight(s, "gGbB")
	} else if strings.HasSuffix(upper, "MB") || strings.HasSuffix(upper, "M") {
		multiplier = 1
		s = strings.TrimRight(s, "mMbB")
	} else if strings.HasSuffix(upper, "KB") || strings.HasSuffix(upper, "K") {
		multiplier = 0
		s = strings.TrimRight(s, "kKbB")
	}
	var val int64
	_, err := fmt.Sscanf(s, "%d", &val)
	if err != nil {
		return 0, fmt.Errorf("invalid memory format: %q", s)
	}
	return val * multiplier, nil
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	return time.ParseDuration(s)
}

// validateSources checks that every bind-mount source exists on the host.
// User-profile mounts fail loudly on a missing path (fail-closed on
// misconfiguration); built-in rungs are filtered instead (loadBuiltin).
func validateSources(p Profile, origin string) error {
	for _, m := range p.Mounts {
		if m.Type != MountBind {
			continue
		}
		if _, err := os.Stat(m.Source); err != nil {
			return fmt.Errorf("sandbox: profile %q%s: bind source %q does not exist on the host",
				p.Name, fmtOrigin(origin), m.Source)
		}
	}
	return nil
}

// expandPath resolves $CWD and $HOME substitutions. Sources/destinations
// may use "$CWD"/"$CWD/sub" for the directory `kite` was invoked from and
// "$HOME"/"$HOME/sub" for the invoking user's home directory. Other
// shell-style expansions (~, env vars) are intentionally NOT supported —
// broad expansion makes profiles harder to audit.
func expandPath(p, cwd, home string) string {
	if p == "" {
		return ""
	}
	if p == "$CWD" {
		return cwd
	}
	if rest, ok := strings.CutPrefix(p, "$CWD/"); ok {
		return filepath.Join(cwd, rest)
	}
	if home != "" {
		if p == "$HOME" {
			return home
		}
		if rest, ok := strings.CutPrefix(p, "$HOME/"); ok {
			return filepath.Join(home, rest)
		}
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

// loadNamed resolves a profile name against the "sandbox:" maps of
// ./config.yaml and ~/.starkite/config.yaml; the project-local file wins.
// A name defined in neither file returns errProfileNotFound (wrapped).
func loadNamed(name string) (Profile, error) {
	paths := []string{"config.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, UserConfigFile))
	}

	var defined []string
	var searched []string
	for _, path := range paths {
		sf, err := readSecurityFile(path)
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
			"%w: %q (built-ins: %s, %s, %s; no config.yaml found)",
			errProfileNotFound, name, ProfileOpaque, ProfileNetAccess, ProfileHost)
	}
	return Profile{}, fmt.Errorf(
		"%w: %q in %s (defined: %v; built-ins: %s, %s, %s)",
		errProfileNotFound, name, strings.Join(searched, ", "), defined,
		ProfileOpaque, ProfileNetAccess, ProfileHost)
}

// readSecurityFile parses path's "sandbox:" section.
func readSecurityFile(path string) (*securityFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	var sf securityFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// Note: KnownFields off here. The file legitimately contains other
	// sections (config:, permissions:). Schema strictness is applied
	// per-profile via profileFromSpec.
	if err := dec.Decode(&sf); err != nil {
		// Empty file is fine; treat as no sandbox section.
		if strings.Contains(err.Error(), "EOF") {
			return &sf, nil
		}
		return nil, fmt.Errorf("sandbox: parse %s: %w", path, err)
	}
	return &sf, nil
}

// profileFromSpec builds a Profile from one "sandbox:" map value. A scalar
// (shorthand for {base: <rung>}) resolves to its built-in rung; a map body
// is re-decoded strictly (unknown fields error), composed on its base when
// one is named, and its own bind sources must exist on the host.
func profileFromSpec(name string, spec profileSpec, origin string) (Profile, error) {
	if spec.Base != "" {
		return composeProfile(name, profileBody{Base: spec.Base}, origin)
	}

	raw, err := yaml.Marshal(&spec.Node)
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: re-marshal: %w", name, err)
	}
	p, err := decodeProfile(name, raw, origin)
	if err != nil {
		return Profile{}, err
	}
	// Composed profiles re-validate harmlessly: the base's mounts were
	// filtered to existing sources and the additions already validated.
	if err := validateSources(p, origin); err != nil {
		return Profile{}, err
	}
	return p, nil
}
