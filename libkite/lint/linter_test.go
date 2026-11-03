package lint_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/syntax"

	"github.com/project-starkite/starkite/libkite/lint"
)

func parseScript(t *testing.T, filename, src string) *syntax.File {
	t.Helper()
	opts := &syntax.FileOptions{}
	f, err := opts.Parse(filename, []byte(src), 0)
	if err != nil {
		t.Fatalf("parse error on %s: %v", filename, err)
	}
	return f
}

func TestLinterL001_TopLevelMain(t *testing.T) {
	src := `def main():
    pass

main()
`
	file := parseScript(t, "test.star", src)
	diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

	found := false
	for _, d := range diags {
		if d.Code == lint.CodeTopLevelMain {
			found = true
			if d.Severity != lint.SeverityHint {
				t.Errorf("expected SeverityHint, got %v", d.Severity)
			}
			if !strings.Contains(d.Message, "skips its automatic main() invocation") {
				t.Errorf("unexpected message: %s", d.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected L001 diagnostic, got %v", diags)
	}
}

func TestLinterL002_UncheckedTryResults(t *testing.T) {
	t.Run("discarded try_ call", func(t *testing.T) {
		src := `os.try_exec(["echo", "hi"])`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeUncheckedTry {
				found = true
				if d.Severity != lint.SeverityWarning {
					t.Errorf("expected SeverityWarning, got %v", d.Severity)
				}
				if !strings.Contains(d.Message, "result of try_ call is discarded") {
					t.Errorf("unexpected message: %s", d.Message)
				}
			}
		}
		if !found {
			t.Errorf("expected L002 for discarded try_, got %v", diags)
		}
	})

	t.Run("assigned but unchecked try_ call", func(t *testing.T) {
		src := `def main():
    res = os.try_exec(["echo", "hi"])
    print(res.value)
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeUncheckedTry {
				found = true
				if !strings.Contains(d.Message, "read res.ok before res.value") {
					t.Errorf("unexpected message: %s", d.Message)
				}
			}
		}
		if !found {
			t.Errorf("expected L002 for unchecked try_, got %v", diags)
		}
	})

	t.Run("properly checked try_ call with ok passes", func(t *testing.T) {
		src := `def main():
    res = os.try_exec(["echo", "hi"])
    if res.ok:
        print(res.value)
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		for _, d := range diags {
			if d.Code == lint.CodeUncheckedTry {
				t.Errorf("unexpected L002 on checked try_ result: %v", d)
			}
		}
	})

	t.Run("properly checked try_ call with err passes", func(t *testing.T) {
		src := `def main():
    res = os.try_exec(["echo", "hi"])
    if res.err:
        print("failed")
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		for _, d := range diags {
			if d.Code == lint.CodeUncheckedTry {
				t.Errorf("unexpected L002 on checked try_ result: %v", d)
			}
		}
	})

	t.Run("scope isolation prevents cross-function collision", func(t *testing.T) {
		src := `def check_network():
    res = os.try_exec("ping", ["1.1.1.1"])
    if res.ok:
        print("online")

def bad_handler():
    res = os.try_exec("rm", ["/tmp/scratch"])
    print(res.value)
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		foundBadHandlerWarning := false
		foundCheckNetworkWarning := false
		for _, d := range diags {
			if d.Code == lint.CodeUncheckedTry {
				if d.Range.Start.Line >= 6 {
					foundBadHandlerWarning = true
				} else {
					foundCheckNetworkWarning = true
				}
			}
		}

		if !foundBadHandlerWarning {
			t.Errorf("expected L002 in bad_handler, but it was suppressed by check_network")
		}
		if foundCheckNetworkWarning {
			t.Errorf("unexpected L002 in properly checked check_network")
		}
	})
}

func TestLinterL003_DeprecatedSymbols(t *testing.T) {
	src := `def main():
    os.system("echo hello")
    fs.read_file("test.txt")
`
	file := parseScript(t, "test.star", src)
	diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

	var deprecatedCodes []string
	for _, d := range diags {
		if d.Code == lint.CodeDeprecatedSymbol {
			deprecatedCodes = append(deprecatedCodes, d.Target)
			if d.Severity != lint.SeverityWarning {
				t.Errorf("expected SeverityWarning, got %v", d.Severity)
			}
		}
	}

	if len(deprecatedCodes) != 2 {
		t.Fatalf("expected 2 deprecation warnings, got %d: %v", len(deprecatedCodes), diags)
	}

	t.Run("strings helper deprecations suggest native methods", func(t *testing.T) {
		src := `def main():
    strings.split("a,b", ",")
    strings.trim_space("  hi  ")
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		var targets []string
		for _, d := range diags {
			if d.Code == lint.CodeDeprecatedSymbol {
				targets = append(targets, d.Target)
			}
		}
		if len(targets) != 2 {
			t.Fatalf("expected 2 strings deprecation warnings, got %d: %v", len(targets), diags)
		}
	})
}

func TestLinterL004_SandboxPolicyHints(t *testing.T) {
	t.Run("subprocess exec hint", func(t *testing.T) {
		src := `def main():
    res = os.exec(["ls"])
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeSandboxPolicy && strings.Contains(d.Message, "--sandbox-exec-allow") {
				found = true
				if d.Severity != lint.SeverityHint {
					t.Errorf("expected SeverityHint, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected L004 sandbox exec hint, got %v", diags)
		}
	})

	t.Run("privileged port warning", func(t *testing.T) {
		src := `def main():
    srv = http.server(port=80)
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeSandboxPolicy && strings.Contains(d.Message, "binding port 80 requires root") {
				found = true
				if d.Severity != lint.SeverityWarning {
					t.Errorf("expected SeverityWarning, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected L004 privileged port warning, got %v", diags)
		}
	})

	t.Run("sensitive path warning", func(t *testing.T) {
		src := `def main():
    fs.write("/etc/shadow", "test")
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeSandboxPolicy && strings.Contains(d.Message, "--sandbox-path-write") {
				found = true
				if d.Severity != lint.SeverityWarning {
					t.Errorf("expected SeverityWarning, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected L004 sensitive path warning, got %v", diags)
		}
	})

	t.Run("sensitive path boundary - non-boundary does not match", func(t *testing.T) {
		src := `def main():
    fs.write("/etcetera/myconfig.txt", "data")
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		for _, d := range diags {
			if d.Code == lint.CodeSandboxPolicy && strings.Contains(d.Message, "--sandbox-path-write") {
				t.Errorf("unexpected sensitive path warning for non-boundary path /etcetera: %v", d)
			}
		}
	})

	t.Run("relative path traversal detects sensitive target", func(t *testing.T) {
		src := `def main():
    fs.try_write("/tmp/../etc/shadow", "data")
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeSandboxPolicy && strings.Contains(d.Message, "--sandbox-path-write") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected sensitive path warning on cleaned traversal path /tmp/../etc/shadow")
		}
	})

	t.Run("fs.path on sensitive location warns about path-read", func(t *testing.T) {
		src := `def main():
    p = fs.path("/etc/hosts")
`
		file := parseScript(t, "test.star", src)
		diags := lint.Lint(file, "test.star", lint.DefaultOptions(""))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeSandboxPolicy && strings.Contains(d.Message, "--sandbox-path-read") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected sensitive path warning on fs.path with --sandbox-path-read, got %v", diags)
		}
	})
}

func TestLinterL005_LoadImports(t *testing.T) {
	tempDir := t.TempDir()

	// Create helper.star
	helperPath := filepath.Join(tempDir, "helper.star")
	if err := os.WriteFile(helperPath, []byte("x = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("bare .star file error", func(t *testing.T) {
		src := `load("helper.star", "x")`
		file := parseScript(t, filepath.Join(tempDir, "script.star"), src)
		diags := lint.Lint(file, filepath.Join(tempDir, "script.star"), lint.DefaultOptions(tempDir))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeUnresolvedLoad && strings.Contains(d.Message, "bare .star name") {
				found = true
				if d.Severity != lint.SeverityError {
					t.Errorf("expected SeverityError, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected bare .star L005 error, got %v", diags)
		}
	})

	t.Run("valid relative import passes", func(t *testing.T) {
		src := `load("./helper.star", "x")`
		file := parseScript(t, filepath.Join(tempDir, "script.star"), src)
		diags := lint.Lint(file, filepath.Join(tempDir, "script.star"), lint.DefaultOptions(tempDir))

		for _, d := range diags {
			if d.Code == lint.CodeUnresolvedLoad {
				t.Errorf("unexpected L005 on valid load: %v", d)
			}
		}
	})

	t.Run("missing relative import file error", func(t *testing.T) {
		src := `load("./nonexistent.star", "x")`
		file := parseScript(t, filepath.Join(tempDir, "script.star"), src)
		diags := lint.Lint(file, filepath.Join(tempDir, "script.star"), lint.DefaultOptions(tempDir))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeUnresolvedLoad && strings.Contains(d.Message, "cannot resolve load target") {
				found = true
				if d.Severity != lint.SeverityError {
					t.Errorf("expected SeverityError, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected missing file L005 error, got %v", diags)
		}
	})

	t.Run("unknown non-builtin module warning", func(t *testing.T) {
		src := `load("completely_unknown_pkg", "fn")`
		file := parseScript(t, filepath.Join(tempDir, "script.star"), src)
		diags := lint.Lint(file, filepath.Join(tempDir, "script.star"), lint.DefaultOptions(tempDir))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeUnresolvedLoad && strings.Contains(d.Message, "unknown module") {
				found = true
				if d.Severity != lint.SeverityWarning {
					t.Errorf("expected SeverityWarning, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected unknown module L005 warning, got %v", diags)
		}
	})

	t.Run("directory module with main.star passes", func(t *testing.T) {
		modDir := filepath.Join(tempDir, "mymod")
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(modDir, "main.star"), []byte("val = 42\n"), 0644); err != nil {
			t.Fatal(err)
		}

		src := `load("./mymod", "val")`
		file := parseScript(t, filepath.Join(tempDir, "script.star"), src)
		diags := lint.Lint(file, filepath.Join(tempDir, "script.star"), lint.DefaultOptions(tempDir))

		for _, d := range diags {
			if d.Code == lint.CodeUnresolvedLoad {
				t.Errorf("unexpected L005 on valid directory module: %v", d)
			}
		}
	})

	t.Run("directory module without main.star fails", func(t *testing.T) {
		emptyDir := filepath.Join(tempDir, "emptymod")
		if err := os.MkdirAll(emptyDir, 0755); err != nil {
			t.Fatal(err)
		}

		src := `load("./emptymod", "val")`
		file := parseScript(t, filepath.Join(tempDir, "script.star"), src)
		diags := lint.Lint(file, filepath.Join(tempDir, "script.star"), lint.DefaultOptions(tempDir))

		found := false
		for _, d := range diags {
			if d.Code == lint.CodeUnresolvedLoad && strings.Contains(d.Message, "contains no main.star") {
				found = true
				if d.Severity != lint.SeverityError {
					t.Errorf("expected SeverityError, got %v", d.Severity)
				}
			}
		}
		if !found {
			t.Errorf("expected directory without main.star error, got %v", diags)
		}
	})
}
