package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/sandbox"
)

// resetDefaults resets flag variables and Changed state to defaults.
func resetDefaults() {
	debugMode = false
	outputFormat = "text"
	timeout = 300

	flags := rootCmd.PersistentFlags()
	flags.Lookup("debug").Changed = false
	flags.Lookup("output").Changed = false
	flags.Lookup("timeout").Changed = false
}

func TestEnvDebug(t *testing.T) {
	resetDefaults()
	t.Setenv("STARKITE_DEBUG", "1")
	applyEnvDefaults()
	if !debugMode {
		t.Error("expected debugMode=true when STARKITE_DEBUG=1")
	}
}

func TestEnvDebugTrue(t *testing.T) {
	resetDefaults()
	t.Setenv("STARKITE_DEBUG", "true")
	applyEnvDefaults()
	if !debugMode {
		t.Error("expected debugMode=true when STARKITE_DEBUG=true")
	}
}

func TestEnvDebugFlagOverride(t *testing.T) {
	resetDefaults()
	t.Setenv("STARKITE_DEBUG", "1")
	rootCmd.PersistentFlags().Lookup("debug").Changed = true
	applyEnvDefaults()
	if debugMode {
		t.Error("expected debugMode=false when --debug flag was explicitly set (Changed=true)")
	}
}

func TestEnvOutput(t *testing.T) {
	resetDefaults()
	t.Setenv("STARKITE_OUTPUT", "json")
	applyEnvDefaults()
	if outputFormat != "json" {
		t.Errorf("expected outputFormat=json, got %s", outputFormat)
	}
}

func TestEnvTimeout(t *testing.T) {
	resetDefaults()
	t.Setenv("STARKITE_TIMEOUT", "60")
	applyEnvDefaults()
	if timeout != 60 {
		t.Errorf("expected timeout=60, got %d", timeout)
	}
}

func TestEnvTimeoutInvalid(t *testing.T) {
	resetDefaults()
	t.Setenv("STARKITE_TIMEOUT", "abc")
	applyEnvDefaults()
	if timeout != 300 {
		t.Errorf("expected timeout=300 (default) for invalid env, got %d", timeout)
	}
}

func resetSandboxFlags() {
	sandboxEnabled = true
	sandboxProfile = ""
	sandboxDriver = ""
	sandboxPathRead = nil
	sandboxPathWrite = nil
	sandboxCallAllow = nil
	sandboxCallDeny = nil
	sandboxExecAllow = nil
	sandboxExecDeny = nil
	sandboxNet = false

	flags := rootCmd.PersistentFlags()
	for _, name := range []string{
		"sandbox-enabled", "sandbox-profile", "sandbox-driver",
		"sandbox-path-read", "sandbox-path-write",
		"sandbox-call-allow", "sandbox-call-deny",
		"sandbox-exec-allow", "sandbox-exec-deny",
		"sandbox-net",
	} {
		if f := flags.Lookup(name); f != nil {
			f.Changed = false
		}
	}
}

