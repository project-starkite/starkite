package permissions

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

func TestLoadProfile_Inline(t *testing.T) {
	tests := []struct {
		input         string
		wantAllow     []string
		wantDeny      []string
		wantParseFail bool
	}{
		// Single allow clause
		{
			input:     "allow:fs.read",
			wantAllow: []string{"fs.read"},
		},
		// allow + deny separated by semicolon
		{
			input:     "allow:fs.read,http.client;deny:os.exec",
			wantAllow: []string{"fs.read", "http.client"},
			wantDeny:  []string{"os.exec"},
		},
		// Whitespace inside rules is trimmed
		{
			input:     "allow: fs.read , http.client ",
			wantAllow: []string{"fs.read", "http.client"},
		},
		// Whitespace around the entire clause is trimmed
		{
			input:     "  allow:fs.read ;  deny:os.exec  ",
			wantAllow: []string{"fs.read"},
			wantDeny:  []string{"os.exec"},
		},
		// Errors
		{input: "allow:", wantParseFail: true},         // empty rule list
		{input: "allow:,fs.read", wantParseFail: true}, // empty entry
		{input: "allow:fs.read,", wantParseFail: true}, // trailing comma → empty entry
		{input: "allow", wantParseFail: true},          // no colon
		{input: "permit:fs.read", wantParseFail: true}, // unknown kind (only matches LoadProfile path; isInline filters)
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg, err := parseInline(tt.input)
			if tt.wantParseFail {
				if err == nil {
					t.Errorf("parseInline(%q) expected error, got %+v", tt.input, cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInline(%q): %v", tt.input, err)
			}
			if !reflect.DeepEqual(cfg.Allow, tt.wantAllow) {
				t.Errorf("Allow = %v, want %v", cfg.Allow, tt.wantAllow)
			}
			if !reflect.DeepEqual(cfg.Deny, tt.wantDeny) {
				t.Errorf("Deny = %v, want %v", cfg.Deny, tt.wantDeny)
			}
			if cfg.Default != libkite.DefaultDeny {
				t.Errorf("Default = %v, want DefaultDeny", cfg.Default)
			}
		})
	}
}

func TestLoadProfile_InlineThroughLoadProfile(t *testing.T) {
	cfg, err := LoadProfile("allow:fs.read,http.client;deny:os.exec")
	if err != nil {
		t.Fatalf("LoadProfile inline: %v", err)
	}
	if !reflect.DeepEqual(cfg.Allow, []string{"fs.read", "http.client"}) {
		t.Errorf("Allow = %v", cfg.Allow)
	}
	if !reflect.DeepEqual(cfg.Deny, []string{"os.exec"}) {
		t.Errorf("Deny = %v", cfg.Deny)
	}
}

func TestLoadProfile_FilePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "team.yaml")
	content := []byte(`permissions:
  team:
    allow:
      - fs.read
      - http.client
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile(%q): %v", path, err)
	}
	if !reflect.DeepEqual(cfg.Allow, []string{"fs.read", "http.client"}) {
		t.Errorf("Allow = %v", cfg.Allow)
	}
	if cfg.Default != libkite.DefaultDeny {
		t.Errorf("Default = %v, want DefaultDeny", cfg.Default)
	}
}

func TestLoadProfile_FilePathWithFragment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "security.yaml")
	content := []byte(`permissions:
  team:
    allow: [fs.read]
  dev:
    allow: [fs.read, fs.write]
    deny: [fs.delete]
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadProfile(path + "#dev")
	if err != nil {
		t.Fatalf("LoadProfile fragment: %v", err)
	}
	if cfg.Default != libkite.DefaultDeny {
		t.Errorf("dev.default = %v, want deny", cfg.Default)
	}
	if !reflect.DeepEqual(cfg.Deny, []string{"fs.delete"}) {
		t.Errorf("Deny = %v", cfg.Deny)
	}

	// No fragment + multi-profile → error suggesting #name.
	if _, err := LoadProfile(path); err == nil {
		t.Error("LoadProfile with no fragment on multi-profile file should error")
	}

	// Unknown fragment → error.
	if _, err := LoadProfile(path + "#missing"); err == nil {
		t.Error("LoadProfile with unknown fragment should error")
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

func TestLoadProfile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("this is: not [valid yaml: ::"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Error("expected parse error for malformed YAML")
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

func TestIsInline(t *testing.T) {
	cases := map[string]bool{
		"allow:fs.read": true,
		"deny:os.exec":  true,
		"strict":        false,
		"./team.yaml":   false,
		"":              false,
		"allow":         false, // missing colon
		"alloW:fs.read": false, // case-sensitive
	}
	for in, want := range cases {
		if got := isInline(in); got != want {
			t.Errorf("isInline(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsFilePath(t *testing.T) {
	cases := map[string]bool{
		"./team.yaml":       true,
		"/abs/path.yml":     true,
		"team.yaml":         true,
		"team.yaml#dev":     true,
		"team.yml":          true,
		"strict":            false,
		"allow-all":         false,
		"some-name":         false,
		"some.name":         false, // not yaml/yml extension
		`C:\path\file.yaml`: true,  // Windows path with backslash
	}
	for in, want := range cases {
		if got := isFilePath(in); got != want {
			t.Errorf("isFilePath(%q) = %v, want %v", in, got, want)
		}
	}
}
