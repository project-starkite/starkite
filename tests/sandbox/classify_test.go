package sandbox_test

import "testing"

func TestSandboxUnavailable(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "runsc sentry boot failure (CI runner)",
			output: "Found 1 test file(s)\n" +
				"W0608 container.go:1976] Skipping cgroup configuration in rootless mode: open /sys/fs/cgroup/cgroup.subtree_control: permission denied\n" +
				"Error: sandbox run: creating container: cannot create sandbox: cannot read client sync file: waiting for sandbox to start: EOF\n",
			want: true,
		},
		{
			name:   "landlock in-process prctl unsupported on CI runner",
			output: "Found 1 test file(s)\nError: failed to apply in-process sandbox (landlock): sandbox: prctl(PR_SET_NO_NEW_PRIVS) failed: operation not supported\n",
			want:   true,
		},
		{
			name:   "genuine in-sandbox test failure is not a host limitation",
			output: "Tests: 9 passed, 2 failed, 11 total\nError: tests failed\n",
			want:   false,
		},
		{
			name:   "permission denial inside the script is a real failure",
			output: "Error: permission denied: fs.write write(\"/tmp/x\") - no matching allow rule\n",
			want:   false,
		},
		{
			name:   "clean pass",
			output: "Tests: 11 passed, 0 failed, 11 total\n",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sandboxUnavailable(tt.output); got != tt.want {
				t.Errorf("sandboxUnavailable() = %v, want %v", got, tt.want)
			}
		})
	}
}
