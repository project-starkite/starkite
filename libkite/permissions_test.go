package libkite

import (
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"go.starlark.net/starlark"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		pattern   string
		module    string
		category  string
		functions []string
		resource  string
		wantErr   bool
	}{
		// Wildcards and category-only
		{pattern: "*.*", module: "*", category: "*"},
		{pattern: "fs.*", module: "fs", category: "*"},
		{pattern: "fs.read", module: "fs", category: "read"},
		{pattern: "os.exec", module: "os", category: "exec"},

		// Resource only
		{pattern: "fs.read(/etc/**)", module: "fs", category: "read", resource: "/etc/**"},
		{pattern: "http.client(api.example.com)", module: "http", category: "client", resource: "api.example.com"},

		// Function list with resource
		{pattern: "fs.read(read_file:*)", module: "fs", category: "read", functions: []string{"read_file"}, resource: "*"},
		{pattern: "fs.read(read_file,read_bytes:/etc/**)", module: "fs", category: "read", functions: []string{"read_file", "read_bytes"}, resource: "/etc/**"},

		// Resource literal that contains no valid funclist prefix is treated as resource
		{pattern: "fs.read(.env)", module: "fs", category: "read", resource: ".env"},
		{pattern: "fs.read(/path/with:colon)", module: "fs", category: "read", resource: "/path/with:colon"},

		// Errors
		{pattern: "fs", wantErr: true},
		{pattern: "", wantErr: true},
		{pattern: "fs.", wantErr: true},
		{pattern: ".read", wantErr: true},
		{pattern: "fs.read(", wantErr: true},
		{pattern: "fs.read(foo", wantErr: true},
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
				t.Fatalf("ParseRule(%q) unexpected error: %v", tt.pattern, err)
			}
			if rule.Module != tt.module {
				t.Errorf("Module = %q, want %q", rule.Module, tt.module)
			}
			if rule.Category != tt.category {
				t.Errorf("Category = %q, want %q", rule.Category, tt.category)
			}
			if !reflect.DeepEqual(rule.Functions, tt.functions) {
				t.Errorf("Functions = %v, want %v", rule.Functions, tt.functions)
			}
			if rule.Resource != tt.resource {
				t.Errorf("Resource = %q, want %q", rule.Resource, tt.resource)
			}
		})
	}
}

func TestRuleMatches(t *testing.T) {
	tests := []struct {
		pattern  string
		module   string
		category string
		function string
		resource string
		want     bool
	}{
		// Wildcards
		{pattern: "*.*", module: "fs", category: "read", function: "read_file", want: true},
		{pattern: "*.*", module: "http", category: "client", function: "get", want: true},
		{pattern: "fs.*", module: "fs", category: "write", function: "mkdir", want: true},
		{pattern: "fs.*", module: "os", category: "exec", function: "exec", want: false},

		// Category match
		{pattern: "fs.read", module: "fs", category: "read", function: "read_file", want: true},
		{pattern: "fs.read", module: "fs", category: "write", function: "write", want: false},
		{pattern: "fs.read", module: "os", category: "read", function: "anything", want: false},

		// Function-list filter
		{pattern: "fs.read(read_file:*)", module: "fs", category: "read", function: "read_file", resource: "/x", want: true},
		{pattern: "fs.read(read_file:*)", module: "fs", category: "read", function: "read_bytes", resource: "/x", want: false},
		{pattern: "fs.read(read_file,exists:*)", module: "fs", category: "read", function: "exists", resource: "/x", want: true},

		// Resource glob
		{pattern: "fs.read(/data/**)", module: "fs", category: "read", function: "read_file", resource: "/data/file.txt", want: true},
		{pattern: "fs.read(/data/**)", module: "fs", category: "read", function: "read_file", resource: "/other/file.txt", want: false},
		{pattern: "fs.read(.env)", module: "fs", category: "read", function: "read_file", resource: ".env", want: true},
		{pattern: "fs.read(.env)", module: "fs", category: "read", function: "read_file", resource: ".env.local", want: false},

		// Function list AND resource glob
		{pattern: "fs.read(read_file,read_bytes:/data/**)", module: "fs", category: "read", function: "read_file", resource: "/data/x.json", want: true},
		{pattern: "fs.read(read_file,read_bytes:/data/**)", module: "fs", category: "read", function: "exists", resource: "/data/x.json", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.function, func(t *testing.T) {
			rule, err := ParseRule(tt.pattern)
			if err != nil {
				t.Fatalf("ParseRule(%q): %v", tt.pattern, err)
			}
			got := rule.Matches(tt.module, tt.category, tt.function, tt.resource)
			if got != tt.want {
				t.Errorf("Rule(%q).Matches(%q, %q, %q, %q) = %v, want %v",
					tt.pattern, tt.module, tt.category, tt.function, tt.resource, got, tt.want)
			}
		})
	}
}

