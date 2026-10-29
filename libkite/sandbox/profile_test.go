package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
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

func TestLoadProfile_denyAll(t *testing.T) {
	p, err := LoadProfile(ProfileDenyAll)
	if err != nil {
		t.Fatalf("LoadProfile(deny-all): %v", err)
	}
	if p.Name != ProfileDenyAll {
		t.Errorf("Name = %q, want %q", p.Name, ProfileDenyAll)
	}
	if p.Network != NetworkNone {
		t.Errorf("Network = %q, want %q", p.Network, NetworkNone)
	}
	if !p.NoExec {
		t.Errorf("NoExec = false, want true")
	}
	if len(p.Exec.Deny) != 1 || p.Exec.Deny[0] != "*" {
		t.Errorf("Exec.Deny = %v, want [*]", p.Exec.Deny)
	}
	if len(p.Paths.Read) != 0 || len(p.Paths.Write) != 0 {
		t.Errorf("Paths should be empty, got read=%v, write=%v", p.Paths.Read, p.Paths.Write)
	}
	if len(p.Mounts) != 1 || p.Mounts[0].Destination != "/tmp" || p.Mounts[0].Mode != MountRW {
		t.Errorf("Mounts = %+v, want only /tmp RW tmpfs mount", p.Mounts)
	}
}

func TestLoadProfile_capabilityLadder(t *testing.T) {
	t.Run("deny-all", func(t *testing.T) {
		p, err := LoadProfile(ProfileDenyAll)
		if err != nil {
			t.Fatalf("LoadProfile(deny-all): %v", err)
		}
		if p.Name != ProfileDenyAll || p.Network != NetworkNone || !p.NoExec {
			t.Errorf("deny-all mismatch: %+v", p)
		}
	})

	t.Run("sandbox-path", func(t *testing.T) {
		p, err := LoadProfile(ProfileSandboxPath)
		if err != nil {
			t.Fatalf("LoadProfile(sandbox-path): %v", err)
		}
		if p.Name != ProfileSandboxPath || p.Network != NetworkNone || !p.NoExec {
			t.Errorf("sandbox-path mismatch: %+v", p)
		}
		if len(p.Paths.Read) != 1 || len(p.Paths.Write) != 1 {
			t.Errorf("expected cwd read/write paths, got read=%v write=%v", p.Paths.Read, p.Paths.Write)
		}
	})

	t.Run("sandbox-exec", func(t *testing.T) {
		p, err := LoadProfile(ProfileSandboxExec)
		if err != nil {
			t.Fatalf("LoadProfile(sandbox-exec): %v", err)
		}
		if p.Name != ProfileSandboxExec || p.Network != NetworkNone || p.NoExec {
			t.Errorf("sandbox-exec mismatch: %+v", p)
		}
		if !slices.Contains(p.Exec.Allow, "*") {
			t.Errorf("expected * in Exec.Allow, got: %v", p.Exec.Allow)
		}
	})

	t.Run("sandbox-net", func(t *testing.T) {
		p, err := LoadProfile(ProfileSandboxNet)
		if err != nil {
			t.Fatalf("LoadProfile(sandbox-net): %v", err)
		}
		if p.Name != ProfileSandboxNet || p.Network != NetworkHost || p.NoExec {
			t.Errorf("sandbox-net mismatch: %+v", p)
		}
	})

	t.Run("unconfined", func(t *testing.T) {
		p, err := LoadProfile(ProfileUnconfined)
		if err != nil {
			t.Fatalf("LoadProfile(unconfined): %v", err)
		}
		if p.Name != ProfileUnconfined || p.Enabled {
			t.Errorf("unconfined must have Enabled=false: %+v", p)
		}
	})
}

