package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProfile_empty(t *testing.T) {
	p, err := LoadProfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsZero() {
		t.Fatalf("empty value should yield zero profile, got %+v", p)
	}
}

func TestLoadProfile_default(t *testing.T) {
	p, err := LoadProfile(ProfileDefault)
	if err != nil {
		t.Fatalf("LoadProfile(default): %v", err)
	}
	if p.Name != ProfileDefault {
		t.Errorf("Name = %q, want %q", p.Name, ProfileDefault)
	}
	if p.Network != NetworkHost {
		t.Errorf("Network = %q, want %q", p.Network, NetworkHost)
	}

	cwd, _ := os.Getwd()
	wantMounts := map[string]Mount{
		cwd:                  {Source: cwd, Destination: cwd, Type: MountBind, Mode: MountRW},
		"/tmp":               {Destination: "/tmp", Type: MountTmpfs, Mode: MountRW},
		"/etc/ssl/certs":     {Source: "/etc/ssl/certs", Destination: "/etc/ssl/certs", Type: MountBind, Mode: MountRO, Optional: true},
		"/etc/resolv.conf":   {Source: "/etc/resolv.conf", Destination: "/etc/resolv.conf", Type: MountBind, Mode: MountRO, Optional: true},
		"/etc/hosts":         {Source: "/etc/hosts", Destination: "/etc/hosts", Type: MountBind, Mode: MountRO, Optional: true},
		"/etc/nsswitch.conf": {Source: "/etc/nsswitch.conf", Destination: "/etc/nsswitch.conf", Type: MountBind, Mode: MountRO, Optional: true},
	}
	if got, want := len(p.Mounts), len(wantMounts); got != want {
		t.Fatalf("len(Mounts) = %d, want %d (mounts: %+v)", got, want, p.Mounts)
	}
	for _, m := range p.Mounts {
		want, ok := wantMounts[m.Destination]
		if !ok {
			t.Errorf("unexpected mount destination %q", m.Destination)
			continue
		}
		if m != want {
			t.Errorf("mount %q: got %+v, want %+v", m.Destination, m, want)
		}
	}
}

func TestLoadProfile_strict(t *testing.T) {
	p, err := LoadProfile(ProfileStrict)
	if err != nil {
		t.Fatalf("LoadProfile(strict): %v", err)
	}
	if p.Name != ProfileStrict {
		t.Errorf("Name = %q, want %q", p.Name, ProfileStrict)
	}
	if p.Network != NetworkSandboxLoopback {
		t.Errorf("Network = %q, want %q", p.Network, NetworkSandboxLoopback)
	}

	if got := len(p.Mounts); got != 2 {
		t.Fatalf("strict should have exactly 2 mounts ($CWD + /tmp); got %d (%+v)", got, p.Mounts)
	}
	for _, m := range p.Mounts {
		switch m.Destination {
		case "/tmp":
			if m.Type != MountTmpfs || m.Mode != MountRW {
				t.Errorf("/tmp mount = %+v, want tmpfs+rw", m)
			}
		default:
			// Should be the $CWD bind.
			if m.Type != MountBind || m.Mode != MountRW {
				t.Errorf("non-/tmp mount = %+v, want bind+rw", m)
			}
		}
		if strings.HasPrefix(m.Destination, "/etc/") {
			t.Errorf("strict profile should have no /etc/* mount, found %q", m.Destination)
		}
	}
}

func TestLoadProfile_unknownName(t *testing.T) {
	// With no ~/.starkite/config.yaml present, an unknown profile name
	// should surface the "missing security file" friendly error pointing
	// at the built-ins.
	_, err := LoadProfile("nope-this-name-does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown profile, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown profile") && !strings.Contains(msg, "not found") {
		t.Errorf("error message should mention 'unknown profile' or 'not found': %v", err)
	}
}

func TestDecodeProfile_cwdExpansion(t *testing.T) {
	cwd, _ := os.Getwd()
	yaml := `
network: host
mounts:
  - source: $CWD
    destination: $CWD/sub
    mode: rw
`
	p, err := decodeProfile("test", []byte(yaml), "")
	if err != nil {
		t.Fatalf("decodeProfile: %v", err)
	}
	if len(p.Mounts) != 1 {
		t.Fatalf("want 1 mount, got %d", len(p.Mounts))
	}
	m := p.Mounts[0]
	if m.Source != cwd {
		t.Errorf("Source = %q, want %q", m.Source, cwd)
	}
	wantDest := filepath.Join(cwd, "sub")
	if m.Destination != wantDest {
		t.Errorf("Destination = %q, want %q", m.Destination, wantDest)
	}
}

