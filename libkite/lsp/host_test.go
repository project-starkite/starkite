package lsp_test

import (
	"os"
	"strings"
	"testing"

	"github.com/M31-Labs/starlsp"

	"github.com/project-starkite/starkite/libkite/loader"
	kitelsp "github.com/project-starkite/starkite/libkite/lsp"
)

// host builds the same composition the lsp subcommand builds.
func host(t *testing.T) starlsp.Host {
	t.Helper()
	return starlsp.Hosts(starlsp.NewVanilla(), kitelsp.NewHost(loader.NewDefaultRegistry))
}

func names(symbols []starlsp.Symbol) map[string]starlsp.Symbol {
	out := make(map[string]starlsp.Symbol, len(symbols))
	for _, sym := range symbols {
		out[sym.Name] = sym
	}
	return out
}

func TestGlobalsCarryModulesAliasesAndTheUniverse(t *testing.T) {
	got := names(host(t).Globals())

	for _, want := range []string{"fs", "http", "os", "json"} {
		if sym, ok := got[want]; !ok {
			t.Errorf("module %q missing from globals", want)
		} else if sym.Kind != starlsp.KindNamespace {
			t.Errorf("module %q has kind %v, want KindNamespace", want, sym.Kind)
		}
	}
	for _, want := range []string{"printf", "read_text", "path", "var_str"} {
		if _, ok := got[want]; !ok {
			t.Errorf("global alias %q missing", want)
		}
	}
	// The specification's own names must survive the composition.
	for _, want := range []string{"len", "range", "fail"} {
		if _, ok := got[want]; !ok {
			t.Errorf("Starlark universe name %q was lost", want)
		}
	}
}

func TestModuleMembersIncludeTryVariants(t *testing.T) {
	got := names(host(t).(starlsp.MemberProvider).Members("fs"))

	for _, want := range []string{"path", "try_path"} {
		if _, ok := got[want]; !ok {
			t.Errorf("fs is missing %q", want)
		}
	}
	if got["try_path"].SortText == "" || got["try_path"].SortText[0] != '9' {
		t.Error("a try_ variant should sort after the plain call")
	}
}

func TestTryTryDefectIsFilteredOut(t *testing.T) {
	// The os module registers explicit try_ members, and the runtime's own
	// AttrNames then prefixes them a second time. Those names are reachable
	// from a script but are not real API, so completion must not offer them.
	got := names(host(t).(starlsp.MemberProvider).Members("os"))

	for name := range got {
		if strings.HasPrefix(name, "try_try_") {
			t.Errorf("completion offers the malformed name %q", name)
		}
	}
	if _, ok := got["exec"]; !ok {
		t.Error("os.exec went missing while filtering")
	}
}

func TestObjectMethodsComeFromFactoryProbing(t *testing.T) {
	h := host(t)
	typ, ok := h.(starlsp.TypeResolver).ResolveType(starlsp.TypeRequest{
		Expr:    `fs.path("/etc/hosts")`,
		Resolve: func(string) (string, bool) { return "", false },
	})
	if !ok {
		t.Fatal("fs.path(...) did not resolve to a type")
	}

	got := names(h.(starlsp.MemberProvider).Members(typ))
	for _, want := range []string{"read_text", "write_text", "exists", "parent", "try_read_text"} {
		if _, ok := got[want]; !ok {
			t.Errorf("the %s object is missing %q", typ, want)
		}
	}
	if len(got) < 50 {
		t.Errorf("the %s object exposes only %d members; factory probing looks broken", typ, len(got))
	}
}

func TestModuleNameResolvesToItself(t *testing.T) {
	owner, ok := host(t).(starlsp.TypeResolver).ResolveType(starlsp.TypeRequest{
		Expr:    "fs",
		Resolve: func(string) (string, bool) { return "", false },
	})
	if !ok || owner != "fs" {
		t.Errorf("ResolveType(fs) = %q, %v; want fs, true", owner, ok)
	}
}

func TestUnknownReceiverIsRefused(t *testing.T) {
	_, ok := host(t).(starlsp.TypeResolver).ResolveType(starlsp.TypeRequest{
		Expr:    "mystery",
		Resolve: func(string) (string, bool) { return "", false },
	})
	if ok {
		t.Error("an unknown receiver resolved; completion would offer the wrong list")
	}
}

func TestDocumentedSignaturesAreAttached(t *testing.T) {
	got := names(host(t).(starlsp.MemberProvider).Members("fs"))
	if got["path"].Signature == "" {
		t.Error("fs.path has no signature; the documentation join produced nothing")
	}
	if got["path"].Doc == "" {
		t.Error("fs.path has no prose")
	}
}

func TestLoadResolverRejectsABarePathlessReference(t *testing.T) {
	h := host(t).(starlsp.LoadResolver)

	// The runtime requires a path prefix on a .star reference, so this is not
	// a navigable link either.
	if _, ok := h.ResolveLoad(starlsp.LoadRequest{Target: "greeter.star", From: "/tmp/x/main.star"}); ok {
		t.Error("a bare .star reference resolved; the runtime would reject it")
	}
	// A built-in module has no file behind it.
	if _, ok := h.ResolveLoad(starlsp.LoadRequest{Target: "fs", From: "/tmp/x/main.star"}); ok {
		t.Error("a built-in module resolved to a file")
	}
}

func TestLoadResolverFindsARelativeModule(t *testing.T) {
	dir := t.TempDir()
	moduleDir := dir + "/modules/greeter"
	if err := mkdirAll(moduleDir); err != nil {
		t.Fatal(err)
	}
	entry := moduleDir + "/main.star"
	if err := writeFile(entry, "def greet():\n    pass\n"); err != nil {
		t.Fatal(err)
	}

	got, ok := host(t).(starlsp.LoadResolver).ResolveLoad(starlsp.LoadRequest{
		Target: "./modules/greeter",
		From:   dir + "/main.star",
	})
	if !ok {
		t.Fatal("a relative module directory did not resolve")
	}
	if got != entry {
		t.Errorf("resolved to %q, want the module's main.star at %q", got, entry)
	}
}

func TestEditionDecidesTheSurface(t *testing.T) {
	// The base edition has no k8s module. The all-in-one does. The host takes
	// whichever registry its edition supplies, which is the whole reason the
	// factory is a parameter.
	base := names(kitelsp.NewHost(loader.NewDefaultRegistry).Globals())
	if _, ok := base["k8s"]; ok {
		t.Error("the base edition offered the k8s module")
	}
	if _, ok := base["fs"]; !ok {
		t.Error("the base edition is missing the fs module")
	}
}

// Small filesystem helpers, kept here so the test file states its own needs.

func mkdirAll(dir string) error { return os.MkdirAll(dir, 0o755) }

func writeFile(path, content string) error { return os.WriteFile(path, []byte(content), 0o644) }