func TestApplyCLIOverrides(t *testing.T) {
	p, err := LoadProfile(ProfileDenyAll)
	if err != nil {
		t.Fatalf("LoadProfile(deny-all): %v", err)
	}

	enabledVal := true
	overrides := CLIOverrides{
		Enabled:      &enabledVal,
		Driver:       "auto",
		PathRead:     []string{"/tmp/read1", "/tmp/read2"},
		PathWrite:    []string{"/tmp/write1"},
		ExecAllow:    []string{"git", "go"},
		ExecDeny:     []string{"rm"},
		CallAllow:    []string{"os.env"},
		CallDeny:     []string{"os.exit"},
		NetworkAllow: []string{"*"},
	}

	if err := p.ApplyCLIOverrides(overrides); err != nil {
		t.Fatalf("ApplyCLIOverrides: %v", err)
	}

	if p.Network != NetworkHost {
		t.Errorf("Network = %v, want host", p.Network)
	}
	if p.NoExec {
		t.Error("NoExec = true, want false after ExecAllow")
	}
	if slices.Contains(p.Exec.Deny, "*") {
		t.Error("Exec.Deny should not contain '*' after explicit ExecAllow")
	}
	if !slices.Contains(p.Exec.Allow, "git") {
		t.Errorf("Exec.Allow missing git: %v", p.Exec.Allow)
	}
	if !slices.Contains(p.Exec.Deny, "rm") {
		t.Errorf("Exec.Deny missing rm: %v", p.Exec.Deny)
	}
	if !slices.Contains(p.Calls.Allow, "os.env") {
		t.Errorf("Calls.Allow missing os.env: %v", p.Calls.Allow)
	}
	if !slices.Contains(p.Calls.Deny, "os.exit") {
		t.Errorf("Calls.Deny missing os.exit: %v", p.Calls.Deny)
	}

	// Verify unconfined override
	unconfinedFalse := false
	if err := p.ApplyCLIOverrides(CLIOverrides{Enabled: &unconfinedFalse}); err != nil {
		t.Fatalf("ApplyCLIOverrides unconfined: %v", err)
	}
	if p.Enabled {
		t.Error("p.Enabled should be false after unconfined override")
	}
}

func TestLoadProfile_defaultReserved(t *testing.T) {
	t.Run("no config: bare default falls back to deny-all", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Chdir(t.TempDir())

		p, err := LoadProfile(DefaultProfileName)
		if err != nil {
			t.Fatalf("LoadProfile(default): %v", err)
		}
		if p.Name != DefaultProfileName || p.Network != NetworkNone || !p.NoExec {
			t.Errorf("fallback = %+v, want Name=default Network=none NoExec=true", p)
		}
	})

	t.Run("config default as alias", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		work := t.TempDir()
		writeFile(t, filepath.Join(work, "config.yaml"), "sandbox:\n  default: sandbox-net\n")
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
		t.Setenv("USERPROFILE", home)
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
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  sealed: sandbox-path
  bad: no-such-rung
  chained: default
`)
	t.Chdir(work)

	t.Run("alias expands to its rung, keeps the selected name", func(t *testing.T) {
		p, err := LoadProfile("sealed")
		if err != nil {
			t.Fatalf("LoadProfile(sealed): %v", err)
		}
		if p.Name != "sealed" || p.Network != NetworkNone || !p.NoExec {
			t.Errorf("alias = %q/%q, want sealed/none", p.Name, p.Network)
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
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  sandbox-path:
    network: host
`)
	t.Chdir(work)

	p, err := LoadProfile(ProfileSandboxPath)
	if err != nil {
		t.Fatalf("LoadProfile(sandbox-path): %v", err)
	}
	if p.Network != NetworkNone {
		t.Errorf("built-in sandbox-path shadowed by config: Network = %q", p.Network)
	}
}

