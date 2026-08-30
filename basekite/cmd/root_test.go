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

func TestGetSandbox(t *testing.T) {
	t.Run("empty returns zero profile", func(t *testing.T) {
		sandboxMode = ""
		sandboxDriver = ""
		t.Setenv("STARKITE_SECURITY_SANDBOX", "")
		t.Setenv("STARKITE_SANDBOX_DRIVER", "")
		p, err := GetSandbox()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !p.IsZero() {
			t.Errorf("expected zero profile, got %+v", p)
		}
	})

	t.Run("simple profile name", func(t *testing.T) {
		sandboxMode = "opaque"
		sandboxDriver = ""
		t.Setenv("STARKITE_SECURITY_SANDBOX", "")
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
		sandboxMode = "opaque"
		sandboxDriver = "podman"
		t.Setenv("STARKITE_SECURITY_SANDBOX", "")
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
		sandboxMode = ""
		sandboxDriver = "docker"
		t.Setenv("STARKITE_SECURITY_SANDBOX", "")
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
		sandboxMode = ""
		sandboxDriver = ""
		t.Setenv("STARKITE_SECURITY_SANDBOX", "host")
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
		sandboxMode = "opaque"
		sandboxDriver = "landlock"
		t.Setenv("STARKITE_SECURITY_SANDBOX", "host")
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
