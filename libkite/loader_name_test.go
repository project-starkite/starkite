package libkite

import (
	"path/filepath"
	"testing"
)

func TestDeriveModuleName(t *testing.T) {
	tests := []struct {
		name       string
		module     string
		modulePath string
		want       string
	}{
		{
			name:       "bare name",
			module:     "greeter",
			modulePath: "/cache/acme/greeter@abc123/main.star",
			want:       "greeter",
		},
		{
			name:       "installed reference resolves to version-addressed dir",
			module:     "acme/leaf",
			modulePath: "/cache/acme/leaf@5a7c2ff8977db0d3/main.star",
			want:       "leaf",
		},
		{
			name:       "relative dir module without rev",
			module:     "./modules/mylib",
			modulePath: "/work/modules/mylib/main.star",
			want:       "mylib",
		},
		{
			name:       "single file module",
			module:     "./modules/util.star",
			modulePath: "/work/modules/util.star",
			want:       "util",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveModuleName(tt.module, filepath.FromSlash(tt.modulePath))
			if got != tt.want {
				t.Errorf("deriveModuleName(%q, %q) = %q, want %q", tt.module, tt.modulePath, got, tt.want)
			}
		})
	}
}