func TestUserProfile_missingSourceErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())

	for _, name := range []string{"strict", "opaque", "net-access", "host", "allow-fs", "allow-local", "allow-net", "allow-all"} {
		if _, err := LoadProfile(name); err == nil {
			t.Errorf("removed name %q should no longer resolve", name)
		}
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
	t.Setenv("USERPROFILE", home)
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
	t.Setenv("USERPROFILE", home)
	t.Chdir(t.TempDir())
	cfgDir := filepath.Join(home, ".starkite")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cfgDir, "config.yaml"), `
config:
  environment: dev
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
	t.Setenv("USERPROFILE", home)
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

func TestComposeProfile_base(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	cache := filepath.Join(work, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  dev:
    base: sandbox-path
    mounts:
      - source: $CWD/cache
        destination: /cache
        mode: rw
  widened:
    base: sandbox-path
    network: host
  scalar: sandbox-path
  badbase:
    base: nope
  badnet:
    base: sandbox-path
    network: bridge
  missing:
    base: sandbox-path
    mounts:
      - source: /nonexistent-base-add-xyz
        destination: /x
`)
	t.Chdir(work)

	t.Run("base + appended mount", func(t *testing.T) {
		p, err := LoadProfile("dev")
		if err != nil {
			t.Fatalf("LoadProfile(dev): %v", err)
		}
		if p.Network != NetworkNone {
			t.Errorf("network should inherit base (none), got %q", p.Network)
		}
		baseProf, _ := LoadProfile(ProfileSandboxPath)
		if len(p.Mounts) != len(baseProf.Mounts)+1 {
			t.Fatalf("mounts = %d, want base+1 (%+v)", len(p.Mounts), p.Mounts)
		}
		m := mountByDest(p)
		add, ok := m["/cache"]
		if !ok || add.Mode != MountRW || add.Source != filepath.Join(work, "cache") {
			t.Errorf("appended mount = %+v", add)
		}
	})

	t.Run("network override on a base", func(t *testing.T) {
		p, err := LoadProfile("widened")
		if err != nil {
			t.Fatalf("LoadProfile(widened): %v", err)
		}
		if p.Network != NetworkHost {
			t.Errorf("network = %q, want host override", p.Network)
		}
	})

	t.Run("scalar shorthand ≡ bare base", func(t *testing.T) {
		p, err := LoadProfile("scalar")
		if err != nil {
			t.Fatalf("LoadProfile(scalar): %v", err)
		}
		baseProf, _ := LoadProfile(ProfileSandboxPath)
		if p.Network != baseProf.Network || len(p.Mounts) != len(baseProf.Mounts) {
			t.Errorf("scalar shorthand differs from rung: %+v", p)
		}
	})

	t.Run("override by destination replaces the base mount", func(t *testing.T) {
		// Narrowing/widening a base mount in place: remount $CWD ro over
		// sandbox-path's rw.
		cwd, _ := os.Getwd()
		writeFile(t, filepath.Join(work, "config2.yaml"), "")
		writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  rotree:
    base: sandbox-path
    mounts:
      - source: $CWD
        destination: $CWD
        mode: ro
`)
		p, err := LoadProfile("rotree")
		if err != nil {
			t.Fatalf("LoadProfile(rotree): %v", err)
		}
		baseProf, _ := LoadProfile(ProfileSandboxPath)
		if len(p.Mounts) != len(baseProf.Mounts) {
			t.Fatalf("override should replace, not append: %d mounts", len(p.Mounts))
		}
		if m := mountByDest(p)[cwd]; m.Mode != MountRO {
			t.Errorf("$CWD override = %+v, want ro", m)
		}
	})
}

func TestComposeProfile_errors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  badbase:
    base: nope
  badnet:
    base: sandbox-path
    network: bridge
  missing:
    base: sandbox-path
    mounts:
      - source: /nonexistent-base-add-xyz
        destination: /x
  chain:
    base: default
`)
	t.Chdir(work)

	t.Run("unknown base errors", func(t *testing.T) {
		if _, err := LoadProfile("badbase"); err == nil || !strings.Contains(err.Error(), "must name a built-in") {
			t.Errorf("unknown base should error, got %v", err)
		}
	})

	t.Run("bad network on a base errors", func(t *testing.T) {
		if _, err := LoadProfile("badnet"); err == nil || !strings.Contains(err.Error(), "unknown network mode") {
			t.Errorf("bad network should error, got %v", err)
		}
	})

	t.Run("missing source in additions errors loudly", func(t *testing.T) {
		if _, err := LoadProfile("missing"); err == nil || !strings.Contains(err.Error(), "does not exist on the host") {
			t.Errorf("missing addition source should error, got %v", err)
		}
	})

	t.Run("base cannot chain to reserved default", func(t *testing.T) {
		if _, err := LoadProfile("chain"); err == nil {
			t.Error("base: default must error (rungs only)")
		}
	})
}

