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

// mountByDest indexes a profile's mounts by destination.
func mountByDest(p Profile) map[string]Mount {
	m := make(map[string]Mount, len(p.Mounts))
	for _, mt := range p.Mounts {
		m[mt.Destination] = mt
	}
	return m
}

func TestLoadProfile_opaque(t *testing.T) {
	p, err := LoadProfile(ProfileOpaque)
	if err != nil {
		t.Fatalf("LoadProfile(opaque): %v", err)
	}
	if p.Name != ProfileOpaque {
		t.Errorf("Name = %q, want %q", p.Name, ProfileOpaque)
	}
	if p.Network != NetworkLoopback {
		t.Errorf("Network = %q, want %q", p.Network, NetworkLoopback)
	}

	cwd, _ := os.Getwd()
	if got := len(p.Mounts); got != 2 {
		t.Fatalf("opaque should have exactly 2 mounts ($CWD + /tmp); got %d (%+v)", got, p.Mounts)
	}
	m := mountByDest(p)
	if cw, ok := m[cwd]; !ok || cw.Type != MountBind || cw.Mode != MountRW {
		t.Errorf("$CWD mount = %+v, want bind+rw at %s", m[cwd], cwd)
	}
	if tmp, ok := m["/tmp"]; !ok || tmp.Type != MountTmpfs || tmp.Mode != MountRW {
		t.Errorf("/tmp mount = %+v, want tmpfs+rw", m["/tmp"])
	}
}

func TestLoadProfile_netAccess(t *testing.T) {
	p, err := LoadProfile(ProfileNetAccess)
	if err != nil {
		t.Fatalf("LoadProfile(net-access): %v", err)
	}
	if p.Network != NetworkHost {
		t.Errorf("Network = %q, want %q", p.Network, NetworkHost)
	}

	cwd, _ := os.Getwd()
	m := mountByDest(p)
	if _, ok := m[cwd]; !ok {
		t.Errorf("net-access must mount $CWD")
	}
	if _, ok := m["/tmp"]; !ok {
		t.Errorf("net-access must mount /tmp")
	}

	// Every host-support file present on this host must be mounted ro;
	// absent ones are filtered at load (built-in portability).
	for _, sf := range []string{"/etc/ssl/certs", "/etc/resolv.conf", "/etc/hosts", "/etc/nsswitch.conf"} {
		_, hostErr := os.Stat(sf)
		mt, mounted := m[sf]
		if hostErr == nil && (!mounted || mt.Mode != MountRO) {
			t.Errorf("support file %s exists on host but is not mounted ro (got %+v)", sf, mt)
		}
		if hostErr != nil && mounted {
			t.Errorf("support file %s absent on host but mounted: %+v", sf, mt)
		}
	}

	// Nothing beyond opaque + support files: no $HOME, no host binaries.
	home, _ := os.UserHomeDir()
	for _, banned := range []string{home, "/usr", "/bin", "/lib"} {
		if _, ok := m[banned]; ok {
			t.Errorf("net-access must not mount %s", banned)
		}
	}
}

func TestLoadProfile_host(t *testing.T) {
	p, err := LoadProfile(ProfileHost)
	if err != nil {
		t.Fatalf("LoadProfile(host): %v", err)
	}
	if p.Network != NetworkHost {
		t.Errorf("Network = %q, want %q", p.Network, NetworkHost)
	}

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	m := mountByDest(p)

	// $HOME and /usr exist everywhere this test runs; both must be ro.
	for _, ro := range []string{home, "/usr"} {
		mt, ok := m[ro]
		if !ok {
			t.Fatalf("host rung must mount %s", ro)
		}
		if mt.Mode != MountRO {
			t.Errorf("%s mount mode = %q, want ro (a writable $HOME is a sandbox escape)", ro, mt.Mode)
		}
	}

	// The only rw mounts are $CWD and /tmp.
	for _, mt := range p.Mounts {
		if mt.Mode == MountRW && mt.Destination != cwd && mt.Destination != "/tmp" {
			t.Errorf("unexpected rw mount %+v — host rung writes only the tree", mt)
		}
	}
}

// TestLadderSubsets asserts each rung is a superset of the one below:
// every mount of the lower rung is present identically in the higher one.
func TestLadderSubsets(t *testing.T) {
	opaque, err := LoadProfile(ProfileOpaque)
	if err != nil {
		t.Fatal(err)
	}
	net, err := LoadProfile(ProfileNetAccess)
	if err != nil {
		t.Fatal(err)
	}
	host, err := LoadProfile(ProfileHost)
	if err != nil {
		t.Fatal(err)
	}

	assertSuperset := func(lo, hi Profile) {
		t.Helper()
		him := mountByDest(hi)
		for _, m := range lo.Mounts {
			hm, ok := him[m.Destination]
			if !ok {
				t.Errorf("%s ⊄ %s: mount %q missing in %s", lo.Name, hi.Name, m.Destination, hi.Name)
				continue
			}
			if hm != m {
				t.Errorf("%s ⊄ %s: mount %q differs: %+v vs %+v", lo.Name, hi.Name, m.Destination, m, hm)
			}
		}
	}
	assertSuperset(opaque, net)
	assertSuperset(net, host)

	if opaque.Network != NetworkLoopback || net.Network != NetworkHost || host.Network != NetworkHost {
		t.Errorf("network ladder wrong: %s/%s/%s", opaque.Network, net.Network, host.Network)
	}
}

