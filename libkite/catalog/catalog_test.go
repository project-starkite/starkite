package catalog_test

import (
	"strings"
	"testing"

	"github.com/project-starkite/starkite/libkite/catalog"
	"github.com/project-starkite/starkite/libkite/loader"
)

func buildCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	reg := loader.NewDefaultRegistry(nil)
	return catalog.Build(reg)
}

func symbolMap(symbols []catalog.Symbol) map[string]catalog.Symbol {
	out := make(map[string]catalog.Symbol, len(symbols))
	for _, s := range symbols {
		out[s.Name] = s
	}
	return out
}

func TestCatalog_Globals(t *testing.T) {
	cat := buildCatalog(t)
	globals := symbolMap(cat.Globals())

	// Test module namespaces
	for _, want := range []string{"fs", "http", "os", "json", "yaml", "csv", "sql", "ssh"} {
		sym, ok := globals[want]
		if !ok {
			t.Errorf("module namespace %q missing from globals", want)
		} else if sym.Kind != catalog.KindNamespace {
			t.Errorf("module %q has kind %v, want KindNamespace", want, sym.Kind)
		}
	}

	// Test global aliases
	for _, want := range []string{"printf", "read_text", "path", "var_str"} {
		if _, ok := globals[want]; !ok {
			t.Errorf("global alias %q missing from globals", want)
		}
	}

	// Verify sorting
	all := cat.Globals()
	for i := 1; i < len(all); i++ {
		if all[i-1].Name > all[i].Name {
			t.Errorf("globals not sorted: %q > %q", all[i-1].Name, all[i].Name)
		}
	}
}

func TestCatalog_ModuleMembersAndTryVariants(t *testing.T) {
	cat := buildCatalog(t)
	fsMembers := symbolMap(cat.Members("fs"))

	for _, want := range []string{"path", "try_path", "read_text", "try_read_text"} {
		if _, ok := fsMembers[want]; !ok {
			t.Errorf("fs member %q missing", want)
		}
	}

	if sym, ok := fsMembers["try_path"]; ok {
		if sym.SortText == "" || sym.SortText[0] != '9' {
			t.Errorf("try_path sortText = %q, want prefix '9'", sym.SortText)
		}
	}

	// Test os module: must have exec and try_exec, and absolutely no try_try_exec
	osMembers := symbolMap(cat.Members("os"))
	if _, ok := osMembers["exec"]; !ok {
		t.Errorf("os.exec missing")
	}
	if _, ok := osMembers["try_exec"]; !ok {
		t.Errorf("os.try_exec missing")
	}
	for name := range osMembers {
		if strings.HasPrefix(name, "try_try_") {
			t.Errorf("found unwanted %q in os members", name)
		}
	}
}

func TestCatalog_DeclarativeTypes(t *testing.T) {
	cat := buildCatalog(t)

	// fs.path object members
	pathMembers := symbolMap(cat.Members("fs.path"))
	for _, want := range []string{"read_text", "write_text", "exists", "parent", "try_read_text"} {
		if _, ok := pathMembers[want]; !ok {
			t.Errorf("fs.path missing member %q", want)
		}
	}

	// http.url object members
	urlMembers := symbolMap(cat.Members("http.url"))
	for _, want := range []string{"get", "post", "try_get", "try_post"} {
		if _, ok := urlMembers[want]; !ok {
			t.Errorf("http.url missing member %q", want)
		}
	}

	// sql.connection object members
	sqlConnMembers := symbolMap(cat.Members("sql.connection"))
	for _, want := range []string{"begin", "query", "exec", "try_begin"} {
		if _, ok := sqlConnMembers[want]; !ok {
			t.Errorf("sql.connection missing member %q", want)
		}
	}
}

func TestCatalog_FactoryReturns(t *testing.T) {
	cat := buildCatalog(t)

	tests := []struct {
		callee string
		want   string
	}{
		{"fs.path", "fs.path"},
		{"http.url", "http.url"},
		{"http.get", "http.response"},
		{"http.url.get", "http.response"},
		{"sql.open", "sql.connection"},
		{"sql.connection.begin", "sql.transaction"},
		{"json.file", "json.file"},
		{"ssh.client", "ssh.client"},
	}

	for _, tt := range tests {
		got, ok := cat.FactoryReturns(tt.callee)
		if !ok || got != tt.want {
			t.Errorf("FactoryReturns(%q) = (%q, %v), want (%q, true)", tt.callee, got, ok, tt.want)
		}
	}
}

func TestCatalog_DocumentationAttached(t *testing.T) {
	cat := buildCatalog(t)

	fsMembers := symbolMap(cat.Members("fs"))
	pathSym, ok := fsMembers["path"]
	if !ok {
		t.Fatal("fs.path member not found")
	}
	if pathSym.Signature == "" {
		t.Error("fs.path has no signature attached")
	}
	if pathSym.Doc == "" {
		t.Error("fs.path has no documentation prose attached")
	}

	// Try variant inherits doc
	tryPathSym, ok := fsMembers["try_path"]
	if !ok {
		t.Fatal("fs.try_path member not found")
	}
	if !strings.Contains(tryPathSym.Doc, "Result") {
		t.Errorf("fs.try_path doc missing Result mention: %q", tryPathSym.Doc)
	}
}