func TestDecodeProfile_DriverAndConstraints(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  podman-builder:
    driver: podman
    image: golang:1.24-alpine
    network: none
    max_memory: 1GB
    timeout: 30s
    mounts:
      - destination: /tmp
        type: tmpfs
`)
	t.Chdir(work)

	p, err := LoadProfile("podman-builder")
	if err != nil {
		t.Fatalf("LoadProfile(podman-builder): %v", err)
	}

	if p.Driver != DriverPodman {
		t.Errorf("p.Driver = %q, want %q", p.Driver, DriverPodman)
	}
	if p.Image != "golang:1.24-alpine" {
		t.Errorf("p.Image = %q, want 'golang:1.24-alpine'", p.Image)
	}
	if p.Network != NetworkNone {
		t.Errorf("p.Network = %q, want %q", p.Network, NetworkNone)
	}
	if p.MaxMemoryMB != 1024 {
		t.Errorf("p.MaxMemoryMB = %d, want 1024", p.MaxMemoryMB)
	}
	if p.Timeout != 30*time.Second {
		t.Errorf("p.Timeout = %v, want 30s", p.Timeout)
	}
}

func TestComposeProfile_DriverInheritance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  custom-seatbelt:
    base: sandbox-net
    driver: seatbelt
    max_memory: 512MB
`)
	t.Chdir(work)

	p, err := LoadProfile("custom-seatbelt")
	if err != nil {
		t.Fatalf("LoadProfile(custom-seatbelt): %v", err)
	}

	if p.Driver != DriverSeatbelt {
		t.Errorf("p.Driver = %q, want %q", p.Driver, DriverSeatbelt)
	}
	if p.MaxMemoryMB != 512 {
		t.Errorf("p.MaxMemoryMB = %d, want 512", p.MaxMemoryMB)
	}
	if p.Network != NetworkHost {
		t.Errorf("p.Network = %v, want host (inherited from base)", p.Network)
	}
}

func TestDecodeProfile_NoExecAndAllowedExecPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "config.yaml"), `
sandbox:
  locked:
    base: sandbox-exec
    no_exec: true
  allowlisted:
    network: none
    allowed_exec_paths:
      - /usr/bin/git
      - $CWD/bin/tool
  bad-exec-path:
    network: none
    allowed_exec_paths:
      - relative/bin
`)
	t.Chdir(work)
	cwd, _ := os.Getwd()

	pLocked, err := LoadProfile("locked")
	if err != nil {
		t.Fatalf("LoadProfile(locked): %v", err)
	}
	if !pLocked.NoExec {
		t.Errorf("expected NoExec=true on locked profile")
	}

	pAllow, err := LoadProfile("allowlisted")
	if err != nil {
		t.Fatalf("LoadProfile(allowlisted): %v", err)
	}
	if len(pAllow.AllowedExecPaths) != 2 {
		t.Fatalf("len(AllowedExecPaths) = %d, want 2", len(pAllow.AllowedExecPaths))
	}
	if pAllow.AllowedExecPaths[0] != "/usr/bin/git" {
		t.Errorf("AllowedExecPaths[0] = %q, want /usr/bin/git", pAllow.AllowedExecPaths[0])
	}
	wantTool := filepath.Join(cwd, "bin", "tool")
	if pAllow.AllowedExecPaths[1] != wantTool {
		t.Errorf("AllowedExecPaths[1] = %q, want %q", pAllow.AllowedExecPaths[1], wantTool)
	}

	if _, err := LoadProfile("bad-exec-path"); err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Errorf("expected relative allowed_exec_paths to fail validation, got: %v", err)
	}
}

