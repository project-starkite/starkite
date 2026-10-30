package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMountFlag(t *testing.T) {
	origMountStat := mountStat
	defer func() { mountStat = origMountStat }()

	mountStat = func(name string) (os.FileInfo, error) {
		if strings.Contains(name, "non/existent") || strings.Contains(name, "nonexistent") {
			return nil, os.ErrNotExist
		}
		return nil, nil
	}

	tests := []struct {
		name        string
		spec        string
		cwd         string
		home        string
		wantMount   Mount
		wantErr     bool
		errContains string
	}{
		{
			name: "full bind ro",
			spec: "/host/dir:/app/dir:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/host/dir"),
				Destination: "/app/dir",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "full bind rw",
			spec: "/host/dir:/app/dir:rw",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/host/dir"),
				Destination: "/app/dir",
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
		{
			name: "full bind default ro",
			spec: "/host/dir:/app/dir",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/host/dir"),
				Destination: "/app/dir",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "identity mount ro",
			spec: "/opt/tools",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/opt/tools"),
				Destination: "/opt/tools",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "identity mount rw",
			spec: "/var/log:rw",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/var/log"),
				Destination: "/var/log",
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
		{
			name: "identity mount bare relative path",
			spec: "node_modules",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/work/node_modules"),
				Destination: filepath.Clean("/work/node_modules"),
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "identity mount bare relative path with mode",
			spec: ".venv:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/work/.venv"),
				Destination: filepath.Clean("/work/.venv"),
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "identity mount bare relative path rw",
			spec: ".venv:rw",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/work/.venv"),
				Destination: filepath.Clean("/work/.venv"),
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
		{
			name: "identity mount bare relative path explicit both sides",
			spec: ".venv:.venv:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/work/.venv"),
				Destination: filepath.Clean("/work/.venv"),
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "relative path against cwd",
			spec: "./data:/data:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/work/data"),
				Destination: "/data",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "home expansion tilde",
			spec: "~/cfg:/etc/cfg:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/home/alice/cfg"),
				Destination: "/etc/cfg",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "tmpfs scratch rw default",
			spec: "tmpfs:/scratch",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      "",
				Destination: "/scratch",
				Type:        MountTmpfs,
				Mode:        MountRW,
			},
		},
		{
			name: "tmpfs scratch ro",
			spec: "tmpfs:/scratch:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      "",
				Destination: "/scratch",
				Type:        MountTmpfs,
				Mode:        MountRO,
			},
		},
		{
			name: "path with commas",
			spec: "/store,2026:/app,data:ro",
			cwd:  "/work",
			home: "/home/alice",
			wantMount: Mount{
				Source:      filepath.Clean("/store,2026"),
				Destination: "/app,data",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "windows drive ro",
			spec: `C:\data:C:\app\data:ro`,
			cwd:  `C:\work`,
			home: `C:\Users\alice`,
			wantMount: Mount{
				Source:      filepath.Clean(`C:\data`),
				Destination: `C:\app\data`,
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
		{
			name: "windows drive rw",
			spec: `D:\cache:D:\cache:rw`,
			cwd:  `C:\work`,
			home: `C:\Users\alice`,
			wantMount: Mount{
				Source:      filepath.Clean(`D:\cache`),
				Destination: `D:\cache`,
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
		{
			name: "windows identity mount",
			spec: `C:\tools:rw`,
			cwd:  `C:\work`,
			home: `C:\Users\alice`,
			wantMount: Mount{
				Source:      filepath.Clean(`C:\tools`),
				Destination: `C:\tools`,
				Type:        MountBind,
				Mode:        MountRW,
			},
		},
		{
			name: "windows tmpfs scratch",
			spec: `tmpfs:C:\scratch`,
			cwd:  `C:\work`,
			home: `C:\Users\alice`,
			wantMount: Mount{
				Source:      "",
				Destination: `C:\scratch`,
				Type:        MountTmpfs,
				Mode:        MountRW,
			},
		},
		{
			name:        "err empty spec",
			spec:        "",
			wantErr:     true,
			errContains: "empty mount specification",
		},
		{
			name:        "err empty tmpfs",
			spec:        "tmpfs:",
			wantErr:     true,
			errContains: "empty tmpfs destination in mount spec",
		},
		{
			name:        "err relative destination",
			spec:        "/host/dir:rel/dir",
			wantErr:     true,
			errContains: `destination "rel/dir" must be an absolute path`,
		},
		{
			name:        "err relative source unresolvable",
			spec:        "rel/dir:/app/dir",
			cwd:         "",
			home:        "",
			wantErr:     true,
			errContains: `source "rel/dir" must be an absolute path`,
		},
		{
			name:        "err invalid mode",
			spec:        "/host/dir:/app:rx",
			wantErr:     true,
			errContains: `invalid mount mode "rx" (want "ro" or "rw")`,
		},
		{
			name:        "err absent source",
			spec:        "/non/existent/path:/app:ro",
			wantErr:     true,
			errContains: `bind source "/non/existent/path" does not exist on the host`,
		},
		{
			name:        "err colon in path",
			spec:        "/host/dir:sub:colon:/app",
			wantErr:     true,
			errContains: "too many colons",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseMountFlag(tt.spec, tt.cwd, tt.home)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMountFlag(%q) expected error, got nil", tt.spec)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseMountFlag(%q) error %q does not contain %q", tt.spec, err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMountFlag(%q) unexpected error: %v", tt.spec, err)
			}
			if m != tt.wantMount {
				t.Errorf("ParseMountFlag(%q) = %+v, want %+v", tt.spec, m, tt.wantMount)
			}
		})
	}
}

func TestProfile_ApplyCLIOverrides_Mounts(t *testing.T) {
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}

	p := Profile{
		Name:    ProfileDenyAll,
		Enabled: true,
		Mounts: []Mount{
			{
				Source:      dataDir,
				Destination: "/app/data",
				Type:        MountBind,
				Mode:        MountRO,
			},
		},
	}

	// Override existing mount to rw
	err := p.ApplyCLIOverrides(CLIOverrides{
		Mounts: []string{dataDir + ":/app/data:rw", "tmpfs:/scratch"},
	})
	if err != nil {
		t.Fatalf("ApplyCLIOverrides: %v", err)
	}

	if len(p.Mounts) != 2 {
		t.Fatalf("len(p.Mounts) = %d, want 2 (%+v)", len(p.Mounts), p.Mounts)
	}

	// Verify /app/data mode is now rw
	var foundData, foundScratch bool
	for _, m := range p.Mounts {
		if m.Destination == "/app/data" {
			foundData = true
			if m.Mode != MountRW {
				t.Errorf("/app/data mode = %s, want rw", m.Mode)
			}
		}
		if m.Destination == "/scratch" {
			foundScratch = true
			if m.Type != MountTmpfs {
				t.Errorf("/scratch type = %s, want tmpfs", m.Type)
			}
			if m.Mode != MountRW {
				t.Errorf("/scratch mode = %s, want rw", m.Mode)
			}
		}
	}
	if !foundData {
		t.Error("missing /app/data mount")
	}
	if !foundScratch {
		t.Error("missing /scratch mount")
	}
}
