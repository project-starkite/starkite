package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
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

// buildPermissionFlagSet mirrors the production registration of --permissions
// and the boolean profile aliases on a throwaway flag set, parses the given
// args (the alias Set writes the shared permissionsMode), and returns it so the
// conflict check can be exercised.
func buildPermissionFlagSet(t *testing.T, args []string) *pflag.FlagSet {
	t.Helper()
	permissionsMode = ""
	fs := pflag.NewFlagSet("x", pflag.ContinueOnError)
	fs.StringVar(&permissionsMode, "permissions", "", "")
	for _, p := range permissionProfileFlags {
		fs.Var(permissionAlias{p}, p, "")
		fs.Lookup(p).NoOptDefVal = "true"
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return fs
}

func TestPermissionAliasFlags(t *testing.T) {
	t.Run("alias sets permissions value", func(t *testing.T) {
		fs := buildPermissionFlagSet(t, []string{"--allow-fs"})
		if err := checkPermissionFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if permissionsMode != "allow-fs" {
			t.Errorf("permissionsMode = %q, want allow-fs", permissionsMode)
		}
	})

	t.Run("deny-all alias", func(t *testing.T) {
		fs := buildPermissionFlagSet(t, []string{"--deny-all"})
		if err := checkPermissionFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if permissionsMode != "deny-all" {
			t.Errorf("permissionsMode = %q, want deny-all", permissionsMode)
		}
	})

	t.Run("two aliases conflict", func(t *testing.T) {
		fs := buildPermissionFlagSet(t, []string{"--allow-fs", "--allow-net"})
		err := checkPermissionFlagConflict(fs)
		if err == nil {
			t.Fatal("expected conflict error for two aliases")
		}
		if !strings.Contains(err.Error(), "--allow-fs") || !strings.Contains(err.Error(), "--allow-net") {
			t.Errorf("error should name both flags; got %q", err.Error())
		}
	})

	t.Run("alias plus --permissions conflict", func(t *testing.T) {
		fs := buildPermissionFlagSet(t, []string{"--allow-fs", "--permissions=allow-all"})
		err := checkPermissionFlagConflict(fs)
		if err == nil {
			t.Fatal("expected conflict error for alias + --permissions")
		}
		if !strings.Contains(err.Error(), "--permissions") || !strings.Contains(err.Error(), "--allow-fs") {
			t.Errorf("error should name both flags; got %q", err.Error())
		}
	})
}

// buildSandboxFlagSet mirrors the production registration of sandbox flags
// on a throwaway flag set so parsing and conflict checks can be exercised.
func buildSandboxFlagSet(t *testing.T, args []string) *pflag.FlagSet {
	t.Helper()
	sandboxed = false
	sandboxProfile = ""
	sandboxDriver = ""
	fs := pflag.NewFlagSet("sandbox_test", pflag.ContinueOnError)
	fs.BoolVar(&sandboxed, "sandboxed", false, "")
	fs.StringVar(&sandboxProfile, "sandbox-profile", "", "")
	fs.StringVar(&sandboxDriver, "sandbox-driver", "", "")
	for flagName, profile := range sandboxProfileAliases {
		fs.Var(sandboxAlias{profile}, flagName, "")
		fs.Lookup(flagName).NoOptDefVal = "true"
	}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return fs
}

func TestSandboxFlagsAndAliases(t *testing.T) {
	t.Run("sandboxed boolean switch", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandboxed"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true")
		}
		if sandboxProfile != "" {
			t.Errorf("expected empty profile (default fallback), got %q", sandboxProfile)
		}
	})

	t.Run("sandbox-profile space separated", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-profile", "opaque"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "opaque" {
			t.Errorf("sandboxProfile = %q, want opaque", sandboxProfile)
		}
	})

	t.Run("sandbox-profile equal separated", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-profile=net-access"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "net-access" {
			t.Errorf("sandboxProfile = %q, want net-access", sandboxProfile)
		}
	})

	t.Run("sandbox-profile custom name", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-profile", "ci-builder"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "ci-builder" {
			t.Errorf("sandboxProfile = %q, want ci-builder", sandboxProfile)
		}
	})

	t.Run("sandbox-opaque shortcut", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-opaque"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "opaque" {
			t.Errorf("sandboxProfile = %q, want opaque", sandboxProfile)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true when shortcut is set")
		}
	})

	t.Run("sandbox-net shortcut", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-net"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "net-access" {
			t.Errorf("sandboxProfile = %q, want net-access", sandboxProfile)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true when shortcut is set")
		}
	})

	t.Run("sandbox-net-access shortcut", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-net-access"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "net-access" {
			t.Errorf("sandboxProfile = %q, want net-access", sandboxProfile)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true when shortcut is set")
		}
	})

	t.Run("sandbox-host shortcut", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-host"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxProfile != "host" {
			t.Errorf("sandboxProfile = %q, want host", sandboxProfile)
		}
		if !sandboxed {
			t.Error("expected sandboxed=true when shortcut is set")
		}
	})

	t.Run("sandbox-driver flag", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-driver", "podman"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sandboxDriver != "podman" {
			t.Errorf("sandboxDriver = %q, want podman", sandboxDriver)
		}
	})

	t.Run("sandboxed switch plus sandbox-profile does not conflict", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandboxed", "--sandbox-profile=opaque"})
		if err := checkSandboxFlagConflict(fs); err != nil {
			t.Fatalf("unexpected conflict error: %v", err)
		}
		if !sandboxed || sandboxProfile != "opaque" {
			t.Errorf("sandboxed=%v, sandboxProfile=%s", sandboxed, sandboxProfile)
		}
	})

	t.Run("two shortcuts conflict", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-opaque", "--sandbox-net"})
		err := checkSandboxFlagConflict(fs)
		if err == nil {
			t.Fatal("expected conflict error for two sandbox shortcuts")
		}
		if !strings.Contains(err.Error(), "--sandbox-opaque") || !strings.Contains(err.Error(), "--sandbox-net") {
			t.Errorf("error should name both flags; got %q", err.Error())
		}
	})

	t.Run("shortcut plus sandbox-profile conflict", func(t *testing.T) {
		fs := buildSandboxFlagSet(t, []string{"--sandbox-profile=host", "--sandbox-opaque"})
		err := checkSandboxFlagConflict(fs)
		if err == nil {
			t.Fatal("expected conflict error for shortcut + --sandbox-profile")
		}
		if !strings.Contains(err.Error(), "--sandbox-profile") || !strings.Contains(err.Error(), "--sandbox-opaque") {
			t.Errorf("error should name both flags; got %q", err.Error())
		}
	})
}

