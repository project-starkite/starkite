package mcp

import "testing"

func TestIsValidIdentifier(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"foo", true},
		{"_foo", true},
		{"foo_bar", true},
		{"foo123", true},
		{"_123", true},
		{"123foo", false},
		{"foo-bar", false},
		{"foo.bar", false},
		{"foo/bar", false},
		{"foo bar", false},
		{"foo@bar", false},
		{"café", true}, // unicode letter
		{"🚀", false},   // emoji — not a letter
		{"CamelCase", true},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := isValidIdentifier(tc.in); got != tc.want {
				t.Errorf("isValidIdentifier(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