func TestLoadProfile_defaultReserved(t *testing.T) {
	t.Run("no config: bare default falls back to opaque", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(t.TempDir())

		p, err := LoadProfile(DefaultProfileName)
		if err != nil {
			t.Fatalf("LoadProfile(default): %v", err)
		}
		if p.Name != ProfileOpaque || p.Network != NetworkLoopback {
			t.Errorf("fallback = %q/%q, want opaque/loopback", p.Name, p.Network)
		}
	})

	t.Run("config default as alias", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		work := t.TempDir()
		writeFile(t, filepath.Join(work, "config.yaml"), "sandbox:\n  default: net-access\n")
		t.Chdir(work)

		p, err := LoadProfile(DefaultProfileName)
		if err != nil {
			t.Fatalf("LoadProfile(default): %v", err)
		}
		if p.Name != DefaultProfileName || p.Network != NetworkHost {
			t.Errorf("default alias = %q/%q, want default/host", p.Name, p.Network)
		}
	})

	t.Run("config default as full spec", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		work := t.TempDir()
		writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  default:
    network: loopback
    mounts:
      - destination: /tmp
        type: tmpfs
`)
		t.Chdir(work)

		p, err := LoadProfile(DefaultProfileName)
		if err != nil {
			t.Fatalf("LoadProfile(default): %v", err)
		}
		if p.Network != NetworkLoopback || len(p.Mounts) != 1 {
			t.Errorf("default spec = %+v", p)
		}
	})
}

func TestProfileAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  sealed: opaque
  bad: no-such-rung
  chained: default
`)
	t.Chdir(work)

	t.Run("alias expands to its rung, keeps the selected name", func(t *testing.T) {
		p, err := LoadProfile("sealed")
		if err != nil {
			t.Fatalf("LoadProfile(sealed): %v", err)
		}
		if p.Name != "sealed" || p.Network != NetworkLoopback {
			t.Errorf("alias = %q/%q, want sealed/loopback", p.Name, p.Network)
		}
	})

	t.Run("alias to unknown rung errors", func(t *testing.T) {
		if _, err := LoadProfile("bad"); err == nil || !strings.Contains(err.Error(), "must name a built-in") {
			t.Errorf("alias to non-rung should error, got %v", err)
		}
	})

	t.Run("alias to reserved default errors", func(t *testing.T) {
		if _, err := LoadProfile("chained"); err == nil {
			t.Error("alias chains are not allowed")
		}
	})
}

func TestBuiltinShadowing(t *testing.T) {
	// A config profile named after a rung is ignored: built-ins win.
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  opaque:
    network: host
`)
	t.Chdir(work)

	p, err := LoadProfile(ProfileOpaque)
	if err != nil {
		t.Fatalf("LoadProfile(opaque): %v", err)
	}
	if p.Network != NetworkLoopback {
		t.Errorf("built-in opaque shadowed by config: Network = %q", p.Network)
	}
}

func TestUserProfile_missingSourceErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  typo:
    network: host
    mounts:
      - source: /nonexistent-path-for-test-xyz
        destination: /data
`)
	t.Chdir(work)

	_, err := LoadProfile("typo")
	if err == nil || !strings.Contains(err.Error(), "does not exist on the host") {
		t.Errorf("missing bind source should fail loudly, got %v", err)
	}
}

func TestOldRungNamesError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())

	if _, err := LoadProfile("strict"); err == nil {
		t.Error("removed name \"strict\" should no longer resolve")
	}
}

func TestDecodeProfile_pathExpansion(t *testing.T) {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	yaml := `
network: host
mounts:
  - source: $CWD
    destination: $CWD/sub
    mode: rw
  - source: $HOME/.kube
    destination: /etc/kube
`
	p, err := decodeProfile("test", []byte(yaml), "")
	if err != nil {
		t.Fatalf("decodeProfile: %v", err)
	}
	if p.Mounts[0].Source != cwd || p.Mounts[0].Destination != filepath.Join(cwd, "sub") {
		t.Errorf("$CWD expansion wrong: %+v", p.Mounts[0])
	}
	if p.Mounts[1].Source != filepath.Join(home, ".kube") {
		t.Errorf("$HOME expansion wrong: %+v", p.Mounts[1])
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
			name:    "removed network value sandbox-loopback",
			yaml:    "network: sandbox-loopback",
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
			name: "removed optional field rejected",
			yaml: `
network: host
mounts:
  - source: /etc/hosts
    destination: /etc/hosts
    optional: true
`,
			wantErr: "field optional not found",
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
	t.Chdir(t.TempDir())
	for _, v := range []string{"./myprofile.yaml", "profiles.yaml#k8s", "/abs/p.yml"} {
		if _, err := LoadProfile(v); err == nil {
			t.Errorf("LoadProfile(%q) should error: not a profile name", v)
		}
	}
}

func TestLoadProfile_fromNamed_userConfig(t *testing.T) {
	// A profile in ~/.starkite/config.yaml resolves; the file's other
	// sections (config:, permissions:) are tolerated.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	cfgDir := filepath.Join(home, ".starkite")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cfgDir, "config.yaml"), `
config:
  environment: dev
permissions:
  ci:
    allow: [fs.read]
sandbox:
  hometest:
    network: host
    mounts: [{destination: /tmp, type: tmpfs}]
`)

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

func TestLoadProfile_fromNamed_projectLocalConfig(t *testing.T) {
	// A profile defined in ./config.yaml resolves, and wins over one with the
	// same name in ~/.starkite/config.yaml.
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".starkite")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cfgDir, "config.yaml"), `
sandbox:
  shared:
    network: loopback
`)

	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  shared:
    network: host
  localonly:
    network: host
`)
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}