func TestGetSandbox(t *testing.T) {
	t.Run("empty returns zero profile", func(t *testing.T) {
		sandboxed = false
		sandboxProfile = ""
		sandboxDriver = ""
		t.Setenv("STARKITE_SANDBOX_PROFILE", "")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.IsZero() {
			t.Errorf("expected zero profile, got %+v", p)
		}
	})

	t.Run("sandboxed boolean switch selects default profile", func(t *testing.T) {
		sandboxed = true
		sandboxProfile = ""
		sandboxDriver = ""
		t.Setenv("STARKITE_SANDBOX_PROFILE", "")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.IsZero() {
			t.Fatal("expected non-zero profile when sandboxed=true")
		}
	})

	t.Run("simple profile name", func(t *testing.T) {
		sandboxed = false
		sandboxProfile = "opaque"
		sandboxDriver = ""
		t.Setenv("STARKITE_SANDBOX_PROFILE", "")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.IsZero() {
			t.Fatal("expected non-zero profile")
		}
		if p.Network != "none" && p.Network != "loopback" {
			t.Errorf("expected opaque network, got %s", p.Network)
		}
	})

	t.Run("profile with driver override flag", func(t *testing.T) {
		sandboxed = false
		sandboxProfile = "opaque"
		sandboxDriver = "podman"
		t.Setenv("STARKITE_SANDBOX_PROFILE", "")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Driver != "podman" {
			t.Errorf("p.Driver = %s, want podman", p.Driver)
		}
		if p.Network != "none" && p.Network != "loopback" {
			t.Errorf("expected opaque network, got %s", p.Network)
		}
	})

	t.Run("driver flag alone defaults to default profile", func(t *testing.T) {
		sandboxed = false
		sandboxProfile = ""
		sandboxDriver = "docker"
		t.Setenv("STARKITE_SANDBOX_PROFILE", "")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.IsZero() {
			t.Fatal("expected non-zero profile when driver is specified")
		}
		if p.Driver != "docker" {
			t.Errorf("p.Driver = %s, want docker", p.Driver)
		}
	})

	t.Run("env vars for profile and driver", func(t *testing.T) {
		sandboxed = false
		sandboxProfile = ""
		sandboxDriver = ""
		t.Setenv("STARKITE_SANDBOX_PROFILE", "host")
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

	t.Run("CLI flag overrides env var", func(t *testing.T) {
		sandboxed = false
		sandboxProfile = "opaque"
		sandboxDriver = "landlock"
		t.Setenv("STARKITE_SANDBOX_PROFILE", "host")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "podman")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Driver != "landlock" {
			t.Errorf("p.Driver = %s, want landlock", p.Driver)
		}
		if p.Network != "none" && p.Network != "loopback" {
			t.Errorf("expected opaque network, got %s", p.Network)
		}
	})
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
			name: "flags before script file",
			args: []string{"kite", "--sandboxed", "./script.star"},
			want: true,
		},
		{
			name: "sandbox profile flag before script file",
			args: []string{"kite", "--sandbox-profile", "opaque", "./script.star"},
			want: true,
		},
		{
			name: "sandbox shortcut before script file",
			args: []string{"kite", "--sandbox-opaque", "./script.star"},
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
