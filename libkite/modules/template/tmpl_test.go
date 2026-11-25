package template

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func TestGetOrParseTemplateCache(t *testing.T) {
	t1, err := getOrParseTemplate("Hello {{.Name}}", "", "")
	if err != nil {
		t.Fatalf("getOrParseTemplate error: %v", err)
	}
	t2, err := getOrParseTemplate("Hello {{.Name}}", "", "")
	if err != nil {
		t.Fatalf("second getOrParseTemplate error: %v", err)
	}
	if t1 != t2 {
		t.Fatal("expected getOrParseTemplate to return cached *template.Template instance")
	}

	// Different delimiters should produce a distinct cache entry
	t3, err := getOrParseTemplate("Hello {{.Name}}", "<%", "%>")
	if err != nil {
		t.Fatalf("getOrParseTemplate with custom delims error: %v", err)
	}
	if t3 == t1 {
		t.Fatal("expected distinct *template.Template for different delimiters")
	}
}

func TestRenderTemplateWithStructs(t *testing.T) {
	m := New()
	globals, err := m.Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	globals["struct"] = starlark.NewBuiltin("struct", starlarkstruct.Make)
	script := `
cfg = struct(host = "db.internal", port = 5432, meta = struct(env = "prod"))
direct = template.render("{{.meta.env}}://{{.host}}:{{.port}}", cfg)
compiled = template.text("{{.host}}:{{.port}}").render(cfg)
`
	thread := &starlark.Thread{Name: "test-template-struct"}
	out, err := starlark.ExecFile(thread, "test.star", script, globals)
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if got := string(out["direct"].(starlark.String)); got != "prod://db.internal:5432" {
		t.Fatalf("direct = %q, want prod://db.internal:5432", got)
	}
	if got := string(out["compiled"].(starlark.String)); got != "db.internal:5432" {
		t.Fatalf("compiled = %q, want db.internal:5432", got)
	}
}