func TestPermissionsSectionRejected(t *testing.T) {
	work := t.TempDir()
	cfgPath := filepath.Join(work, "config.yaml")
	writeFile(t, cfgPath, `
permissions:
  legacy:
    allow: [fs.read]
sandbox:
  enabled: true
`)
	_, err := readConfigFile(cfgPath)
	if err == nil {
		t.Fatal("expected rejection of legacy permissions: section, got nil")
	}
	want := `top-level "permissions:" section is no longer supported; define capabilities in "sandbox:"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want substring %q", err.Error(), want)
	}
}

func TestUnifiedSandboxSchema(t *testing.T) {
	work := t.TempDir()
	cfgPath := filepath.Join(work, "config.yaml")
	writeFile(t, cfgPath, `
sandbox:
  enabled: true

  default:
    paths:
      read: ["."]
      write: []
    exec:
      deny: ["*"]
    network: none
    env:
      inherit: false

  ci-builder:
    driver: auto
    max_memory: 2Gi
    timeout: 15m
    paths:
      read:
        - .
        - /etc/ssl/certs
      write:
        - ./dist
        - /tmp
    exec:
      allow:
        - git
        - go
        - docker
      deny:
        - rm
        - dd
    network:
      allow:
        - "*.golang.org:443"
        - "ghcr.io:443"
    calls:
      allow:
        - "os.*"
      deny:
        - "os.exit"
    env:
      inherit: false
      GITHUB_TOKEN:
      GOPROXY: "https://proxy.golang.org,direct"

  unconfined:
    enabled: false
`)
	t.Chdir(work)
	cfg, err := readConfigFile(cfgPath)
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}
	if !cfg.Enabled {
		t.Error("expected sandbox.enabled = true")
	}
	if len(cfg.Profiles) != 3 {
		t.Fatalf("got %d profiles, want 3", len(cfg.Profiles))
	}

	// Verify unconfined
	pUnconfined, err := profileFromBody("unconfined", cfg.Profiles["unconfined"], cfgPath)
	if err != nil {
		t.Fatalf("profileFromBody(unconfined): %v", err)
	}
	if pUnconfined.Enabled {
		t.Error("unconfined should have Enabled=false")
	}
	unconfinedPerms := pUnconfined.CompilePermissions()
	if unconfinedPerms.Default != libkite.DefaultAllow {
		t.Errorf("unconfined default = %v, want DefaultAllow", unconfinedPerms.Default)
	}

	// Verify ci-builder
	pCI, err := profileFromBody("ci-builder", cfg.Profiles["ci-builder"], cfgPath)
	if err != nil {
		t.Fatalf("profileFromBody(ci-builder): %v", err)
	}
	if pCI.MaxMemoryMB != 2048 {
		t.Errorf("MaxMemoryMB = %d, want 2048", pCI.MaxMemoryMB)
	}
	if pCI.Timeout != 15*time.Minute {
		t.Errorf("Timeout = %v, want 15m", pCI.Timeout)
	}
	if pCI.Network != NetworkHost {
		t.Errorf("Network = %v, want host", pCI.Network)
	}
	if len(pCI.NetworkAllow) != 2 {
		t.Errorf("len(NetworkAllow) = %d, want 2", len(pCI.NetworkAllow))
	}
	if pCI.NoExec {
		t.Error("ci-builder has exec.allow, NoExec must be false")
	}

	// Mount synthesis and conflict resolution
	// . is read, but ./dist is write -> both should be present, dist rw and . ro
	var hasDistRW, hasDotRO, hasTmpRW bool
	for _, m := range pCI.Mounts {
		if m.Destination == filepath.Join(work, "dist") && m.Mode == MountRW {
			hasDistRW = true
		}
		if m.Destination == work && m.Mode == MountRO {
			hasDotRO = true
		}
		if m.Destination == "/tmp" && m.Mode == MountRW {
			hasTmpRW = true
		}
	}
	if !hasDistRW {
		t.Errorf("missing dist RW mount in %+v", pCI.Mounts)
	}
	if !hasDotRO {
		t.Errorf("missing . RO mount in %+v", pCI.Mounts)
	}
	if !hasTmpRW {
		t.Errorf("missing /tmp RW tmpfs mount in %+v", pCI.Mounts)
	}

	// Dual-engine compilation to libkite.PermissionConfig
	ciPerms := pCI.CompilePermissions()
	if ciPerms.Default != libkite.DefaultDeny {
		t.Errorf("ci-builder default = %v, want DefaultDeny", ciPerms.Default)
	}
	// Calls allow/deny
	if !slices.Contains(ciPerms.Allow, "os.*") {
		t.Errorf("ciPerms missing os.* in Allow: %v", ciPerms.Allow)
	}
	if !slices.Contains(ciPerms.Deny, "os.exit") {
		t.Errorf("ciPerms missing os.exit in Deny: %v", ciPerms.Deny)
	}
	// Exec allow/deny
	if !slices.Contains(ciPerms.Allow, "os.exec(git)") {
		t.Errorf("ciPerms missing os.exec(git) in Allow: %v", ciPerms.Allow)
	}
	if !slices.Contains(ciPerms.Deny, "os.exec(rm)") {
		t.Errorf("ciPerms missing os.exec(rm) in Deny: %v", ciPerms.Deny)
	}

	// Env compilation
	hostEnv := []string{
		"PATH=/usr/bin:/bin",
		"GITHUB_TOKEN=secret123",
		"SUPER_SECRET=ignoreme",
		"TERM=xterm-256color",
	}
	compiledEnv := pCI.CompileEnv(hostEnv)
	if !slices.Contains(compiledEnv, "GITHUB_TOKEN=secret123") {
		t.Errorf("CompileEnv missing forwarded GITHUB_TOKEN: %v", compiledEnv)
	}
	if !slices.Contains(compiledEnv, "GOPROXY=https://proxy.golang.org,direct") {
		t.Errorf("CompileEnv missing static GOPROXY: %v", compiledEnv)
	}
	for _, e := range compiledEnv {
		if strings.HasPrefix(e, "SUPER_SECRET=") {
			t.Errorf("CompileEnv leaked uninherited SUPER_SECRET: %v", e)
		}
	}
}

func TestResolvePermissions(t *testing.T) {
	customDir := t.TempDir()
	cfg := &SandboxConfig{
		Profiles: map[string]ProfileBody{
			"custom": {
				Paths: PathsSpec{Read: []string{customDir}},
			},
		},
	}

	// Built-in ladder resolution
	pPath, err := ResolvePermissions("sandbox-path", cfg)
	if err != nil {
		t.Fatalf("ResolvePermissions(sandbox-path): %v", err)
	}
	if pPath == nil || pPath.Default != libkite.DefaultDeny {
		t.Errorf("unexpected pPath: %+v", pPath)
	}

	// Legacy alias rejected
	_, err = ResolvePermissions("allow-fs", cfg)
	if err == nil {
		t.Error("expected ResolvePermissions(allow-fs) to fail (removed legacy name)")
	}

	for _, name := range []string{ProfileDenyAll, ProfileSandboxExec, ProfileSandboxNet, ProfileUnconfined} {
		pLadder, err := ResolvePermissions(name, cfg)
		if err != nil {
			t.Fatalf("ResolvePermissions(%s): %v", name, err)
		}
		if pLadder == nil {
			t.Fatalf("ResolvePermissions(%s) returned nil", name)
		}
	}

	// Custom profile resolution
	pCustom, err := ResolvePermissions("custom", cfg)
	if err != nil {
		t.Fatalf("ResolvePermissions(custom): %v", err)
	}
	expectedRule := fmt.Sprintf("fs.read(%s/**)", filepath.Clean(customDir))
	if !slices.Contains(pCustom.Allow, expectedRule) {
		t.Errorf("pCustom missing %s: %v", expectedRule, pCustom.Allow)
	}
}

func TestCompilePaths_NetworkAssetAutoUnveil(t *testing.T) {
	body := ProfileBody{
		Network: networkSpec{Mode: "host"},
	}
	p, err := profileFromBody("net-test", body, "test")
	if err != nil {
		t.Fatalf("profileFromBody: %v", err)
	}

	destMap := make(map[string]bool)
	for _, m := range p.Mounts {
		destMap[m.Destination] = true
	}

	for _, req := range []string{"/etc/resolv.conf", "/etc/hosts"} {
		if _, statErr := os.Stat(req); statErr == nil {
			if !destMap[req] {
				t.Errorf("expected network profile to auto-unveil %q, mounts: %+v", req, p.Mounts)
			}
		}
	}

	bodyNone := ProfileBody{
		Network: networkSpec{Mode: "none"},
	}
	pNone, err := profileFromBody("none-test", bodyNone, "test")
	if err != nil {
		t.Fatalf("profileFromBody(none): %v", err)
	}
	for _, m := range pNone.Mounts {
		if m.Destination == "/etc/resolv.conf" || m.Destination == "/etc/hosts" {
			t.Errorf("network: none profile should not unveil %q", m.Destination)
		}
	}
}

func TestProfile_NetworkAllowDeny_Parity(t *testing.T) {
	yamlConfig := `
network:
  mode: host
  allow:
    - api.github.com:443
    - 192.168.1.0/24:80
  deny:
    - 10.0.0.0/8
max_memory: 512MB
`
	var body ProfileBody
	if err := yaml.Unmarshal([]byte(yamlConfig), &body); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if len(body.Network.Allow) != 2 || body.Network.Allow[0] != "api.github.com:443" {
		t.Errorf("Network.Allow = %v, want [api.github.com:443, 192.168.1.0/24:80]", body.Network.Allow)
	}
	if len(body.Network.Deny) != 1 || body.Network.Deny[0] != "10.0.0.0/8" {
		t.Errorf("Network.Deny = %v, want [10.0.0.0/8]", body.Network.Deny)
	}
	prof, err := profileFromBody("test-parity", body, "test")
	if err != nil {
		t.Fatalf("profileFromBody failed: %v", err)
	}
	if len(prof.NetworkAllow) != 2 || prof.NetworkAllow[0] != "api.github.com:443" {
		t.Errorf("prof.NetworkAllow = %v, want [api.github.com:443, 192.168.1.0/24:80]", prof.NetworkAllow)
	}
	if len(prof.NetworkDeny) != 1 || prof.NetworkDeny[0] != "10.0.0.0/8" {
		t.Errorf("prof.NetworkDeny = %v, want [10.0.0.0/8]", prof.NetworkDeny)
	}
	if prof.Network != NetworkHost {
		t.Errorf("prof.Network = %v, want host", prof.Network)
	}
	if prof.MaxMemoryMB != 512 {
		t.Errorf("prof.MaxMemoryMB = %v, want 512", prof.MaxMemoryMB)
	}

	t.Run("inheritance with deny wildcard resets host mode", func(t *testing.T) {
		inheritYaml := `
base: sandbox-net
network:
  deny:
    - "*"
`
		var inheritBody ProfileBody
		if err := yaml.Unmarshal([]byte(inheritYaml), &inheritBody); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		profInherit, err := profileFromBody("test-inherit-airgap", inheritBody, "test")
		if err != nil {
			t.Fatalf("profileFromBody failed: %v", err)
		}
		if profInherit.Network != NetworkNone {
			t.Errorf("profInherit.Network = %v, want none", profInherit.Network)
		}
		if len(profInherit.NetworkAllow) != 0 {
			t.Errorf("profInherit.NetworkAllow = %v, want empty", profInherit.NetworkAllow)
		}
		if len(profInherit.NetworkDeny) != 1 || profInherit.NetworkDeny[0] != "*" {
			t.Errorf("profInherit.NetworkDeny = %v, want [*]", profInherit.NetworkDeny)
		}
	})
}

func TestProfile_ApplyCLIOverrides_Network(t *testing.T) {
	t.Run("bare allow flag sets NetworkHost and wildcard", func(t *testing.T) {
		p := Profile{Network: NetworkNone}
		err := p.ApplyCLIOverrides(CLIOverrides{
			NetworkAllow: []string{"*"},
		})
		if err != nil {
			t.Fatalf("ApplyCLIOverrides: %v", err)
		}
		if p.Network != NetworkHost {
			t.Errorf("Network = %v, want host", p.Network)
		}
		if len(p.NetworkAllow) != 1 || p.NetworkAllow[0] != "*" {
			t.Errorf("NetworkAllow = %v, want [*]", p.NetworkAllow)
		}
	})

	t.Run("bare deny flag sets NetworkNone and wildcard", func(t *testing.T) {
		p := Profile{Network: NetworkHost, NetworkAllow: []string{"api.github.com:443"}}
		err := p.ApplyCLIOverrides(CLIOverrides{
			NetworkDeny: []string{"*"},
		})
		if err != nil {
			t.Fatalf("ApplyCLIOverrides: %v", err)
		}
		if p.Network != NetworkNone {
			t.Errorf("Network = %v, want none", p.Network)
		}
		if len(p.NetworkAllow) != 0 {
			t.Errorf("NetworkAllow = %v, want empty after wildcard deny", p.NetworkAllow)
		}
		if len(p.NetworkDeny) != 1 || p.NetworkDeny[0] != "*" {
			t.Errorf("NetworkDeny = %v, want [*]", p.NetworkDeny)
		}
	})

	t.Run("scoped allow endpoint sets NetworkHost and preserves endpoints", func(t *testing.T) {
		p := Profile{Network: NetworkNone}
		err := p.ApplyCLIOverrides(CLIOverrides{
			NetworkAllow: []string{"api.github.com:443", "registry.internal:8443"},
		})
		if err != nil {
			t.Fatalf("ApplyCLIOverrides: %v", err)
		}
		if p.Network != NetworkHost {
			t.Errorf("Network = %v, want host", p.Network)
		}
		if len(p.NetworkAllow) != 2 || p.NetworkAllow[0] != "api.github.com:443" {
			t.Errorf("NetworkAllow = %v", p.NetworkAllow)
		}
	})

	t.Run("network mode override", func(t *testing.T) {
		p := Profile{Network: NetworkHost}
		err := p.ApplyCLIOverrides(CLIOverrides{
			NetworkMode: "loopback",
		})
		if err != nil {
			t.Fatalf("ApplyCLIOverrides: %v", err)
		}
		if p.Network != NetworkLoopback {
			t.Errorf("Network = %v, want loopback", p.Network)
		}
	})

	t.Run("invalid network mode returns error", func(t *testing.T) {
		p := Profile{}
		err := p.ApplyCLIOverrides(CLIOverrides{
			NetworkMode: "invalid-mode",
		})
		if err == nil {
			t.Fatal("expected error for invalid network mode, got nil")
		}
	})
}

func TestProfile_ApplyCLIOverrides_MaxMemory(t *testing.T) {
	t.Run("valid memory override", func(t *testing.T) {
		p := Profile{}
		err := p.ApplyCLIOverrides(CLIOverrides{
			MaxMemory: "1GB",
		})
		if err != nil {
			t.Fatalf("ApplyCLIOverrides: %v", err)
		}
		if p.MaxMemoryMB != 1024 {
			t.Errorf("MaxMemoryMB = %d, want 1024", p.MaxMemoryMB)
		}
	})

	t.Run("invalid memory returns error", func(t *testing.T) {
		p := Profile{}
		err := p.ApplyCLIOverrides(CLIOverrides{
			MaxMemory: "not-a-number",
		})
		if err == nil {
			t.Fatal("expected error for invalid max memory, got nil")
		}
	})
}
