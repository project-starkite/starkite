package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

// UserConfigFile is the default location for user-defined sandbox
// profiles, relative to the user's home directory.
const UserConfigFile = ".starkite/config.yaml"

// errProfileNotFound reports that a named profile is not defined in any config file.
var errProfileNotFound = errors.New("sandbox: profile not found")

// SandboxConfig is the in-memory representation of the unified sandbox: section.
type SandboxConfig struct {
	Enabled  bool                   `yaml:"enabled"`
	Profiles map[string]ProfileBody `yaml:"-"`
}

// ProfileBody is the YAML schema of a capability profile definition.
type ProfileBody struct {
	Enabled          *bool           `yaml:"enabled,omitempty"`
	Base             string          `yaml:"base,omitempty"`
	Driver           string          `yaml:"driver,omitempty"`
	Image            string          `yaml:"image,omitempty"`
	Network          networkSpec     `yaml:"network,omitempty"`
	MaxMemory        string          `yaml:"max_memory,omitempty"`
	Timeout          string          `yaml:"timeout,omitempty"`
	Paths            PathsSpec       `yaml:"paths,omitempty"`
	Mounts           []mountSpec     `yaml:"mounts,omitempty"`
	Exec             ExecPolicySpec  `yaml:"exec,omitempty"`
	Calls            CallsPolicySpec `yaml:"calls,omitempty"`
	Env              envSpec         `yaml:"env,omitempty"`
	NoExec           *bool           `yaml:"no_exec,omitempty"`
	AllowedExecPaths []string        `yaml:"allowed_exec_paths,omitempty"`
}

// Validate checks that the profile body has valid network and limit specifications.
func (b *ProfileBody) Validate() error {
	hasCapabilities := len(b.Paths.Read) > 0 || len(b.Paths.Write) > 0 ||
		len(b.Exec.Allow) > 0 || len(b.Exec.Deny) > 0 ||
		len(b.Calls.Allow) > 0 || len(b.Calls.Deny) > 0

	if (b.Enabled == nil || *b.Enabled) && b.Base == "" && b.Network.Mode == "" && len(b.Network.Allow) == 0 && !hasCapabilities {
		return fmt.Errorf("network mode is required")
	}
	if b.Network.Mode != "" {
		switch NetworkMode(b.Network.Mode) {
		case NetworkNone, NetworkHost, NetworkLoopback:
		default:
			return fmt.Errorf("unknown network mode %q (want %q, %q, or %q)",
				b.Network.Mode, NetworkNone, NetworkHost, NetworkLoopback)
		}
	}
	if b.MaxMemory != "" {
		if _, err := parseMemoryMB(b.MaxMemory); err != nil {
			return err
		}
	}
	if b.Timeout != "" {
		if _, err := parseDuration(b.Timeout); err != nil {
			return err
		}
	}
	return nil
}

func isBuiltinProfile(name string) bool {
	switch name {
	case ProfileDenyAll, ProfileSandboxPath, ProfileSandboxExec, ProfileSandboxNet, ProfileUnconfined:
		return true
	default:
		return false
	}
}

type networkSpec struct {
	Mode  string   `yaml:"mode,omitempty"`
	Allow []string `yaml:"allow,omitempty"`
}

func (ns *networkSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		ns.Mode = node.Value
		return nil
	}
	type plain networkSpec
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*ns = networkSpec(p)
	if ns.Mode == "" && len(ns.Allow) > 0 {
		ns.Mode = string(NetworkHost)
	}
	return nil
}

type envSpec struct {
	Inherit bool
	Vars    map[string]*string
}

func (es *envSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("env must be a mapping")
	}
	es.Vars = make(map[string]*string)
	for i := 0; i < len(node.Content); i += 2 {
		kNode := node.Content[i]
		vNode := node.Content[i+1]
		key := kNode.Value
		if key == "inherit" {
			if err := vNode.Decode(&es.Inherit); err != nil {
				return fmt.Errorf("env.inherit must be a boolean: %w", err)
			}
			continue
		}
		if vNode.Tag == "!!null" || (vNode.Kind == yaml.ScalarNode && vNode.Value == "" && vNode.Style == 0) {
			es.Vars[key] = nil
		} else {
			var val string
			if err := vNode.Decode(&val); err != nil {
				return fmt.Errorf("env.%s must be a string or empty: %w", key, err)
			}
			es.Vars[key] = &val
		}
	}
	return nil
}

