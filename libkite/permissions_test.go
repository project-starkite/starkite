package libkite

import (
	"testing"

	"go.starlark.net/starlark"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		pattern  string
		module   string
		function string
		resource string
		wantErr  bool
	}{
		// Valid patterns
		{"fs.*", "fs", "*", "", false},
		{"os.exec", "os", "exec", "", false},
		{"http.get", "http", "get", "", false},
		{"*.*", "*", "*", "", false},
		{"fs.read_file(./data/**)", "fs", "read_file", "./data/**", false},
		{"http.get(api.example.com)", "http", "get", "api.example.com", false},

		// Invalid patterns
		{"fs", "", "", "", true},           // Missing function
		{"", "", "", "", true},             // Empty
		{"fs.", "", "", "", true},          // Empty function
		{".read", "", "", "", true},        // Empty module
		{"fs.read(", "", "", "", true},     // Unclosed paren
		{"fs.read(foo", "", "", "", true},  // Unclosed paren
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			rule, err := ParseRule(tt.pattern)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseRule(%q) expected error, got nil", tt.pattern)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseRule(%q) unexpected error: %v", tt.pattern, err)
				return
			}
			if rule.Module != tt.module {
				t.Errorf("ParseRule(%q).Module = %q, want %q", tt.pattern, rule.Module, tt.module)
			}
			if rule.Function != tt.function {
				t.Errorf("ParseRule(%q).Function = %q, want %q", tt.pattern, rule.Function, tt.function)
			}
			if rule.Resource != tt.resource {
				t.Errorf("ParseRule(%q).Resource = %q, want %q", tt.pattern, rule.Resource, tt.resource)
			}
		})
	}
}

func TestRuleMatches(t *testing.T) {
	tests := []struct {
		pattern  string
		module   string
		function string
		resource string
		want     bool
	}{
		// Wildcard patterns
		{"*.*", "fs", "read_file", "", true},
		{"*.*", "http", "get", "", true},
		{"fs.*", "fs", "read_file", "", true},
		{"fs.*", "fs", "write_file", "", true},
		{"fs.*", "os", "exec", "", false},

		// Exact match
		{"fs.read_file", "fs", "read_file", "", true},
		{"fs.read_file", "fs", "write_file", "", false},
		{"fs.read_file", "os", "read_file", "", false},

		// Resource patterns
		{"fs.read_file(./data/**)", "fs", "read_file", "./data/file.txt", true},
		{"fs.read_file(./data/**)", "fs", "read_file", "./other/file.txt", false},
		{"fs.read_file(.env)", "fs", "read_file", ".env", true},
		{"fs.read_file(.env)", "fs", "read_file", ".env.local", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			rule, err := ParseRule(tt.pattern)
			if err != nil {
				t.Fatalf("ParseRule(%q) error: %v", tt.pattern, err)
			}
			got := rule.Matches(tt.module, tt.function, tt.resource)
			if got != tt.want {
				t.Errorf("Rule(%q).Matches(%q, %q, %q) = %v, want %v",
					tt.pattern, tt.module, tt.function, tt.resource, got, tt.want)
			}
		})
	}
}