func TestDecodeProfile_validation(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "missing network",
			yaml:    "mounts: []",
			wantErr: "network mode is required",
		},
		{
			name:    "unknown network",
			yaml:    "network: bridge",
			wantErr: "unknown network mode",
		},
		{
			name: "tmpfs with source",
			yaml: `
network: host
mounts:
  - source: /var/tmp
    destination: /tmp
    type: tmpfs
`,
			wantErr: "tmpfs mount must not specify source",
		},
		{
			name: "tmpfs with optional",
			yaml: `
network: host
mounts:
  - destination: /tmp
    type: tmpfs
    optional: true
`,
			wantErr: "tmpfs mount cannot be optional",
		},
		{
			name: "bind without source",
			yaml: `
network: host
mounts:
  - destination: /etc/foo
`,
			wantErr: "bind mount requires source",
		},
		{
			name: "unknown mount type",
			yaml: `
network: host
mounts:
  - source: /etc/foo
    destination: /etc/foo
    type: overlay
`,
			wantErr: "unknown mount type",
		},
		{
			name: "unknown mode",
			yaml: `
network: host
mounts:
  - source: /etc/foo
    destination: /etc/foo
    mode: append
`,
			wantErr: "unknown mode",
		},
		{
			name: "destination not absolute",
			yaml: `
network: host
mounts:
  - source: /etc/foo
    destination: foo
`,
			wantErr: "must be an absolute path",
		},
		{
			name: "source not absolute",
			yaml: `
network: host
mounts:
  - source: etc/foo
    destination: /etc/foo
`,
			wantErr: "must be an absolute path",
		},
		{
			name: "unknown top-level field",
			yaml: `
network: host
unexpected: 42
`,
			wantErr: "field unexpected not found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeProfile("test", []byte(tc.yaml), "")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %v does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadProfile_fromNamed_userConfig(t *testing.T) {
	// Stage a fake home dir with a config.yaml; redirect HOME so the
	// loader picks it up.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".starkite")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `
config:
  environment: dev
permissions:
  ci:
    allow: [fs.read]
sandbox:
  hometest:
    network: host
    mounts: [{destination: /tmp, type: tmpfs}]
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(strings.TrimLeft(yaml, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadProfile("hometest")
	if err != nil {
		t.Fatalf("LoadProfile(hometest): %v", err)
	}
	if p.Name != "hometest" || p.Network != NetworkHost {
		t.Errorf("got %+v, want Name=hometest Network=host", p)
	}

	// Unknown name in same file → error mentions defined names.
	_, err = LoadProfile("missing")
	if err == nil {
		t.Fatal("expected error for unknown profile in config.yaml")
	}
	if !strings.Contains(err.Error(), "hometest") {
		t.Errorf("error should list defined profiles: %v", err)
	}
}

func TestDecodeProfile_defaults(t *testing.T) {
	yaml := `
network: host
mounts:
  - source: /etc/foo
    destination: /etc/foo
  - destination: /tmp
    type: tmpfs
`
	p, err := decodeProfile("test", []byte(yaml), "")
	if err != nil {
		t.Fatalf("decodeProfile: %v", err)
	}
	if p.Mounts[0].Type != MountBind {
		t.Errorf("default Type for omitted = %q, want bind", p.Mounts[0].Type)
	}
	if p.Mounts[0].Mode != MountRO {
		t.Errorf("default Mode for bind = %q, want ro", p.Mounts[0].Mode)
	}
	if p.Mounts[1].Mode != MountRW {
		t.Errorf("default Mode for tmpfs = %q, want rw", p.Mounts[1].Mode)
	}
}

func TestLoadProfile_RemovedSyntaxesError(t *testing.T) {
	// File paths and fragments are no longer accepted; they resolve as
	// unknown profile names.
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range []string{"./myprofile.yaml", "profiles.yaml#k8s", "/abs/p.yml"} {
		if _, err := LoadProfile(v); err == nil {
			t.Errorf("LoadProfile(%q) should error: not a profile name", v)
		}
	}
}

func TestLoadProfile_fromNamed_projectLocalConfig(t *testing.T) {
	// A profile defined in ./config.yaml resolves, and wins over one with the
	// same name in ~/.starkite/config.yaml.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".starkite")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	homeYAML := `
sandbox:
  shared:
    network: sandbox-loopback
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(strings.TrimLeft(homeYAML, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	work := t.TempDir()
	localYAML := `
sandbox:
  shared:
    network: host
  localonly:
    network: host
`
	if err := os.WriteFile(filepath.Join(work, "config.yaml"), []byte(strings.TrimLeft(localYAML, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)

	p, err := LoadProfile("shared")
	if err != nil {
		t.Fatalf("LoadProfile(shared): %v", err)
	}
	if p.Network != NetworkHost {
		t.Errorf("project-local profile should win: Network = %v, want host", p.Network)
	}

	if _, err := LoadProfile("localonly"); err != nil {
		t.Errorf("LoadProfile(localonly): %v", err)
	}

	// Unknown name lists profiles from both files.
	_, err = LoadProfile("missing")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if !strings.Contains(err.Error(), "shared") {
		t.Errorf("error should list defined profiles: %v", err)
	}
}
