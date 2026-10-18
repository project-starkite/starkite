package libkite

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
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

	t.Run("sandbox-path reads and writes under $CWD, blocks outside and blocks net/exec", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd: %v", err)
		}
		checker, err := NewPermissionChecker(SandboxPathPermissions())
		if err != nil {
			t.Fatalf("NewPermissionChecker: %v", err)
		}

		// Read/write/delete inside $CWD; env and prompt.
		for _, c := range []struct{ mod, cat, fn, res string }{
			{"fs", "read", "read_file", filepath.Join(cwd, "in.txt")},
			{"fs", "write", "write", filepath.Join(cwd, "out.txt")},
			{"fs", "delete", "remove", filepath.Join(cwd, "sub/x")},
			{"os", "env", "env", "PATH"},
			{"io", "prompt", "prompt", ""},
			{"sql", "open", "sqlite", filepath.Join(cwd, "test.db")},
		} {
			if err := checker.Check(c.mod, c.cat, c.fn, c.res); err != nil {
				t.Errorf("sandbox-path should allow %s.%s.%s(%s): %v", c.mod, c.cat, c.fn, c.res, err)
			}
		}

		// Reads/writes/deletes outside $CWD are blocked.
		for _, c := range []struct{ mod, cat, fn, res string }{
			{"fs", "read", "read_file", "/etc/passwd"},
			{"fs", "write", "write", "/tmp/out.txt"},
			{"fs", "delete", "remove", "/etc/hosts"},
		} {
			if err := checker.Check(c.mod, c.cat, c.fn, c.res); err == nil {
				t.Errorf("sandbox-path should block %s.%s.%s(%s) outside $CWD", c.mod, c.cat, c.fn, c.res)
			}
		}

		// Network, exec, and higher-level services are blocked.
		for _, c := range []struct{ mod, cat, fn string }{
			{"os", "exec", "exec"},
			{"http", "client", "get"},
			{"ssh", "connect", "config"},
			{"k8s", "read", "read"},
			{"mcp", "client", "connect"},
		} {
			if err := checker.Check(c.mod, c.cat, c.fn, "anything"); err == nil {
				t.Errorf("sandbox-path should block %s.%s.%s", c.mod, c.cat, c.fn)
			}
		}
	})

	t.Run("capability ladder monotonic levels", func(t *testing.T) {
		cwd, _ := os.Getwd()

		// Level 0: deny-all
		c0, _ := NewPermissionChecker(DenyAllPermissions())
		if err := c0.Check("fs", "read", "read", filepath.Join(cwd, "file")); err == nil {
			t.Error("deny-all should block fs.read")
		}
		if err := c0.Check("os", "exec", "exec", "ls"); err == nil {
			t.Error("deny-all should block os.exec")
		}
		if err := c0.Check("http", "client", "get", "https://x"); err == nil {
			t.Error("deny-all should block http.client")
		}

		// Level 1: sandbox-path
		c1, _ := NewPermissionChecker(SandboxPathPermissions())
		if err := c1.Check("fs", "read", "read", filepath.Join(cwd, "file")); err != nil {
			t.Errorf("sandbox-path should allow $CWD read: %v", err)
		}
		if err := c1.Check("os", "exec", "exec", "ls"); err == nil {
			t.Error("sandbox-path should block os.exec")
		}

		// Level 2: sandbox-exec
		c2, _ := NewPermissionChecker(SandboxExecPermissions())
		if err := c2.Check("fs", "read", "read", filepath.Join(cwd, "file")); err != nil {
			t.Errorf("sandbox-exec should allow $CWD read: %v", err)
		}
		if err := c2.Check("os", "exec", "exec", "ls"); err != nil {
			t.Errorf("sandbox-exec should allow os.exec: %v", err)
		}
		if err := c2.Check("http", "client", "get", "https://x"); err == nil {
			t.Error("sandbox-exec should block http.client")
		}

		// Level 3: sandbox-net
		c3, _ := NewPermissionChecker(SandboxNetPermissions())
		if err := c3.Check("http", "client", "get", "https://x"); err != nil {
			t.Errorf("sandbox-net should allow http.client: %v", err)
		}
		if err := c3.Check("fs", "write", "write", "/etc/passwd"); err == nil {
			t.Error("sandbox-net should block write outside $CWD")
		}

		// Level 4: unconfined (allow-all)
		c4, _ := NewPermissionChecker(AllowAllPermissions())
		if err := c4.Check("fs", "write", "write", "/etc/passwd"); err != nil {
			t.Errorf("unconfined should allow write to /etc/passwd: %v", err)
		}
	})

	t.Run("rule synthesis helpers", func(t *testing.T) {
		readRules := PathReadRules([]string{".", "/data"})
		if !slices.Contains(readRules, "fs.read($CWD/**)") || !slices.Contains(readRules, "fs.read(/data/**)") {
			t.Errorf("PathReadRules missing expected rules: %v", readRules)
		}

		writeRules := PathWriteRules([]string{"./dist"})
		if !slices.Contains(writeRules, "fs.write($CWD/dist/**)") || !slices.Contains(writeRules, "fs.delete($CWD/dist/**)") {
			t.Errorf("PathWriteRules missing expected rules: %v", writeRules)
		}

		execRules := ExecAllowRules([]string{"git", "go", "*"})
		if !slices.Contains(execRules, "os.exec") || !slices.Contains(execRules, "os.exec(git)") || !slices.Contains(execRules, "os.exec(/**/git)") {
			t.Errorf("ExecAllowRules missing expected rules: %v", execRules)
		}

		netLoopback := NetworkAllowRules("loopback", nil)
		if !slices.Contains(netLoopback, "http.server") {
			t.Errorf("NetworkAllowRules loopback missing http.server: %v", netLoopback)
		}

		netHost := NetworkAllowRules("host", nil)
		if !slices.Contains(netHost, "http.client") {
			t.Errorf("NetworkAllowRules host missing http.client: %v", netHost)
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

	t.Run("with deny-all checker", func(t *testing.T) {
		thread := &starlark.Thread{Name: "test"}
		checker, _ := NewPermissionChecker(DenyAllPermissions())
		SetPermissions(thread, checker)

		// deny-all blocks every gated operation.
		if err := Check(thread, "fs", "read", "read_file", "/etc/passwd"); err == nil {
			t.Error("deny-all should block fs.read")
		}
		if err := Check(thread, "os", "exec", "exec", "ls"); err == nil {
			t.Error("deny-all should block os.exec")
		}
	})
}

func TestPermissionError(t *testing.T) {
	// A deny-rule block names module.category and carries no opt-up hint.
	err := &PermissionError{
		Module:   "fs",
		Category: "read",
		Function: "read_file",
		Resource: "/etc/passwd",
		Reason:   "blocked by deny rule",
	}
	expected := `permission denied: fs.read read_file("/etc/passwd") - blocked by deny rule`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}

	// A missing-grant denial includes a prescriptive opt-up suggestion.
	err2 := &PermissionError{
		Module:   "os",
		Category: "exec",
		Function: "exec",
		Reason:   "no matching allow rule",
		Suggest:  "allow-local",
	}
	msg := err2.Error()
	if !strings.HasPrefix(msg, "permission denied: os.exec exec - no matching allow rule") {
		t.Errorf("Error() = %q, missing capability prefix", msg)
	}
	if !strings.Contains(msg, "--sandbox-profile=allow-local") {
		t.Errorf("Error() = %q, missing opt-up suggestion", msg)
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
		// Module names can contain hyphens.
		rule, err := ParseRule("my-mod.run(myfn:*)")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rule.Module != "my-mod" || rule.Category != "run" {
			t.Errorf("Module=%q Category=%q, want my-mod/run", rule.Module, rule.Category)
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
	var out strings.Builder
	out.WriteString(parts[0])
	for _, p := range parts[1:] {
		out.WriteString("," + p)
	}
	return out.String()
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
	for range goroutines {
		wg.Go(func() {
			for range calls {
				_ = checker.Check("fs", "read", "read_file", "/tmp/x")
				_ = checker.Check("os", "exec", "exec", "ls")
				_ = checker.Check("http", "client", "get", "https://x")
			}
		})
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

func TestRuleMatches_EmptyResourceBypassBlocked(t *testing.T) {
	checker, err := NewPermissionChecker(&PermissionConfig{
		Allow:   []string{"http.server(127.0.0.1:8080)"},
		Default: DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	if err := checker.Check("http", "server", "serve", ""); err == nil {
		t.Fatal("resource-scoped allow rule must not grant call with empty resource")
	}
	if err := checker.Check("http", "server", "serve", "127.0.0.1:8080"); err != nil {
		t.Fatalf("expected exact resource to be allowed: %v", err)
	}
}

func TestMatchDoublestar_MultiWildcardAndAnchoring(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/work/**", "/work/a/b/c.txt", true},
		{"/work/**", "/work-other/a.txt", false},
		{"/**/secret/**", "/a/b/secret/c/key.pem", true},
		{"/**/secret/**", "/a/b/public/c/key.pem", false},
		{"/var/**/log/*.log", "/var/a/b/log/app.log", true},
		{"/var/**/log/*.log", "/var/a/b/log/app.txt", false},
		{"sqlite:**", "sqlite:/tmp/test.db", true},
		{"sqlite:**", "postgres://localhost/db", false},
	}
	for _, tc := range cases {
		if got := matchDoublestar(tc.pattern, tc.path); got != tc.want {
			t.Errorf("matchDoublestar(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestCanonicalizePath_SymlinkEscapeBlocked(t *testing.T) {
	allowedDir := t.TempDir()
	outsideDir := t.TempDir()

	escapeLink := filepath.Join(allowedDir, "escape")
	if err := os.Symlink(outsideDir, escapeLink); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	checker, err := NewPermissionChecker(&PermissionConfig{
		Allow:   []string{"fs.write(" + filepath.ToSlash(allowedDir) + "/**)"},
		Default: DefaultDeny,
	})
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}

	safeTarget := CanonicalizePath(filepath.Join(allowedDir, "sub", "file.txt"))
	if err := checker.Check("fs", "write", "write", safeTarget); err != nil {
		t.Fatalf("expected path inside allowedDir to succeed: %v", err)
	}

	escapedTarget := CanonicalizePath(filepath.Join(escapeLink, "pwned.txt"))
	if err := checker.Check("fs", "write", "write", escapedTarget); err == nil {
		t.Fatalf("expected symlink escape %q -> %q to be blocked", filepath.Join(escapeLink, "pwned.txt"), escapedTarget)
	}
}

func TestParseRule_WindowsDriveLetter(t *testing.T) {
	rule, err := ParseRule("fs.write(C:/Users/alice/work/**)")
	if err != nil {
		t.Fatalf("ParseRule: %v", err)
	}
	if rule.Functions != nil {
		t.Fatalf("expected Functions == nil for Windows drive path, got %v", rule.Functions)
	}
	if rule.Resource != "C:/Users/alice/work/**" {
		t.Fatalf("expected Resource == %q, got %q", "C:/Users/alice/work/**", rule.Resource)
	}
}

func TestCapabilityLadderLevels(t *testing.T) {
	// Level 0: Deny-All
	p0 := DenyAllPermissions()
	if p0.Default != DefaultDeny || len(p0.Allow) != 0 {
		t.Errorf("Level 0 DenyAll: default=%v, allow=%v", p0.Default, p0.Allow)
	}

	// Level 1: Sandbox-Path
	p1 := SandboxPathPermissions()
	if p1.Default != DefaultDeny {
		t.Errorf("Level 1 SandboxPath: default=%v", p1.Default)
	}
	if !slices.Contains(p1.Allow, "fs.read($CWD/**)") || !slices.Contains(p1.Allow, "fs.write($CWD/**)") {
		t.Errorf("Level 1 SandboxPath missing workspace rules: %v", p1.Allow)
	}
	if slices.Contains(p1.Allow, "os.exec") {
		t.Errorf("Level 1 SandboxPath must not have os.exec: %v", p1.Allow)
	}

	// Level 2: Sandbox-Exec
	p2 := SandboxExecPermissions()
	if p2.Default != DefaultDeny {
		t.Errorf("Level 2 SandboxExec: default=%v", p2.Default)
	}
	for _, r := range p1.Allow {
		if !slices.Contains(p2.Allow, r) {
			t.Errorf("Level 2 SandboxExec missing Level 1 rule %q", r)
		}
	}
	if !slices.Contains(p2.Allow, "os.exec") || !slices.Contains(p2.Allow, "os.shell") {
		t.Errorf("Level 2 SandboxExec missing os.exec/os.shell: %v", p2.Allow)
	}
	if slices.Contains(p2.Allow, "http.client") {
		t.Errorf("Level 2 SandboxExec must not have http.client: %v", p2.Allow)
	}

	// Level 3: Sandbox-Net
	p3 := SandboxNetPermissions()
	if p3.Default != DefaultDeny {
		t.Errorf("Level 3 SandboxNet: default=%v", p3.Default)
	}
	for _, r := range p2.Allow {
		if !slices.Contains(p3.Allow, r) {
			t.Errorf("Level 3 SandboxNet missing Level 2 rule %q", r)
		}
	}
	if !slices.Contains(p3.Allow, "http.client") || !slices.Contains(p3.Allow, "ssh.connect") {
		t.Errorf("Level 3 SandboxNet missing network rules: %v", p3.Allow)
	}

	// Level 4: Unconfined
	p4 := AllowAllPermissions()
	if p4.Default != DefaultAllow {
		t.Errorf("Level 4 AllowAll: default=%v, want DefaultAllow", p4.Default)
	}
}

func TestRuleSynthesisHelpers(t *testing.T) {
	// PathReadRules
	rRead := PathReadRules([]string{"./src", "/var/data", "/"})
	if !slices.Contains(rRead, "fs.read($CWD/src/**)") || !slices.Contains(rRead, "fs.read(/var/data/**)") {
		t.Errorf("PathReadRules missing expected path: %v", rRead)
	}
	if !slices.Contains(rRead, "fs.read(/**)") {
		t.Errorf("PathReadRules missing root rule: %v", rRead)
	}

	// PathWriteRules
	rWrite := PathWriteRules([]string{"./dist"})
	if !slices.Contains(rWrite, "fs.read($CWD/dist/**)") {
		t.Errorf("PathWriteRules missing read: %v", rWrite)
	}
	if !slices.Contains(rWrite, "fs.write($CWD/dist/**)") || !slices.Contains(rWrite, "fs.delete($CWD/dist/**)") {
		t.Errorf("PathWriteRules missing write/delete: %v", rWrite)
	}
	if !slices.Contains(rWrite, "sql.open(sqlite::memory:)") {
		t.Errorf("PathWriteRules missing sqlite memory: %v", rWrite)
	}

	// ExecAllowRules
	rExecWildcard := ExecAllowRules([]string{"*"})
	if !slices.Contains(rExecWildcard, "os.exec") || !slices.Contains(rExecWildcard, "os.shell") {
		t.Errorf("ExecAllowRules(*) failed: %v", rExecWildcard)
	}
	rExecNamed := ExecAllowRules([]string{"git", "/usr/bin/tar"})
	if !slices.Contains(rExecNamed, "os.exec(git)") || !slices.Contains(rExecNamed, "os.exec(/**/git)") {
		t.Errorf("ExecAllowRules bare binary failed: %v", rExecNamed)
	}
	if !slices.Contains(rExecNamed, "os.exec(/usr/bin/tar)") {
		t.Errorf("ExecAllowRules absolute binary failed: %v", rExecNamed)
	}

	// ExecDenyRules
	rDeny := ExecDenyRules([]string{"rm"})
	if !slices.Contains(rDeny, "os.exec(rm)") || !slices.Contains(rDeny, "os.exec(/**/rm)") {
		t.Errorf("ExecDenyRules failed: %v", rDeny)
	}

	// NetworkAllowRules
	if rNone := NetworkAllowRules("none", nil); len(rNone) != 0 {
		t.Errorf("NetworkAllowRules(none) = %v, want nil/empty", rNone)
	}
	rLoopback := NetworkAllowRules("loopback", nil)
	if !slices.Contains(rLoopback, "http.server") || !slices.Contains(rLoopback, "http.client(http://localhost*)") {
		t.Errorf("NetworkAllowRules(loopback) failed: %v", rLoopback)
	}
	rHost := NetworkAllowRules("host", nil)
	if !slices.Contains(rHost, "http.client") || !slices.Contains(rHost, "ssh.connect") {
		t.Errorf("NetworkAllowRules(host) failed: %v", rHost)
	}
	rEndpoints := NetworkAllowRules("none", []string{"ghcr.io:443"})
	if !slices.Contains(rEndpoints, "http.client") {
		t.Errorf("NetworkAllowRules with endpoints failed: %v", rEndpoints)
	}
}