func TestSandboxFlags(t *testing.T) {
	defer resetSandboxFlags()

	t.Run("default resolution", func(t *testing.T) {
		resetSandboxFlags()
		if !sandboxEnabled {
			t.Error("expected sandboxEnabled=true by default")
		}
		if sandboxProfile != "" {
			t.Errorf("expected empty sandboxProfile, got %q", sandboxProfile)
		}
		prof, err := ResolveActiveProfile()
		if err != nil {
			t.Fatalf("ResolveActiveProfile: %v", err)
		}
		if prof.Name != sandbox.DefaultProfileName {
			t.Errorf("prof.Name = %q, want %q", prof.Name, sandbox.DefaultProfileName)
		}
		if !prof.Enabled {
			t.Error("expected prof.Enabled=true")
		}
		if !prof.NoExec {
			t.Error("expected prof.NoExec=true under default deny-all")
		}
		if prof.Network != sandbox.NetworkNone {
			t.Errorf("prof.Network = %q, want %q", prof.Network, sandbox.NetworkNone)
		}
		perms := prof.CompilePermissions()
		if perms.Default != libkite.DefaultDeny {
			t.Errorf("perms.Default = %v, want DefaultDeny", perms.Default)
		}
	})

	t.Run("sandbox-enabled=false disables sandbox", func(t *testing.T) {
		resetSandboxFlags()
		sandboxEnabled = false
		rootCmd.PersistentFlags().Lookup("sandbox-enabled").Changed = true

		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("GetSandbox: %v", err)
		}
		if !p.IsZero() {
			t.Errorf("expected zero profile when disabled, got %+v", p)
		}

		prof, err := ResolveActiveProfile()
		if err != nil {
			t.Fatalf("ResolveActiveProfile: %v", err)
		}
		if prof.Enabled {
			t.Error("expected prof.Enabled=false")
		}
		perms := prof.CompilePermissions()
		if perms.Default != libkite.DefaultAllow {
			t.Errorf("perms.Default = %v, want DefaultAllow", perms.Default)
		}
	})

	t.Run("sandbox-profile=unconfined disables sandbox", func(t *testing.T) {
		resetSandboxFlags()
		sandboxProfile = "unconfined"

		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("GetSandbox: %v", err)
		}
		if !p.IsZero() {
			t.Errorf("expected zero profile for unconfined, got %+v", p)
		}

		prof, err := ResolveActiveProfile()
		if err != nil {
			t.Fatalf("ResolveActiveProfile: %v", err)
		}
		if prof.Enabled {
			t.Error("expected prof.Enabled=false")
		}
	})

	t.Run("ladder profiles", func(t *testing.T) {
		cases := []struct {
			profile string
			noExec  bool
			network sandbox.NetworkMode
		}{
			{"deny-all", true, sandbox.NetworkNone},
			{"sandbox-path", true, sandbox.NetworkNone},
			{"sandbox-exec", false, sandbox.NetworkNone},
			{"sandbox-net", false, sandbox.NetworkHost},
		}

		for _, tc := range cases {
			t.Run(tc.profile, func(t *testing.T) {
				resetSandboxFlags()
				sandboxProfile = tc.profile
				prof, err := ResolveActiveProfile()
				if err != nil {
					t.Fatalf("ResolveActiveProfile(%s): %v", tc.profile, err)
				}
				if !prof.Enabled {
					t.Errorf("expected prof.Enabled=true for %s", tc.profile)
				}
				if prof.NoExec != tc.noExec {
					t.Errorf("prof.NoExec = %v, want %v", prof.NoExec, tc.noExec)
				}
				if prof.Network != tc.network {
					t.Errorf("prof.Network = %q, want %q", prof.Network, tc.network)
				}
			})
		}
	})

	t.Run("capability flag overrides", func(t *testing.T) {
		resetSandboxFlags()
		sandboxProfile = "sandbox-path"
		sandboxPathRead = []string{"/extra/read"}
		sandboxPathWrite = []string{"/extra/write"}
		sandboxCallAllow = []string{"custom.call"}
		sandboxCallDeny = []string{"denied.call"}
		sandboxExecAllow = []string{"echo"}
		sandboxExecDeny = []string{"rm"}
		sandboxNet = true

		prof, err := ResolveActiveProfile()
		if err != nil {
			t.Fatalf("ResolveActiveProfile: %v", err)
		}
		if prof.NoExec {
			t.Error("expected prof.NoExec=false after sandboxExecAllow")
		}
		if prof.Network != sandbox.NetworkHost {
			t.Errorf("prof.Network = %q, want host", prof.Network)
		}

		perms := prof.CompilePermissions()
		if !slices.Contains(perms.Allow, "custom.call") {
			t.Errorf("perms missing custom.call: %v", perms.Allow)
		}
		if !slices.Contains(perms.Deny, "denied.call") {
			t.Errorf("perms missing denied.call: %v", perms.Deny)
		}
		if !slices.Contains(perms.Allow, "os.exec(echo)") {
			t.Errorf("perms missing os.exec(echo): %v", perms.Allow)
		}
		if !slices.Contains(perms.Deny, "os.exec(rm)") {
			t.Errorf("perms missing os.exec(rm): %v", perms.Deny)
		}
	})

	t.Run("legacy flags rejected", func(t *testing.T) {
		legacyFlags := []string{
			"--permissions",
			"--allow-all",
			"--allow-fs",
			"--sandboxed",
			"--sandbox-opaque",
			"--sandbox-net-access",
			"--sandbox-host",
			"--deny-all",
		}
		for _, flag := range legacyFlags {
			f := rootCmd.PersistentFlags().Lookup(strings.TrimPrefix(flag, "--"))
			if f != nil {
				t.Errorf("legacy flag %q still registered in persistent flags!", flag)
			}
		}
	})
}