func TestPermissionChecker(t *testing.T) {
	t.Run("trusted allows everything", func(t *testing.T) {
		checker, err := NewPermissionChecker(AllowAllPermissions())
		if err != nil {
			t.Fatalf("NewPermissionChecker: %v", err)
		}
		if err := checker.Check("fs", "read", "read_file", "/etc/passwd"); err != nil {
			t.Errorf("trusted should allow fs.read.read_file, got: %v", err)
		}
		if err := checker.Check("os", "exec", "exec", "rm -rf /"); err != nil {
			t.Errorf("trusted should allow os.exec, got: %v", err)
		}
	})

	t.Run("strict allows working-tree fs, blocks the rest", func(t *testing.T) {
		checker, err := NewPermissionChecker(StrictPermissions())
		if err != nil {
			t.Fatalf("NewPermissionChecker: %v", err)
		}

		cwd, _ := filepath.Abs(".")

		// Working-tree fs operations are allowed.
		for _, c := range []struct{ cat, fn, res string }{
			{"read", "read_file", cwd + "/data.json"},
			{"write", "write", cwd + "/out.txt"},
			{"delete", "remove", cwd + "/tmp/x"},
		} {
			if err := checker.Check("fs", c.cat, c.fn, c.res); err != nil {
				t.Errorf("strict should allow fs.%s.%s under $CWD: %v", c.cat, c.fn, err)
			}
		}

		// Outside-cwd fs operations are blocked.
		for _, c := range []struct{ cat, fn, res string }{
			{"read", "read_file", "/etc/passwd"},
			{"write", "write", "/etc/hosts"},
			{"delete", "remove", "/etc/foo"},
		} {
			if err := checker.Check("fs", c.cat, c.fn, c.res); err == nil {
				t.Errorf("strict should block fs.%s.%s outside $CWD", c.cat, c.fn)
			}
		}

		// Non-fs gated operations are all blocked.
		for _, c := range []struct{ mod, cat, fn string }{
			{"os", "exec", "exec"},
			{"os", "env", "env"},
			{"http", "client", "get"},
			{"ssh", "connect", "config"},
			{"k8s", "write", "write"},
			{"ai", "generate", "generate"},
			{"io", "prompt", "prompt"},
		} {
			if err := checker.Check(c.mod, c.cat, c.fn, "anything"); err == nil {
				t.Errorf("strict should block %s.%s.%s", c.mod, c.cat, c.fn)
			}
		}
	})

	t.Run("custom allow/deny", func(t *testing.T) {
		checker, err := NewPermissionChecker(&PermissionConfig{
			Allow: []string{
				"fs.read(/data/**)",
				"http.client",
			},
			Deny: []string{
				"fs.read(.env)",
			},
			Default: DefaultDeny,
		})
		if err != nil {
			t.Fatalf("NewPermissionChecker: %v", err)
		}

		if err := checker.Check("fs", "read", "read_file", "/data/config.yaml"); err != nil {
			t.Errorf("should allow /data/config.yaml: %v", err)
		}
		if err := checker.Check("http", "client", "get", "https://api.example.com"); err != nil {
			t.Errorf("should allow http.client.get: %v", err)
		}
		if err := checker.Check("fs", "read", "read_file", ".env"); err == nil {
			t.Error("should block .env via deny")
		}
		if err := checker.Check("os", "exec", "exec", "ls"); err == nil {
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
			t.Fatalf("NewPermissionChecker: %v", err)
		}
		if err := checker.Check("fs", "read", "read_file", "any"); err != nil {
			t.Errorf("should allow fs.read: %v", err)
		}
		if err := checker.Check("os", "exec", "exec", "ls"); err == nil {
			t.Error("should block os.exec via explicit deny")
		}
	})
}

