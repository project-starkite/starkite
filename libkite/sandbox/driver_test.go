package sandbox

import (
	"context"
	"testing"
	"time"
)

type mockDriver struct {
	name      string
	available bool
}

func (m *mockDriver) Name() string    { return m.name }
func (m *mockDriver) Available() bool { return m.available }
func (m *mockDriver) ValidateSpec(spec *ExecutionSpec) error {
	return spec.Validate()
}
func (m *mockDriver) Exec(ctx context.Context, spec *ExecutionSpec) (*ExecResult, error) {
	if err := m.ValidateSpec(spec); err != nil {
		return nil, err
	}
	return &ExecResult{
		ExitCode: 0,
		Duration: 5 * time.Millisecond,
		Stdout:   "mock output",
	}, nil
}

func TestExecutionSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    ExecutionSpec
		wantErr bool
	}{
		{
			name: "valid minimal spec",
			spec: ExecutionSpec{
				Command: []string{"echo", "hello"},
			},
			wantErr: false,
		},
		{
			name: "empty command",
			spec: ExecutionSpec{
				Command: []string{},
			},
			wantErr: true,
		},
		{
			name: "negative memory",
			spec: ExecutionSpec{
				Command:     []string{"ls"},
				MaxMemoryMB: -1,
			},
			wantErr: true,
		},
		{
			name: "negative cpus",
			spec: ExecutionSpec{
				Command: []string{"ls"},
				MaxCPUs: -0.5,
			},
			wantErr: true,
		},
		{
			name: "negative pids",
			spec: ExecutionSpec{
				Command: []string{"ls"},
				MaxPIDs: -10,
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			spec: ExecutionSpec{
				Command: []string{"ls"},
				Timeout: -5 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "mount missing destination",
			spec: ExecutionSpec{
				Command: []string{"ls"},
				Mounts: []Mount{
					{Type: MountBind, Source: "/host/path"},
				},
			},
			wantErr: true,
		},
		{
			name: "bind mount missing source",
			spec: ExecutionSpec{
				Command: []string{"ls"},
				Mounts: []Mount{
					{Type: MountBind, Destination: "/sandbox/path"},
				},
			},
			wantErr: true,
		},
		{
			name: "tmpfs mount without source is valid",
			spec: ExecutionSpec{
				Command: []string{"ls"},
				Mounts: []Mount{
					{Type: MountTmpfs, Destination: "/tmp"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMockDriver_Exec(t *testing.T) {
	d := &mockDriver{name: "test-mock", available: true}
	spec := &ExecutionSpec{
		Command: []string{"test-cmd"},
	}

	res, err := d.Exec(context.Background(), spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if res.Stdout != "mock output" {
		t.Errorf("Stdout = %q, want 'mock output'", res.Stdout)
	}
}