func TestPermissionChecker(t *testing.T) {
	t.Run("allow all", func(t *testing.T) {
		checker, err := NewPermissionChecker(TrustedPermissions())
		if err != nil {
			t.Fatalf("NewPermissionChecker error: %v", err)
		}
		if err := checker.Check("fs", "read_file", "/etc/passwd"); err != nil {
			t.Errorf("Check should allow, got: %v", err)
		}
		if err := checker.Check("os", "exec", "rm -rf /"); err != nil {
			t.Errorf("Check should allow, got: %v", err)
		}
	})

	t.Run("sandboxed", func(t *testing.T) {
		checker, err := NewPermissionChecker(SandboxedPermissions())
		if err != nil {
			t.Fatalf("NewPermissionChecker error: %v", err)
		}

		// Safe modules should be allowed
		if err := checker.Check("strings", "upper", ""); err != nil {
			t.Errorf("strings.upper should be allowed: %v", err)
		}
		if err := checker.Check("json", "encode", ""); err != nil {
			t.Errorf("json.encode should be allowed: %v", err)
		}

		// I/O modules should be blocked
		if err := checker.Check("fs", "read_file", "/etc/passwd"); err == nil {
			t.Error("fs.read_file should be blocked")
		}
		if err := checker.Check("os", "exec", "ls"); err == nil {
			t.Error("os.exec should be blocked")
		}
		if err := checker.Check("http", "get", "https://example.com"); err == nil {
			t.Error("http.get should be blocked")
		}
		if err := checker.Check("ssh", "config", "host"); err == nil {
			t.Error("ssh.config should be blocked")
		}
	})

	t.Run("custom allow/deny", func(t *testing.T) {
		checker, err := NewPermissionChecker(&PermissionConfig{
			Allow: []string{
				"fs.read_file(./data/**)",
				"http.get",
			},
			Deny: []string{
				"fs.read_file(.env)",
			},
			Default: DefaultDeny,
		})
		if err != nil {
			t.Fatalf("NewPermissionChecker error: %v", err)
		}

		// Allowed by pattern
		if err := checker.Check("fs", "read_file", "./data/config.yaml"); err != nil {
			t.Errorf("should allow ./data/config.yaml: %v", err)
		}
		if err := checker.Check("http", "get", "https://api.example.com"); err != nil {
			t.Errorf("should allow http.get: %v", err)
		}

		// Blocked by deny rule
		if err := checker.Check("fs", "read_file", ".env"); err == nil {
			t.Error("should block .env")
		}

		// Blocked by default (not in allow list)
		if err := checker.Check("os", "exec", "ls"); err == nil {
			t.Error("should block os.exec (not in allow list)")
		}
	})

	t.Run("deny takes precedence", func(t *testing.T) {
		checker, err := NewPermissionChecker(&PermissionConfig{
			Allow:   []string{"*.*"},
			Deny:    []string{"os.exec"},
			Default: DefaultAllow,
		})
		if err != nil {
			t.Fatalf("NewPermissionChecker error: %v", err)
		}

		// Allowed by *.*
		if err := checker.Check("fs", "read_file", "any"); err != nil {
			t.Errorf("should allow fs.read_file: %v", err)
		}

		// Blocked by explicit deny
		if err := checker.Check("os", "exec", "ls"); err == nil {
			t.Error("should block os.exec (explicit deny)")
		}
	})
}

func TestCheckWithThread(t *testing.T) {
	t.Run("nil thread returns nil", func(t *testing.T) {
		if err := Check(nil, "fs", "read_file", "any"); err != nil {
			t.Errorf("Check with nil thread should return nil, got: %v", err)
		}
	})

	t.Run("no checker returns nil", func(t *testing.T) {
		thread := &starlark.Thread{Name: "test"}
		// No permissions set
		if err := Check(thread, "fs", "read_file", "any"); err != nil {
			t.Errorf("Check with no checker should return nil, got: %v", err)
		}
	})

	t.Run("with checker", func(t *testing.T) {
		thread := &starlark.Thread{Name: "test"}
		checker, _ := NewPermissionChecker(SandboxedPermissions())
		SetPermissions(thread, checker)

		// Should allow safe modules
		if err := Check(thread, "strings", "upper", ""); err != nil {
			t.Errorf("should allow strings.upper: %v", err)
		}

		// Should block I/O
		if err := Check(thread, "fs", "read_file", "any"); err == nil {
			t.Error("should block fs.read_file")
		}
	})
}

func TestPermissionError(t *testing.T) {
	err := &PermissionError{
		Module:   "fs",
		Function: "read_file",
		Resource: "/etc/passwd",
		Reason:   "blocked by deny rule",
	}

	expected := `permission denied: fs.read_file("/etc/passwd") - blocked by deny rule`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}

	// Without resource
	err2 := &PermissionError{
		Module:   "os",
		Function: "exec",
		Reason:   "no matching allow rule",
	}
	expected2 := "permission denied: os.exec - no matching allow rule"
	if err2.Error() != expected2 {
		t.Errorf("Error() = %q, want %q", err2.Error(), expected2)
	}
}

func TestIsPermissionError(t *testing.T) {
	permErr := &PermissionError{Module: "fs", Function: "read"}
	if !IsPermissionError(permErr) {
		t.Error("IsPermissionError should return true for PermissionError")
	}

	otherErr := starlark.EvalError{}
	if IsPermissionError(&otherErr) {
		t.Error("IsPermissionError should return false for other errors")
	}
}
