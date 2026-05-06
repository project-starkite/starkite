//go:build linux

package gvisor

import (
	"os"
	"reflect"
	"testing"
)

func TestLooksLikeRunscInvocation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		// Empty / minimal
		{name: "empty", args: nil, want: false},
		{name: "argv0 only", args: []string{"kite"}, want: false},

		// gVisor self-exec via cosmetic argv[0]
		{name: "argv0 runsc-sandbox", args: []string{"runsc-sandbox", "--root=/tmp", "--platform=systrap", "boot", "--bundle=/x"}, want: true},
		{name: "argv0 runsc-gofer", args: []string{"runsc-gofer", "--root=/tmp", "gofer", "--bundle=/x"}, want: true},
		{name: "argv0 runsc-sandbox with full path", args: []string{"/proc/self/exe", "boot"}, want: true}, // matched via positional scan

		// argv[0] starts with runsc- — unconditional dispatch
		{name: "runsc- prefix without subcmd in argv[1:]", args: []string{"runsc-sandbox"}, want: true},

		// Direct invocation of internal subcommand without prefix
		{name: "kite boot", args: []string{"kite", "boot"}, want: true},
		{name: "kite gofer --bundle=x", args: []string{"kite", "gofer", "--bundle=/x"}, want: true},
		{name: "kite umount", args: []string{"kite", "umount"}, want: true},
		{name: "kite boot with leading flags", args: []string{"kite", "--root=/tmp", "--rootless", "boot"}, want: true},

		// Normal kite cobra commands → fall through
		{name: "kite run script", args: []string{"kite", "run", "script.star"}, want: false},
		{name: "kite test ./tests", args: []string{"kite", "test", "./tests"}, want: false},
		{name: "kite exec inline", args: []string{"kite", "exec", "print(1)"}, want: false},
		{name: "kite repl", args: []string{"kite", "repl"}, want: false},
		{name: "kite --debug run script", args: []string{"kite", "--debug", "run", "script.star"}, want: false},
		{name: "kite --output=json run", args: []string{"kite", "--output=json", "run", "script.star"}, want: false},
		{name: "kite --version", args: []string{"kite", "--version"}, want: false},

		// Edge: a script literally named boot.star — first non-flag is "run"
		{name: "kite run boot.star", args: []string{"kite", "run", "boot.star"}, want: false},

		// Edge: unknown flag, not internal cmd
		{name: "kite --bogus stuff", args: []string{"kite", "--bogus", "stuff"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeRunscInvocation(tt.args); got != tt.want {
				t.Errorf("looksLikeRunscInvocation(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestIsRuntimeMarker(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"kite"}, false},
		{[]string{"kite", "__runtime__"}, true},
		{[]string{"kite", "__runtime__", "run", "script.star"}, true},
		{[]string{"kite", "run", "__runtime__"}, false}, // marker only valid at index 1
		{[]string{"kite", "run", "script.star"}, false},
	}
	for _, tt := range tests {
		if got := isRuntimeMarker(tt.args); got != tt.want {
			t.Errorf("isRuntimeMarker(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestLooksLikeGvisorSelfExec(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "empty", args: nil, want: false},
		{name: "runsc-sandbox argv0", args: []string{"runsc-sandbox", "boot"}, want: true},
		{name: "runsc-gofer argv0", args: []string{"runsc-gofer", "gofer"}, want: true},
		{name: "runsc- prefix anywhere in basename", args: []string{"runsc-foo"}, want: true},
		{name: "kite argv0 (direct invocation)", args: []string{"kite", "boot"}, want: false},
		{name: "/proc/self/exe argv0 (no runsc- prefix)", args: []string{"/proc/self/exe", "boot"}, want: false},
		{name: "full path to runsc-sandbox", args: []string{"/some/path/runsc-sandbox", "boot"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeGvisorSelfExec(tt.args); got != tt.want {
				t.Errorf("looksLikeGvisorSelfExec(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestStripRuntimeMarker(t *testing.T) {
	saved := os.Args
	defer func() { os.Args = saved }()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "marker present",
			in:   []string{"kite", "__runtime__", "run", "script.star"},
			want: []string{"kite", "run", "script.star"},
		},
		{
			name: "marker absent — no-op",
			in:   []string{"kite", "run", "script.star"},
			want: []string{"kite", "run", "script.star"},
		},
		{
			name: "argv too short — no-op",
			in:   []string{"kite"},
			want: []string{"kite"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = append([]string(nil), tt.in...)
			stripRuntimeMarker()
			if !reflect.DeepEqual(os.Args, tt.want) {
				t.Errorf("after stripRuntimeMarker, os.Args = %v, want %v", os.Args, tt.want)
			}
		})
	}
}