// mountSpec is the YAML form of a single mount entry.
type mountSpec struct {
	Source      string `yaml:"source,omitempty"`
	Destination string `yaml:"destination"`
	Type        string `yaml:"type,omitempty"`
	Mode        string `yaml:"mode,omitempty"`
}

// DecodeConfigNode decodes the YAML node of a "sandbox:" section into SandboxConfig.
func DecodeConfigNode(node *yaml.Node) (*SandboxConfig, error) {
	if node == nil || node.Kind == 0 {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("sandbox section must be a mapping")
	}
	cfg := &SandboxConfig{
		Enabled:  true,
		Profiles: make(map[string]ProfileBody),
	}
	for i := 0; i < len(node.Content); i += 2 {
		kNode := node.Content[i]
		vNode := node.Content[i+1]
		key := kNode.Value
		if key == "enabled" {
			if err := vNode.Decode(&cfg.Enabled); err != nil {
				return nil, fmt.Errorf("sandbox.enabled must be a boolean: %w", err)
			}
			continue
		}
		if key == "profiles" && vNode.Kind == yaml.MappingNode {
			for j := 0; j < len(vNode.Content); j += 2 {
				pkNode := vNode.Content[j]
				pvNode := vNode.Content[j+1]
				pKey := pkNode.Value
				var pBody ProfileBody
				if pvNode.Kind == yaml.ScalarNode {
					pBody.Base = pvNode.Value
				} else {
					dec := yaml.NewDecoder(strings.NewReader(encodeNode(pvNode)))
					dec.KnownFields(true)
					if err := dec.Decode(&pBody); err != nil {
						return nil, fmt.Errorf("sandbox profile %q: %w", pKey, err)
					}
				}
				cfg.Profiles[pKey] = pBody
			}
			continue
		}
		var body ProfileBody
		if vNode.Kind == yaml.ScalarNode {
			body.Base = vNode.Value
		} else {
			dec := yaml.NewDecoder(strings.NewReader(encodeNode(vNode)))
			dec.KnownFields(true)
			if err := dec.Decode(&body); err != nil {
				return nil, fmt.Errorf("sandbox profile %q: %w", key, err)
			}
		}
		cfg.Profiles[key] = body
	}
	return cfg, nil
}

func encodeNode(node *yaml.Node) string {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	_ = enc.Encode(node)
	_ = enc.Close()
	return buf.String()
}

func decodeProfileYAML(name string, data []byte, origin string) (Profile, error) {
	var body ProfileBody
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&body); err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: %w", name, errWithOrigin(err, origin))
	}
	return profileFromBody(name, body, origin)
}

func decodeProfile(name string, data []byte, origin string) (Profile, error) {
	return decodeProfileYAML(name, data, origin)
}

