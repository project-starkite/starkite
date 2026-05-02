package libkite

import (
	"testing"
)

// FuzzParseRule feeds arbitrary inputs to ParseRule and asserts:
//   - no panics
//   - successful parses produce a Rule with non-empty Module and Category
//   - if Functions is set, every entry is a non-empty identifier
func FuzzParseRule(f *testing.F) {
	seed := []string{
		"*.*",
		"fs.read",
		"fs.*",
		"fs.read(/etc/**)",
		"fs.read(read_file:*)",
		"fs.read(read_file,read_bytes:/etc/**)",
		"fs.read()",
		"fs.read(:resource)",
		"fs.read(foo:)",
		"fs.read(.env)",
		"fs.read(/path/with:colon)",
		"my-mod.wasm(myfn:*)",
		"http.client(api.example.com)",
		"fs.read( foo : * )",
		"fs.read(1foo:*)",
		"fs.read(,foo:*)",
		"fs.",
		".read",
		"fs",
		"",
		"fs.read(",
		"fs.read(foo",
		"fs.read())",
		"a.b(c,d,e,f,g,h:resource)",
		"fs.read($CWD/**)",
		"fs.read($HOME/.config)",
	}
	for _, s := range seed {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, pattern string) {
		rule, err := ParseRule(pattern)
		if err != nil {
			return // Expected for many random inputs
		}
		if rule.Module == "" {
			t.Errorf("ParseRule(%q) returned empty Module", pattern)
		}
		if rule.Category == "" {
			t.Errorf("ParseRule(%q) returned empty Category", pattern)
		}
		for i, fn := range rule.Functions {
			if !isIdent(fn) {
				t.Errorf("ParseRule(%q).Functions[%d] = %q, not a valid ident", pattern, i, fn)
			}
		}
	})
}

// FuzzRuleMatches parses random patterns and runs Matches against random
// operation tuples. Asserts:
//   - no panics in either ParseRule or Matches
//   - Matches is deterministic (same inputs → same output)
func FuzzRuleMatches(f *testing.F) {
	type tuple struct {
		pattern, module, category, function, resource string
	}
	seed := []tuple{
		{"*.*", "fs", "read", "read_file", "/etc/passwd"},
		{"fs.read", "fs", "read", "read_file", "/x"},
		{"fs.read", "fs", "write", "write", "/x"},
		{"fs.read(read_file:*)", "fs", "read", "read_file", "/x"},
		{"fs.read(read_file:*)", "fs", "read", "read_bytes", "/x"},
		{"fs.read(/data/**)", "fs", "read", "read_file", "/data/sub/file"},
		{"fs.read(/data/**)", "fs", "read", "read_file", "/etc/passwd"},
		{"fs.write(read_file,read_bytes:/d/**)", "fs", "write", "read_file", "/d/x"},
		{"my-mod.wasm", "my-mod", "wasm", "anything", ""},
	}
	for _, s := range seed {
		f.Add(s.pattern, s.module, s.category, s.function, s.resource)
	}

	f.Fuzz(func(t *testing.T, pattern, module, category, function, resource string) {
		rule, err := ParseRule(pattern)
		if err != nil {
			return
		}

		got1 := rule.Matches(module, category, function, resource)
		got2 := rule.Matches(module, category, function, resource)
		if got1 != got2 {
			t.Errorf("Matches not deterministic for %q against (%q,%q,%q,%q): %v vs %v",
				pattern, module, category, function, resource, got1, got2)
		}
	})
}

// FuzzPermissionChecker runs full Check() against fuzzed configs to verify
// the parse + match pipeline doesn't panic on arbitrary inputs.
func FuzzPermissionChecker(f *testing.F) {
	f.Add("fs.read", "os.exec", "fs", "read", "read_file", "/x")
	f.Add("*.*", "os.exec", "fs", "read", "read_file", "/x")
	f.Add("fs.read($CWD/**)", "fs.delete", "fs", "delete", "remove", "/tmp/x")

	f.Fuzz(func(t *testing.T, allow, deny, mod, cat, fn, res string) {
		checker, err := NewPermissionChecker(&PermissionConfig{
			Allow:   []string{allow},
			Deny:    []string{deny},
			Default: DefaultDeny,
		})
		if err != nil {
			return // Invalid rules expected for random inputs
		}
		_ = checker.Check(mod, cat, fn, res)
	})
}