func TestGetSandboxDriverPrecedence(t *testing.T) {
	defer resetSandboxFlags()

	t.Run("flag overrides env var", func(t *testing.T) {
		resetSandboxFlags()
		sandboxProfile = "sandbox-path"
		sandboxDriver = "landlock"
		t.Setenv("STARKITE_SANDBOX_PROFILE", "sandbox-net")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "podman")

		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Driver != "landlock" {
			t.Errorf("p.Driver = %s, want landlock", p.Driver)
		}
		if p.Name != "sandbox-path" {
			t.Errorf("p.Name = %s, want sandbox-path", p.Name)
		}
	})

	t.Run("env var used when flag empty", func(t *testing.T) {
		resetSandboxFlags()
		sandboxProfile = ""
		sandboxDriver = ""
		t.Setenv("STARKITE_SANDBOX_PROFILE", "sandbox-net")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "seatbelt")

		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Driver != "seatbelt" {
			t.Errorf("p.Driver = %s, want seatbelt", p.Driver)
		}
		if p.Network != "host" {
			t.Errorf("p.Network = %s, want host", p.Network)
		}
	})
}

func TestStripSandboxArgs(t *testing.T) {
	args := []string{
		"run",
		"--sandbox-enabled",
		"--sandbox-profile", "sandbox-path",
		"--sandbox-driver=landlock",
		"--sandbox-path-read", "/tmp/r",
		"--sandbox-path-write=/tmp/w",
		"--sandbox-call-allow", "os.env",
		"--sandbox-call-deny=io.prompt",
		"--sandbox-exec-allow", "echo",
		"--sandbox-exec-deny=rm",
		"--sandbox-net",
		"--var=foo=bar",
		"script.star",
		"arg1",
	}

	stripped := stripSandboxArgs(args)
	expected := []string{
		"run",
		"--var=foo=bar",
		"script.star",
		"arg1",
	}

	if !slices.Equal(stripped, expected) {
		t.Errorf("stripSandboxArgs got %v, want %v", stripped, expected)
	}
}

func TestNeedsImplicitRun(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "script file without subcommand",
			args: []string{"kite", "./script.star"},
			want: true,
		},
		{
			name: "sandbox flag before script file",
			args: []string{"kite", "--sandbox-net", "./script.star"},
			want: true,
		},
		{
			name: "sandbox profile flag before script file",
			args: []string{"kite", "--sandbox-profile", "sandbox-path", "./script.star"},
			want: true,
		},
		{
			name: "explicit run subcommand",
			args: []string{"kite", "run", "./script.star"},
			want: false,
		},
		{
			name: "explicit test subcommand",
			args: []string{"kite", "test", "./tests/sandbox/..."},
			want: false,
		},
		{
			name: "help flag only",
			args: []string{"kite", "--help"},
			want: false,
		},
		{
			name: "version subcommand",
			args: []string{"kite", "version"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsImplicitRun(tt.args)
			if got != tt.want {
				t.Errorf("needsImplicitRun(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