func profileFromBody(name string, body ProfileBody, origin string) (Profile, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q: getwd: %w", name, err)
	}
	home, _ := os.UserHomeDir()

	if body.Base == DefaultProfileName {
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: base cannot chain to reserved %q", name, fmtOrigin(origin), DefaultProfileName)
	}
	if err := body.Validate(); err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: %w", name, fmtOrigin(origin), err)
	}

	var base Profile
	if body.Base != "" {
		if !isBuiltinProfile(body.Base) {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: base %q must name a built-in profile", name, fmtOrigin(origin), body.Base)
		}
		loadedBase, err := loadProfileResolved(body.Base)
		if err != nil {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: base %q: %w",
				name, fmtOrigin(origin), body.Base, err)
		}
		base = loadedBase
	}

	p := Profile{
		Name:             name,
		Enabled:          true,
		Driver:           base.Driver,
		Image:            base.Image,
		Network:          base.Network,
		NetworkAllow:     append([]string(nil), base.NetworkAllow...),
		MaxMemoryMB:      base.MaxMemoryMB,
		Timeout:          base.Timeout,
		NoExec:           base.NoExec,
		AllowedExecPaths: append([]string(nil), base.AllowedExecPaths...),
		Paths: PathsSpec{
			Read:  append([]string(nil), base.Paths.Read...),
			Write: append([]string(nil), base.Paths.Write...),
		},
		Exec: ExecPolicySpec{
			Allow: append([]string(nil), base.Exec.Allow...),
			Deny:  append([]string(nil), base.Exec.Deny...),
		},
		Calls: CallsPolicySpec{
			Allow: append([]string(nil), base.Calls.Allow...),
			Deny:  append([]string(nil), base.Calls.Deny...),
		},
		Env: EnvPolicySpec{
			Inherit: base.Env.Inherit,
			Vars:    make(map[string]*string),
		},
	}
	maps.Copy(p.Env.Vars, base.Env.Vars)

	if body.Enabled != nil {
		p.Enabled = *body.Enabled
	} else if body.Base != "" {
		p.Enabled = base.Enabled
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

	if body.Network.Mode != "" {
		switch body.Network.Mode {
		case string(NetworkHost):
			p.Network = NetworkHost
		case string(NetworkLoopback):
			p.Network = NetworkLoopback
		case string(NetworkNone):
			p.Network = NetworkNone
		default:
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: unknown network mode %q",
				name, fmtOrigin(origin), body.Network.Mode)
		}
	} else if p.Network == "" {
		if len(body.Network.Allow) > 0 {
			p.Network = NetworkHost
		} else {
			p.Network = NetworkNone
		}
	}
	if len(body.Network.Allow) > 0 {
		p.NetworkAllow = append(p.NetworkAllow, body.Network.Allow...)
	}

	// Paths
	p.Paths.Read = append(p.Paths.Read, body.Paths.Read...)
	p.Paths.Write = append(p.Paths.Write, body.Paths.Write...)

	// Mounts synthesis
	mounts, err := compilePaths(cwd, home, p.Paths, body.Mounts, base.Mounts)
	if err != nil {
		return Profile{}, fmt.Errorf("sandbox: profile %q%s: %w", name, fmtOrigin(origin), err)
	}
	if p.Network == NetworkHost {
		mounts = ResolveNetworkAssetMounts(mounts)
	}
	p.Mounts = mounts

	// Exec & W^X
	p.Exec.Allow = append(p.Exec.Allow, body.Exec.Allow...)
	p.Exec.Deny = append(p.Exec.Deny, body.Exec.Deny...)

	if body.NoExec != nil {
		p.NoExec = *body.NoExec
	} else if len(p.Exec.Allow) == 0 && (len(p.Exec.Deny) == 0 || slices.Contains(p.Exec.Deny, "*")) {
		if body.Base == "" {
			p.NoExec = true
		}
	} else if len(p.Exec.Allow) > 0 {
		if slices.Contains(p.Exec.Allow, "*") {
			p.NoExec = false
		} else {
			p.NoExec = false
			p.AllowedExecPaths = append(p.AllowedExecPaths, ResolveExecutablePaths(p.Exec.Allow, cwd, home)...)
		}
	}

	for i, rawPath := range body.AllowedExecPaths {
		expanded := expandPath(rawPath, cwd, home)
		if expanded == "" || !isHostAbs(expanded) {
			return Profile{}, fmt.Errorf("sandbox: profile %q%s: allowed_exec_paths[%d]: %q must be an absolute path",
				name, fmtOrigin(origin), i, rawPath)
		}
		if !slices.Contains(p.AllowedExecPaths, expanded) {
			p.AllowedExecPaths = append(p.AllowedExecPaths, expanded)
		}
	}

	// Calls
	p.Calls.Allow = append(p.Calls.Allow, body.Calls.Allow...)
	p.Calls.Deny = append(p.Calls.Deny, body.Calls.Deny...)

	// Env
	if body.Env.Inherit {
		p.Env.Inherit = true
	}
	maps.Copy(p.Env.Vars, body.Env.Vars)

	return p, nil
}

