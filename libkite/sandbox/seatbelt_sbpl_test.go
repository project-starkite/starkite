package sandbox

import (
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
