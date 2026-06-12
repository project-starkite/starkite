package permissions

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

func TestLoadProfile_BuiltIns(t *testing.T) {
	tests := []struct {
		name        string
		wantDefault libkite.PermissionDefault
		wantAllow   []string
	}{
		{ProfileAllowAll, libkite.DefaultAllow, []string{"*.*"}},
		{ProfileDenyAll, libkite.DefaultDeny, nil},
		{ProfileAllowFS, libkite.DefaultDeny, []string{
			"fs.read", "fs.write($CWD/**)", "fs.delete($CWD/**)", "os.env", "io.prompt",
			"sql.open(sqlite:**)",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadProfile(tt.name)
			if err != nil {
				t.Fatalf("LoadProfile(%q): %v", tt.name, err)
			}
			if cfg == nil {
				t.Fatalf("LoadProfile(%q) returned nil", tt.name)
			}
			if cfg.Default != tt.wantDefault {
				t.Errorf("Default = %v, want %v", cfg.Default, tt.wantDefault)
			}
			if !reflect.DeepEqual(cfg.Allow, tt.wantAllow) {
				t.Errorf("Allow = %v, want %v", cfg.Allow, tt.wantAllow)
			}
		})
	}
}

func TestLoadProfile_EmptyReturnsNil(t *testing.T) {
	cfg, err := LoadProfile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("LoadProfile(\"\") = %+v, want nil (trust mode)", cfg)
	}
}

func TestLoadProfile_UnknownNameErrors(t *testing.T) {
	_, err := LoadProfile("nonexistent-profile-xyz")
	if err == nil {
		t.Fatal("expected error for unknown name")
	}
	if !strings.Contains(err.Error(), "allow-all") || !strings.Contains(err.Error(), "deny-all") {
		t.Errorf("error should mention built-in names; got %q", err.Error())
	}
}

func TestResolve(t *testing.T) {
	defined := map[string]ProfileSpec{
		"default": {Allow: []string{"fs.read"}},
		"team":    {Allow: []string{"fs.read", "http.client"}},
	}

	t.Run("empty falls back to default profile", func(t *testing.T) {
		cfg, err := Resolve("", defined)
		if err != nil {
			t.Fatalf("Resolve(\"\"): %v", err)
		}
		if !reflect.DeepEqual(cfg.Allow, []string{"fs.read"}) {
			t.Errorf("Allow = %v, want [fs.read]", cfg.Allow)
		}
	})

	t.Run("empty with no default falls back to deny-all", func(t *testing.T) {
		cfg, err := Resolve("", nil)
		if err != nil {
			t.Fatalf("Resolve(\"\", nil): %v", err)
		}
		if cfg.Default != libkite.DefaultDeny || len(cfg.Allow) != 0 {
			t.Errorf("want deny-all, got %+v", cfg)
		}
	})

	t.Run("named config profile", func(t *testing.T) {
		cfg, err := Resolve("team", defined)
		if err != nil {
			t.Fatalf("Resolve(team): %v", err)
		}
		if !reflect.DeepEqual(cfg.Allow, []string{"fs.read", "http.client"}) {
			t.Errorf("Allow = %v", cfg.Allow)
		}
	})

	t.Run("built-in wins over config", func(t *testing.T) {
		cfg, err := Resolve(ProfileAllowAll, defined)
		if err != nil {
			t.Fatalf("Resolve(allow-all): %v", err)
		}
		if cfg.Default != libkite.DefaultAllow {
			t.Errorf("Default = %v, want DefaultAllow", cfg.Default)
		}
	})

	t.Run("default name undefined errors", func(t *testing.T) {
		if _, err := Resolve("default", nil); err == nil {
			t.Error("Resolve(\"default\", nil) should error: named profile not defined")
		}
	})

	t.Run("unknown name errors", func(t *testing.T) {
		if _, err := Resolve("nope", defined); err == nil {
			t.Error("Resolve of undefined name should error")
		}
	})
}

func TestResolve_Alias(t *testing.T) {
	defined := map[string]ProfileSpec{
		"default": {Alias: ProfileAllowFS},
		"wide":    {Alias: ProfileAllowAll},
		"broken":  {Alias: "no-such-builtin"},
	}

	t.Run("alias expands to its built-in", func(t *testing.T) {
		cfg, err := Resolve("wide", defined)
		if err != nil {
			t.Fatalf("Resolve(wide): %v", err)
		}
		if cfg.Default != libkite.DefaultAllow {
			t.Errorf("alias allow-all: Default = %v, want DefaultAllow", cfg.Default)
		}
	})

	t.Run("default profile as alias", func(t *testing.T) {
		cfg, err := Resolve("", defined)
		if err != nil {
			t.Fatalf("Resolve(\"\"): %v", err)
		}
		want, _ := Resolve(ProfileAllowFS, nil)
		if !reflect.DeepEqual(cfg.Allow, want.Allow) {
			t.Errorf("default alias allow-fs: Allow = %v, want %v", cfg.Allow, want.Allow)
		}
	})

	t.Run("alias to unknown built-in errors", func(t *testing.T) {
		if _, err := Resolve("broken", defined); err == nil {
			t.Error("alias to non-built-in should error")
		}
	})
}

func TestProfileSpec_UnmarshalYAML(t *testing.T) {
	var m map[string]ProfileSpec
	src := []byte(`
default: allow-fs
ci:
  allow: [fs.read]
  deny: [fs.delete]
`)
	if err := yaml.Unmarshal(src, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["default"].Alias != "allow-fs" {
		t.Errorf("default.Alias = %q, want allow-fs", m["default"].Alias)
	}
	if !reflect.DeepEqual(m["ci"].Allow, []string{"fs.read"}) || !reflect.DeepEqual(m["ci"].Deny, []string{"fs.delete"}) {
		t.Errorf("ci spec = %+v", m["ci"])
	}
}

func TestResolve_RemovedSyntaxesError(t *testing.T) {
	// Inline rules and file paths are no longer accepted; they resolve as
	// unknown profile names.
	for _, v := range []string{
		"allow:fs.read",
		"allow:fs.read;deny:os.exec",
		"./profile.yaml",
		"profiles.yaml#deploy",
	} {
		if _, err := Resolve(v, nil); err == nil {
			t.Errorf("Resolve(%q) should error: not a profile name", v)
		}
	}
}