func compilePaths(cwd, home string, paths PathsSpec, explicitMounts []mountSpec, baseMounts []Mount) ([]Mount, error) {
	writeSet := make(map[string]bool)
	var writes []string
	for _, p := range paths.Write {
		exp := expandPath(p, cwd, home)
		if exp == "" {
			continue
		}
		clean := filepath.Clean(exp)
		if !writeSet[clean] {
			writeSet[clean] = true
			writes = append(writes, clean)
		}
	}

	var reads []string
	readSet := make(map[string]bool)
	for _, p := range paths.Read {
		exp := expandPath(p, cwd, home)
		if exp == "" {
			continue
		}
		clean := filepath.Clean(exp)
		if writeSet[clean] {
			continue
		}
		if !readSet[clean] {
			readSet[clean] = true
			reads = append(reads, clean)
		}
	}

	type pathEntry struct {
		path string
		mode MountMode
	}
	allPaths := make([]pathEntry, 0, len(writes)+len(reads))
	for _, w := range writes {
		allPaths = append(allPaths, pathEntry{path: w, mode: MountRW})
	}
	for _, r := range reads {
		allPaths = append(allPaths, pathEntry{path: r, mode: MountRO})
	}

	slices.SortFunc(allPaths, func(a, b pathEntry) int {
		if len(a.path) != len(b.path) {
			return len(a.path) - len(b.path)
		}
		return strings.Compare(a.path, b.path)
	})

	var mounts []Mount
	for _, ap := range allPaths {
		mounts = append(mounts, Mount{
			Source:      ap.path,
			Destination: ap.path,
			Type:        MountBind,
			Mode:        ap.mode,
		})
	}

	// Base mounts
	for _, bm := range baseMounts {
		replaced := false
		for i, m := range mounts {
			if m.Destination == bm.Destination {
				mounts[i] = m // child/paths override base
				replaced = true
				break
			}
		}
		if !replaced {
			mounts = append(mounts, bm)
		}
	}

	// Explicit mounts
	for i, m := range explicitMounts {
		decM, err := decodeMount(m, cwd, home)
		if err != nil {
			return nil, fmt.Errorf("mount[%d]: %w", i, err)
		}
		replaced := false
		for j, em := range mounts {
			if em.Destination == decM.Destination {
				mounts[j] = decM
				replaced = true
				break
			}
		}
		if !replaced {
			mounts = append(mounts, decM)
		}
	}

	// Always ensure /tmp tmpfs mount is present if not already declared
	hasTmp := false
	for _, m := range mounts {
		if m.Destination == "/tmp" {
			hasTmp = true
			break
		}
	}
	if !hasTmp {
		mounts = append(mounts, Mount{
			Destination: "/tmp",
			Type:        MountTmpfs,
			Mode:        MountRW,
		})
	}

	return mounts, nil
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

func parseMemoryMB(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	s = strings.TrimSpace(s)
	multiplier := int64(1)
	upper := strings.ToUpper(s)
	if strings.HasSuffix(upper, "GIB") || strings.HasSuffix(upper, "GI") || strings.HasSuffix(upper, "GB") || strings.HasSuffix(upper, "G") {
		multiplier = 1024
		s = strings.TrimRight(s, "gGbBiI")
	} else if strings.HasSuffix(upper, "MIB") || strings.HasSuffix(upper, "MI") || strings.HasSuffix(upper, "MB") || strings.HasSuffix(upper, "M") {
		multiplier = 1
		s = strings.TrimRight(s, "mMbBiI")
	} else if strings.HasSuffix(upper, "KIB") || strings.HasSuffix(upper, "KI") || strings.HasSuffix(upper, "KB") || strings.HasSuffix(upper, "K") {
		multiplier = 0
		s = strings.TrimRight(s, "kKbBiI")
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

func expandPath(p, cwd, home string) string {
	if p == "" {
		return ""
	}
	if p == "." || p == "$CWD" {
		return cwd
	}
	if after, ok := strings.CutPrefix(p, "./"); ok {
		return filepath.Join(cwd, after)
	}
	if rest, ok := strings.CutPrefix(p, "$CWD/"); ok {
		return filepath.Join(cwd, rest)
	}
	if home != "" {
		if p == "$HOME" || p == "~" {
			return home
		}
		if rest, ok := strings.CutPrefix(p, "$HOME/"); ok {
			return filepath.Join(home, rest)
		}
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
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

// CompilePermissions compiles a sandbox.Profile into a *libkite.PermissionConfig.
// CompilePermissions compiles a sandbox.Profile into a *libkite.PermissionConfig.
func (p Profile) CompilePermissions() *libkite.PermissionConfig {
	if !p.Enabled {
		return &libkite.PermissionConfig{
			Allow:   []string{"*.*"},
			Deny:    append(slices.Clone(p.Calls.Deny), libkite.ExecDenyRules(p.Exec.Deny)...),
			Default: libkite.DefaultAllow,
		}
	}

	cfg := &libkite.PermissionConfig{
		Default: libkite.DefaultDeny,
	}

	if len(p.Paths.Read) > 0 {
		cfg.Allow = append(cfg.Allow, libkite.PathReadRules(p.Paths.Read)...)
	}
	if len(p.Paths.Write) > 0 {
		cfg.Allow = append(cfg.Allow, libkite.PathWriteRules(p.Paths.Write)...)
	}

	hasNonTmpMount := false
	for _, m := range p.Mounts {
		if m.Destination == "/tmp" {
			if m.Mode == MountRW {
				cfg.Allow = append(cfg.Allow, libkite.PathWriteRules([]string{"/tmp"})...)
			} else {
				cfg.Allow = append(cfg.Allow, libkite.PathReadRules([]string{"/tmp"})...)
			}
			continue
		}
		hasNonTmpMount = true
		if m.Mode == MountRW {
			cfg.Allow = append(cfg.Allow, libkite.PathWriteRules([]string{m.Destination})...)
			if m.Type == MountBind && m.Source != "" && m.Source != m.Destination {
				cfg.Allow = append(cfg.Allow, libkite.PathWriteRules([]string{m.Source})...)
			}
		} else {
			cfg.Allow = append(cfg.Allow, libkite.PathReadRules([]string{m.Destination})...)
			if m.Type == MountBind && m.Source != "" && m.Source != m.Destination {
				cfg.Allow = append(cfg.Allow, libkite.PathReadRules([]string{m.Source})...)
			}
		}
	}

	if len(p.Paths.Read) > 0 || len(p.Paths.Write) > 0 || hasNonTmpMount {
		if !slices.Contains(p.Calls.Deny, "os.env") {
			cfg.Allow = append(cfg.Allow, "os.env")
		}
		if !slices.Contains(p.Calls.Deny, "io.prompt") {
			cfg.Allow = append(cfg.Allow, "io.prompt")
		}
	}

	if len(p.Exec.Allow) > 0 {
		cfg.Allow = append(cfg.Allow, libkite.ExecAllowRules(p.Exec.Allow)...)
	}
	if len(p.Exec.Deny) > 0 {
		cfg.Deny = append(cfg.Deny, libkite.ExecDenyRules(p.Exec.Deny)...)
	}
	if !p.NoExec && len(p.AllowedExecPaths) > 0 && len(p.Exec.Allow) == 0 {
		cfg.Allow = append(cfg.Allow, libkite.ExecAllowRules(p.AllowedExecPaths)...)
	}

	if (p.Network != "" && p.Network != NetworkNone) || len(p.NetworkAllow) > 0 {
		netRules := libkite.NetworkAllowRules(string(p.Network), p.NetworkAllow)
		cfg.Allow = append(cfg.Allow, netRules...)
	}

	cfg.Allow = append(cfg.Allow, p.Calls.Allow...)
	cfg.Deny = append(cfg.Deny, p.Calls.Deny...)

	return cfg
}

// CompileEnv compiles the execution environment based on the profile's Env policy.
func (p Profile) CompileEnv(hostEnv []string) []string {
	res, _ := p.SynthesizeEnv(hostEnv, nil, false)
	return res
}

// CLIOverrides holds granular CLI flag overrides applied on top of a resolved Profile.
type CLIOverrides struct {
	Enabled   *bool
	Driver    string
	PathRead  []string
	PathWrite []string
	CallAllow []string
	CallDeny  []string
	ExecAllow []string
	ExecDeny  []string
	Net       bool
}

// HasCapabilityOverrides reports whether any additive capability flag was set.
func (o CLIOverrides) HasCapabilityOverrides() bool {
	return len(o.PathRead) > 0 || len(o.PathWrite) > 0 ||
		len(o.CallAllow) > 0 || len(o.CallDeny) > 0 ||
		len(o.ExecAllow) > 0 || len(o.ExecDeny) > 0 ||
		o.Net
}

// splitCommaList expands comma-separated CLI flag entries and trims whitespace.
func splitCommaList(items []string) []string {
	var out []string
	for _, raw := range items {
		for part := range strings.SplitSeq(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// ApplyCLIOverrides applies command-line flag overrides onto the receiving Profile.
func (p *Profile) ApplyCLIOverrides(o CLIOverrides) error {
	if o.Enabled != nil {
		p.Enabled = *o.Enabled
		if !p.Enabled {
			if p.Name == "" {
				p.Name = ProfileUnconfined
			}
			p.NoExec = false
			if p.Network == "" {
				p.Network = NetworkHost
			}
		}
	}

	if o.Driver != "" {
		p.Driver = o.Driver
	}
	if p.Driver == "" {
		p.Driver = DriverDefault
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("sandbox: getwd: %w", err)
	}
	home, _ := os.UserHomeDir()

	if len(o.PathRead) > 0 || len(o.PathWrite) > 0 {
		p.Paths.Read = append(p.Paths.Read, splitCommaList(o.PathRead)...)
		p.Paths.Write = append(p.Paths.Write, splitCommaList(o.PathWrite)...)
		mounts, err := compilePaths(cwd, home, p.Paths, nil, p.Mounts)
		if err != nil {
			return fmt.Errorf("sandbox: cli path overrides: %w", err)
		}
		p.Mounts = mounts
	}

	if o.Net {
		p.Network = NetworkHost
	}
	if p.Network == NetworkHost {
		p.Mounts = ResolveNetworkAssetMounts(p.Mounts)
	}

	execAllow := splitCommaList(o.ExecAllow)
	if len(execAllow) > 0 {
		p.Exec.Allow = append(p.Exec.Allow, execAllow...)
		// Remove wildcard "*" from Exec.Deny inherited from deny-all/sandbox-path
		// so explicit allow entries can take effect.
		filteredDeny := p.Exec.Deny[:0]
		for _, d := range p.Exec.Deny {
			if d != "*" {
				filteredDeny = append(filteredDeny, d)
			}
		}
		p.Exec.Deny = filteredDeny
		p.NoExec = false
		if slices.Contains(p.Exec.Allow, "*") {
			p.AllowedExecPaths = nil
		} else {
			for _, resolved := range ResolveExecutablePaths(execAllow, cwd, home) {
				if !slices.Contains(p.AllowedExecPaths, resolved) {
					p.AllowedExecPaths = append(p.AllowedExecPaths, resolved)
				}
			}
		}
	}

	execDeny := splitCommaList(o.ExecDeny)
	if len(execDeny) > 0 {
		p.Exec.Deny = append(p.Exec.Deny, execDeny...)
		if slices.Contains(execDeny, "*") && len(p.Exec.Allow) == 0 {
			p.NoExec = true
		}
	}

	if len(o.CallAllow) > 0 {
		p.Calls.Allow = append(p.Calls.Allow, splitCommaList(o.CallAllow)...)
	}
	if len(o.CallDeny) > 0 {
		p.Calls.Deny = append(p.Calls.Deny, splitCommaList(o.CallDeny)...)
	}

	return nil
}

// ResolveProfile resolves a profile name against SandboxConfig or built-in ladder profiles.
func ResolveProfile(profileName string, cfg *SandboxConfig) (Profile, error) {
	if profileName == "" || profileName == DefaultProfileName {
		if cfg != nil {
			if body, ok := cfg.Profiles[DefaultProfileName]; ok {
				p, err := profileFromBody(DefaultProfileName, body, "config")
				if err != nil {
					return Profile{}, err
				}
				if err := validateSources(p, "config"); err != nil {
					return Profile{}, err
				}
				if !cfg.Enabled && body.Enabled == nil {
					p.Enabled = false
				}
				return p, nil
			}
		}
		p, err := LoadProfile(DefaultProfileName)
		if err != nil {
			return Profile{}, err
		}
		if cfg != nil && !cfg.Enabled {
			p.Enabled = false
		}
		return p, nil
	}

	if isBuiltinProfile(profileName) {
		return loadProfileResolved(profileName)
	}

	if cfg != nil {
		if body, ok := cfg.Profiles[profileName]; ok {
			p, err := profileFromBody(profileName, body, "config")
			if err != nil {
				return Profile{}, err
			}
			if err := validateSources(p, "config"); err != nil {
				return Profile{}, err
			}
			return p, nil
		}
	}

	return LoadProfile(profileName)
}

// ResolvePermissions resolves a profile name against SandboxConfig or built-in ladder profiles.
func ResolvePermissions(profileName string, cfg *SandboxConfig) (*libkite.PermissionConfig, error) {
	if profileName == "" {
		if cfg != nil {
			if body, ok := cfg.Profiles[DefaultProfileName]; ok {
				p, err := profileFromBody(DefaultProfileName, body, "config")
				if err != nil {
					return nil, err
				}
				return p.CompilePermissions(), nil
			}
			if !cfg.Enabled {
				return libkite.AllowAllPermissions(), nil
			}
		}
		return libkite.DenyAllPermissions(), nil
	}

	switch profileName {
	case ProfileDenyAll:
		return libkite.DenyAllPermissions(), nil
	case ProfileSandboxPath:
		return libkite.SandboxPathPermissions(), nil
	case ProfileSandboxExec:
		return libkite.SandboxExecPermissions(), nil
	case ProfileSandboxNet:
		return libkite.SandboxNetPermissions(), nil
	case ProfileUnconfined:
		return libkite.AllowAllPermissions(), nil
	}

	p, err := ResolveProfile(profileName, cfg)
	if err != nil {
		return nil, fmt.Errorf("unknown profile %q: %w", profileName, err)
	}
	return p.CompilePermissions(), nil
}

func loadProfileResolved(value string) (Profile, error) {
	if value == "" {
		return Profile{}, nil
	}

	switch value {
	case ProfileDenyAll:
		return Profile{
			Name:    ProfileDenyAll,
			Enabled: true,
			Driver:  DriverDefault,
			Paths:   PathsSpec{Read: nil, Write: nil},
			Exec:    ExecPolicySpec{Deny: []string{"*"}},
			NoExec:  true,
			Network: NetworkNone,
			Mounts: []Mount{
				{Destination: "/tmp", Type: MountTmpfs, Mode: MountRW},
			},
		}, nil
	case ProfileSandboxPath:
		cwd, _ := os.Getwd()
		return Profile{
			Name:    value,
			Enabled: true,
			Driver:  DriverDefault,
			Paths:   PathsSpec{Read: []string{cwd}, Write: []string{cwd}},
			Exec:    ExecPolicySpec{Deny: []string{"*"}},
			NoExec:  true,
			Network: NetworkNone,
			Mounts: []Mount{
				{Source: cwd, Destination: cwd, Type: MountBind, Mode: MountRW},
				{Destination: "/tmp", Type: MountTmpfs, Mode: MountRW},
			},
		}, nil
	case ProfileSandboxExec:
		cwd, _ := os.Getwd()
		return Profile{
			Name:    value,
			Enabled: true,
			Driver:  DriverDefault,
			Paths:   PathsSpec{Read: []string{cwd}, Write: []string{cwd}},
			Exec:    ExecPolicySpec{Allow: []string{"*"}},
			NoExec:  false,
			Network: NetworkNone,
			Mounts: []Mount{
				{Source: cwd, Destination: cwd, Type: MountBind, Mode: MountRW},
				{Destination: "/tmp", Type: MountTmpfs, Mode: MountRW},
			},
		}, nil
	case ProfileSandboxNet:
		cwd, _ := os.Getwd()
		return Profile{
			Name:    value,
			Enabled: true,
			Driver:  DriverDefault,
			Paths:   PathsSpec{Read: []string{cwd}, Write: []string{cwd}},
			Exec:    ExecPolicySpec{Allow: []string{"*"}},
			NoExec:  false,
			Network: NetworkHost,
			Mounts: ResolveNetworkAssetMounts([]Mount{
				{Source: cwd, Destination: cwd, Type: MountBind, Mode: MountRW},
				{Destination: "/tmp", Type: MountTmpfs, Mode: MountRW},
			}),
		}, nil
	case ProfileUnconfined:
		return Profile{
			Name:    ProfileUnconfined,
			Enabled: false,
			Driver:  DriverDefault,
			Network: NetworkHost,
			NoExec:  false,
		}, nil
	}

	if value == DefaultProfileName {
		p, err := loadNamed(DefaultProfileName)
		if errors.Is(err, errProfileNotFound) {
			def, err := loadProfileResolved(ProfileDenyAll)
			if err != nil {
				return Profile{}, err
			}
			def.Name = DefaultProfileName
			return def, nil
		}
		return p, err
	}

	return loadNamed(value)
}

func loadNamed(name string) (Profile, error) {
	paths := []string{"config.yaml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, UserConfigFile))
	}

	var defined []string
	var searched []string
	for _, path := range paths {
		cfg, err := readConfigFile(path)
		if err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist) {
				continue
			}
			return Profile{}, err
		}
		if cfg == nil {
			continue
		}
		searched = append(searched, path)
		if body, ok := cfg.Profiles[name]; ok {
			p, err := profileFromBody(name, body, path)
			if err != nil {
				return Profile{}, err
			}
			if err := validateSources(p, path); err != nil {
				return Profile{}, err
			}
			return p, nil
		}
		for n := range cfg.Profiles {
			defined = append(defined, n)
		}
	}

	builtinList := fmt.Sprintf("%s, %s, %s, %s, %s",
		ProfileDenyAll, ProfileSandboxPath, ProfileSandboxExec, ProfileSandboxNet, ProfileUnconfined)

	if len(searched) == 0 {
		return Profile{}, fmt.Errorf(
			"%w: %q (built-ins: %s; no config.yaml found)",
			errProfileNotFound, name, builtinList)
	}
	return Profile{}, fmt.Errorf(
		"%w: %q in %s (defined: %v; built-ins: %s)",
		errProfileNotFound, name, strings.Join(searched, ", "), defined, builtinList)
}

func readConfigFile(path string) (*SandboxConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	return ParseConfigFileData(data, path)
}

// ParseConfigFileData parses configuration bytes, strictly rejecting legacy permissions:.
func ParseConfigFileData(data []byte, origin string) (*SandboxConfig, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("sandbox: parse %s: %w", origin, err)
	}
	if len(root.Content) == 0 {
		return nil, nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, nil
	}

	var sandboxNode *yaml.Node
	for i := 0; i < len(top.Content); i += 2 {
		key := top.Content[i].Value
		val := top.Content[i+1]
		if key == "permissions" {
			if val.Kind != 0 && (val.Kind != yaml.ScalarNode || val.Tag != "!!null") {
				return nil, fmt.Errorf("top-level \"permissions:\" section is no longer supported; define capabilities in \"sandbox:\"")
			}
		}
		if key == "sandbox" {
			sandboxNode = val
		}
	}

	if sandboxNode == nil {
		return nil, nil
	}
	return DecodeConfigNode(sandboxNode)
}
