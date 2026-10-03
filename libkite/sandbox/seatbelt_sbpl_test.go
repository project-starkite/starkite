package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSeatbeltSBPL(t *testing.T) {
	spec := &ExecutionSpec{
		Command: []string{"echo", "test"},
		Cwd:     "/workspace",
		Network: NetworkNone,
		Mounts: []Mount{
			{
				Source:      "/data/inputs",
				Destination: "/data/inputs",
				Type:        MountBind,
				Mode:        MountRO,
			},
			{
				Source:      "/data/outputs",
				Destination: "/data/outputs",
				Type:        MountBind,
				Mode:        MountRW,
			},
			{
				Destination: "/tmp/scratch",
				Type:        MountTmpfs,
				Mode:        MountRW,
			},
		},
	}

	sbpl := GenerateSeatbeltSBPL(spec)

	// Check core denials and allowances
	expectedSnippets := []string{
		"(version 1)",
		"(deny default)",
		"(allow process-exec)",
		"(allow file-read*)",
		"(allow mach-lookup)",
		`(allow file-write* (subpath "/data/outputs"))`,
		`(allow file-write* (subpath "/tmp/scratch"))`,
		`(allow file-write* (subpath "/workspace"))`,
		"(deny network*)",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(sbpl, snippet) {
			t.Errorf("GenerateSeatbeltSBPL output missing snippet:\n%s\n\nFull SBPL:\n%s", snippet, sbpl)
		}
	}
}

func TestGenerateSeatbeltSBPL_NetworkModes(t *testing.T) {
	tests := []struct {
		network NetworkMode
		want    string
	}{
		{
			network: NetworkHost,
			want:    "(allow network*)",
		},
		{
			network: NetworkLoopback,
			want:    `(allow network* (local ip "localhost:*"))`,
		},
		{
			network: NetworkNone,
			want:    "(deny network*)",
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.network), func(t *testing.T) {
			spec := &ExecutionSpec{Network: tt.network}
			sbpl := GenerateSeatbeltSBPL(spec)
			if !strings.Contains(sbpl, tt.want) {
				t.Errorf("Network %s: missing snippet %q in SBPL", tt.network, tt.want)
			}
		})
	}
}

func TestGenerateSeatbeltSBPL_ContainerParity(t *testing.T) {
	rawHome, err := os.UserHomeDir()
	if err != nil || rawHome == "" {
		t.Skip("skipping test: os.UserHomeDir() unavailable")
	}
	home := cleanSBPLPath(rawHome)

	t.Run("home directory execution", func(t *testing.T) {
		scriptFile := cleanSBPLPath(filepath.Join(home, "hello.star"))
		spec := &ExecutionSpec{
			Command:    []string{"kite", "run", scriptFile},
			Cwd:        home,
			ScriptFile: scriptFile,
		}
		sbpl := GenerateSeatbeltSBPL(spec)

		// Must deny home read and write
		if !strings.Contains(sbpl, fmt.Sprintf("(deny file-read* (subpath %q))", home)) {
			t.Errorf("expected home read deny in SBPL")
		}
		if !strings.Contains(sbpl, fmt.Sprintf("(deny file-write* (subpath %q))", home)) {
			t.Errorf("expected home write deny in SBPL")
		}

		// Must NOT re-allow home or ~/.starkite
		if strings.Contains(sbpl, fmt.Sprintf("(allow file-write* (subpath %q))", home)) {
			t.Errorf("home directory must not be granted write access")
		}
		starkiteDir := cleanSBPLPath(filepath.Join(home, ".starkite"))
		if strings.Contains(sbpl, fmt.Sprintf("(allow file-read* (subpath %q))", starkiteDir)) {
			t.Errorf("~/.starkite must not be granted read access when Cwd is $HOME")
		}

		// Must allow only the script file
		if !strings.Contains(sbpl, fmt.Sprintf("(allow file-read* (literal %q))", scriptFile)) {
			t.Errorf("expected script file literal allow in SBPL")
		}
	})

	t.Run("root directory execution", func(t *testing.T) {
		spec := &ExecutionSpec{
			Command:    []string{"kite", "run", "/hello.star"},
			Cwd:        "/",
			ScriptFile: "/hello.star",
		}
		sbpl := GenerateSeatbeltSBPL(spec)

		// Must NOT allow write on "/"
		if strings.Contains(sbpl, `(allow file-write* (subpath "/"))`) {
			t.Errorf("root directory / must not be granted write access")
		}

		// Must allow only the script file
		if !strings.Contains(sbpl, `(allow file-read* (literal "/hello.star"))`) {
			t.Errorf("expected script file literal allow in SBPL")
		}
	})

	t.Run("subdirectory inside home", func(t *testing.T) {
		subDir := cleanSBPLPath(filepath.Join(home, "projects", "demo"))
		spec := &ExecutionSpec{
			Command: []string{"kite", "run", "main.star"},
			Cwd:     subDir,
		}
		sbpl := GenerateSeatbeltSBPL(spec)

		// Must deny broad home
		if !strings.Contains(sbpl, fmt.Sprintf("(deny file-read* (subpath %q))", home)) {
			t.Errorf("expected home read deny in SBPL")
		}

		// Must re-allow subDir
		if !strings.Contains(sbpl, fmt.Sprintf("(allow file-read* (subpath %q))", subDir)) {
			t.Errorf("expected subDir read allow in SBPL")
		}
		if !strings.Contains(sbpl, fmt.Sprintf("(allow file-write* (subpath %q))", subDir)) {
			t.Errorf("expected subDir write allow in SBPL")
		}

		// Must re-allow ~/.starkite for dependencies
		starkiteDir := cleanSBPLPath(filepath.Join(home, ".starkite"))
		if !strings.Contains(sbpl, fmt.Sprintf("(allow file-read* (subpath %q))", starkiteDir)) {
			t.Errorf("expected ~/.starkite read allow in SBPL")
		}
	})
}