func TestCheckWithThread(t *testing.T) {
	t.Run("nil thread returns nil", func(t *testing.T) {
		if err := Check(nil, "fs", "read", "read_file", "any"); err != nil {
			t.Errorf("Check with nil thread should return nil, got: %v", err)
		}
	})

	t.Run("no checker returns nil", func(t *testing.T) {
		thread := &starlark.Thread{Name: "test"}
		if err := Check(thread, "fs", "read", "read_file", "any"); err != nil {
			t.Errorf("Check with no checker should return nil, got: %v", err)
		}
	})

	t.Run("with strict checker", func(t *testing.T) {
		thread := &starlark.Thread{Name: "test"}
		checker, _ := NewPermissionChecker(StrictPermissions())
		SetPermissions(thread, checker)

		// outside-$CWD path → blocked
		if err := Check(thread, "fs", "read", "read_file", "/etc/passwd"); err == nil {
			t.Error("strict should block fs.read outside $CWD")
		}
		// non-fs gated op → blocked
		if err := Check(thread, "os", "exec", "exec", "ls"); err == nil {
			t.Error("strict should block os.exec")
		}
	})
}

func TestPermissionError(t *testing.T) {
	err := &PermissionError{
		Module:   "fs",
		Category: "read",
		Function: "read_file",
		Resource: "/etc/passwd",
		Reason:   "blocked by deny rule",
	}
	expected := `permission denied: fs.read_file("/etc/passwd") - blocked by deny rule`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}

	err2 := &PermissionError{
		Module:   "os",
		Category: "exec",
		Function: "exec",
		Reason:   "no matching allow rule",
	}
	expected2 := "permission denied: os.exec - no matching allow rule"
	if err2.Error() != expected2 {
		t.Errorf("Error() = %q, want %q", err2.Error(), expected2)
	}
}

func TestIsPermissionError(t *testing.T) {
	permErr := &PermissionError{Module: "fs", Category: "read", Function: "read_file"}
	if !IsPermissionError(permErr) {
		t.Error("IsPermissionError should return true for PermissionError")
	}
	otherErr := starlark.EvalError{}
	if IsPermissionError(&otherErr) {
		t.Error("IsPermissionError should return false for other errors")
	}
}

func TestExpandPathVariables(t *testing.T) {
	tests := []struct {
		pattern, cwd, home, want string
	}{
		{"$CWD/foo.txt", "/work", "/home/u", "/work/foo.txt"},
		{"$HOME/.config", "/work", "/home/u", "/home/u/.config"},
		{"$CWD/**", "/work", "/home/u", "/work/**"},
		{"plain/path", "/work", "/home/u", "plain/path"},
		{"$CWD/$HOME", "/work", "/home/u", "/work//home/u"},
		// Empty cwd / home leave variables in place
		{"$CWD/x", "", "/home/u", "$CWD/x"},
		{"$HOME/x", "/work", "", "$HOME/x"},
	}
	for _, tt := range tests {
		got := expandPathVariables(tt.pattern, tt.cwd, tt.home)
		if got != tt.want {
			t.Errorf("expandPathVariables(%q, %q, %q) = %q, want %q",
				tt.pattern, tt.cwd, tt.home, got, tt.want)
		}
	}
}

// TestParseRule_EdgeCases covers grammar edges that aren't core happy paths.
func TestParseRule_EdgeCases(t *testing.T) {
	t.Run("empty parens", func(t *testing.T) {
		// fs.read() — empty paren contents. Treated as Resource="" (no constraint),
		// equivalent to bare fs.read.
		rule, err := ParseRule("fs.read()")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.Resource != "" || rule.Functions != nil {
			t.Errorf("empty paren should mean no constraints, got Functions=%v Resource=%q",
				rule.Functions, rule.Resource)
		}
	})

	t.Run("trailing colon (function-only, empty resource)", func(t *testing.T) {
		// fs.read(read_file:) — function filter, no resource constraint.
		rule, err := ParseRule("fs.read(read_file:)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(rule.Functions, []string{"read_file"}) {
			t.Errorf("Functions = %v, want [read_file]", rule.Functions)
		}
		if rule.Resource != "" {
			t.Errorf("Resource = %q, want empty", rule.Resource)
		}
	})

	t.Run("whitespace inside parens is NOT stripped", func(t *testing.T) {
		// "fs.read( read_file : * )" — the space-padded ident isn't a valid funclist,
		// so the whole thing is a resource literal.
		rule, err := ParseRule("fs.read( read_file : * )")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.Functions != nil {
			t.Error("padded ident should not parse as funclist")
		}
		if rule.Resource != " read_file : * " {
			t.Errorf("Resource = %q, want exact whitespace preserved", rule.Resource)
		}
	})

	t.Run("module name with hyphens", func(t *testing.T) {
		// WASM module names can contain hyphens.
		rule, err := ParseRule("my-mod.wasm(myfn:*)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.Module != "my-mod" || rule.Category != "wasm" {
			t.Errorf("Module=%q Category=%q, want my-mod/wasm", rule.Module, rule.Category)
		}
	})

	t.Run("doublestar prefix", func(t *testing.T) {
		// **/*.json patterns must work via matchDoublestar fallback.
		rule, err := ParseRule("fs.read(/data/**)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rule.Matches("fs", "read", "read_file", "/data/sub/dir/file.json") {
			t.Error("/data/** should match nested paths")
		}
	})

	t.Run("very long pattern", func(t *testing.T) {
		funcs := make([]string, 50)
		for i := range funcs {
			funcs[i] = "fn_" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		}
		pattern := "fs.read(" + joinComma(funcs) + ":/data/**)"
		rule, err := ParseRule(pattern)
		if err != nil {
			t.Fatalf("unexpected error parsing long pattern: %v", err)
		}
		if len(rule.Functions) != 50 {
			t.Errorf("Functions len = %d, want 50", len(rule.Functions))
		}
	})

	t.Run("ident leading-digit rejected", func(t *testing.T) {
		// 1foo is not a valid ident, so the funclist fails and the whole becomes resource.
		rule, err := ParseRule("fs.read(1foo:*)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.Functions != nil {
			t.Errorf("1foo should not parse as funclist, got Functions=%v", rule.Functions)
		}
		if rule.Resource != "1foo:*" {
			t.Errorf("Resource = %q, want literal", rule.Resource)
		}
	})
}

func joinComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += "," + p
	}
	return out
}

// TestPermissionChecker_Concurrent verifies the RWMutex-guarded Check is
// race-free under concurrent load. Run with `go test -race`.
func TestPermissionChecker_Concurrent(t *testing.T) {
	checker, err := NewPermissionChecker(&PermissionConfig{
		Allow: []string{
			"fs.read",
			"fs.write($CWD/**)",
			"http.client",
		},
		Deny:    []string{"os.exec"},
		Default: DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}

	const goroutines = 32
	const calls = 500

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < calls; i++ {
				_ = checker.Check("fs", "read", "read_file", "/tmp/x")
				_ = checker.Check("os", "exec", "exec", "ls")
				_ = checker.Check("http", "client", "get", "https://x")
			}
		}()
	}
	wg.Wait()
}

// TestPermissionConfig_EmptyAllowsNothing — a deny-default config with no
// rules denies every gated operation.
func TestPermissionConfig_EmptyAllowsNothing(t *testing.T) {
	checker, err := NewPermissionChecker(&PermissionConfig{Default: DefaultDeny})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	if err := checker.Check("fs", "read", "read_file", "/x"); err == nil {
		t.Error("empty-allow deny-default should block fs.read")
	}
	if err := checker.Check("os", "exec", "exec", "ls"); err == nil {
		t.Error("empty-allow deny-default should block os.exec")
	}
}

// TestPermissionConfig_DenyExpandsCWD — deny rules also get $CWD/$HOME expansion.
func TestPermissionConfig_DenyExpandsCWD(t *testing.T) {
	cwd, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	checker, err := NewPermissionChecker(&PermissionConfig{
		Allow:   []string{"fs.read"},
		Deny:    []string{"fs.read($CWD/secret/**)"},
		Default: DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	if err := checker.Check("fs", "read", "read_file", cwd+"/secret/key.pem"); err == nil {
		t.Error("deny rule with $CWD should block file under cwd/secret")
	}
	if err := checker.Check("fs", "read", "read_file", cwd+"/public/data.json"); err != nil {
		t.Errorf("non-secret path should be allowed: %v", err)
	}
}

// TestPermissionError_CategoryAccessible — Category is exposed on the struct
// even though the default Error() format omits it for backward consistency.
func TestPermissionError_CategoryAccessible(t *testing.T) {
	err := &PermissionError{
		Module:   "fs",
		Category: "delete",
		Function: "remove",
		Resource: "/tmp/x",
		Reason:   "no matching allow rule",
	}
	if err.Category != "delete" {
		t.Errorf("Category = %q, want delete", err.Category)
	}
}

// TestParseRule_EmptyFuncInList — `fs.read(,foo:*)` has an empty entry; the
// funclist should fail validation and the whole becomes a resource literal.
func TestParseRule_EmptyFuncInList(t *testing.T) {
	rule, err := ParseRule("fs.read(,foo:*)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.Functions != nil {
		t.Errorf("empty entry should reject funclist, got %v", rule.Functions)
	}
	if rule.Resource != ",foo:*" {
		t.Errorf("Resource = %q, want literal ,foo:*", rule.Resource)
	}
}
