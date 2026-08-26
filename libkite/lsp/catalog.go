package lsp

import (
	"sort"
	"strings"

	"github.com/M31-Labs/starlsp"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// catalog is the completable surface of a Starkite runtime.
//
// Every name in it is read out of a live libkite.Registry rather than written
// by hand, so the catalog cannot drift from the binary it ships in: a module
// added to the registry appears in completion with no second edit. The API
// reference documents supply the parameter lists and prose that Starlark
// builtins do not carry.
type catalog struct {
	// globals are the names visible with no qualifier: module namespaces and
	// the global aliases modules export.
	globals []starlsp.Symbol

	// members maps an owner — a module name like "fs", or an object type like
	// "fs.path" — to the names reachable through a dot on it.
	members map[string][]starlsp.Symbol

	// factoryReturns maps a call such as "fs.path" to the object type it
	// produces, which is what makes completion work after `p = fs.path(...)`.
	factoryReturns map[string]string

	// modules records which names are module namespaces.
	modules map[string]string
}

func (c *catalog) isModule(name string) bool {
	_, ok := c.modules[name]
	return ok
}

// buildCatalog introspects a registry into a completable surface.
//
// Three passes: module namespaces and their members; global aliases and
// predeclared names; then object methods, reached by calling each module's
// factory with an inert argument and reading AttrNames off the result. The
// third pass is what reaches Path, Response, and the source objects, which
// hold most of the real API surface and are invisible from the module
// namespace.
func buildCatalog(reg *libkite.Registry, docs *docIndex) *catalog {
	c := &catalog{
		members:        make(map[string][]starlsp.Symbol),
		factoryReturns: make(map[string]string),
		modules:        make(map[string]string),
	}

	loaded, _ := reg.LoadAll() // a load error still yields the modules that worked

	// ---- pass 1: modules and their members ----
	for _, name := range reg.Names() {
		module := string(name)
		description := ""
		if m, ok := reg.Get(name); ok {
			description = m.Description()
		}
		c.modules[module] = description
		c.globals = append(c.globals, starlsp.Symbol{
			Name: module,
			Kind: starlsp.KindNamespace,
			Doc:  description,
		})

		if value, ok := loaded[module]; ok {
			c.members[module] = attrSymbols(value, module, docs)
		}
	}

	// ---- pass 2: aliases and predeclared names ----
	seen := make(map[string]bool, len(c.globals))
	for _, g := range c.globals {
		seen[g.Name] = true
	}
	add := func(name string, value starlark.Value) {
		if seen[name] {
			return
		}
		seen[name] = true
		c.globals = append(c.globals, globalSymbol(name, value, docs))
	}
	for name, value := range reg.GetAliases() {
		add(name, value)
	}
	for name, value := range reg.Predeclared() {
		add(name, value)
	}

	// ---- pass 3: object methods behind factories ----
	c.probeFactories(loaded, docs)

	sort.Slice(c.globals, func(i, j int) bool { return c.globals[i].Name < c.globals[j].Name })
	return c
}

// globalSymbol classifies one top-level name.
func globalSymbol(name string, value starlark.Value, docs *docIndex) starlsp.Symbol {
	sym := starlsp.Symbol{Name: name, Kind: starlsp.KindFunction}
	if _, ok := value.(*starlark.Builtin); !ok {
		sym.Kind = starlsp.KindVariable
	}
	if entry, ok := docs.lookupGlobal(name); ok {
		sym.Signature, sym.Returns, sym.Doc = entry.signature, entry.returns, entry.doc
	}
	if strings.HasPrefix(name, "try_") {
		sym.SortText = "9" + name // a fallback is not the first thing anyone wants
		if sym.Returns == "" {
			sym.Returns = "Result"
		}
	}
	return sym
}

// attrSymbols reads the attribute names off a value and classifies each.
func attrSymbols(value starlark.Value, owner string, docs *docIndex) []starlsp.Symbol {
	attrs, ok := value.(starlark.HasAttrs)
	if !ok {
		return nil
	}
	names := attrs.AttrNames()
	sort.Strings(names)

	out := make([]starlsp.Symbol, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		// Guard against a module that registers an explicit try_ member and
		// then has the prefix applied a second time. The os module does this
		// today, which yields try_try_exec and a duplicate try_exec.
		if seen[name] || strings.HasPrefix(name, "try_try_") {
			continue
		}
		seen[name] = true

		sym := starlsp.Symbol{Name: name, Owner: owner, Kind: starlsp.KindFunction}
		if attr, err := attrs.Attr(name); err == nil && attr != nil {
			if _, isBuiltin := attr.(*starlark.Builtin); !isBuiltin {
				sym.Kind = starlsp.KindProperty
			}
		}
		if entry, ok := docs.lookupMember(owner, name); ok {
			sym.Signature, sym.Returns, sym.Doc = entry.signature, entry.returns, entry.doc
		}
		if strings.HasPrefix(name, "try_") {
			sym.SortText = "9" + name
			if sym.Returns == "" {
				sym.Returns = "Result"
			}
		}
		out = append(out, sym)
	}
	return out
}

// factoryProbe describes one safe constructor call.
//
// Each argument is inert: a path that is never read, or a URL that is never
// fetched. Starkite's factory functions only record configuration — the
// documentation is explicit that they perform no input or output, and that
// failures surface on the subsequent method call — so probing has no effect.
type factoryProbe struct {
	module string
	fn     string
	arg    string
}

var factoryProbes = []factoryProbe{
	{"fs", "path", "/nonexistent/kite-lsp-probe"},
	{"json", "file", "/nonexistent/kite-lsp-probe.json"},
	{"yaml", "file", "/nonexistent/kite-lsp-probe.yaml"},
	{"csv", "file", "/nonexistent/kite-lsp-probe.csv"},
	{"zip", "file", "/nonexistent/kite-lsp-probe.zip"},
	{"gzip", "file", "/nonexistent/kite-lsp-probe.gz"},
	{"http", "url", "https://kite-lsp-probe.invalid"},
	{"base64", "text", ""},
	{"hash", "text", ""},
	{"template", "text", ""},
}

func (c *catalog) probeFactories(loaded starlark.StringDict, docs *docIndex) {
	thread := &starlark.Thread{Name: "kite-lsp-catalog"}

	for _, probe := range factoryProbes {
		moduleValue, ok := loaded[probe.module]
		if !ok {
			continue
		}
		attrs, ok := moduleValue.(starlark.HasAttrs)
		if !ok {
			continue
		}
		fn, err := attrs.Attr(probe.fn)
		if err != nil || fn == nil {
			continue
		}
		result, err := safeCall(thread, fn, probe.arg)
		if err != nil || result == nil {
			continue
		}
		objectType := result.Type()
		if objectType == "" {
			continue
		}
		c.factoryReturns[probe.module+"."+probe.fn] = objectType
		if _, done := c.members[objectType]; done {
			continue
		}
		c.members[objectType] = attrSymbols(result, objectType, docs)
	}
}

// safeCall invokes a factory and converts a panic into an error, so one
// misbehaving module cannot take down the server at start-up.
func safeCall(thread *starlark.Thread, fn starlark.Value, arg string) (result starlark.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, errProbePanicked
		}
	}()
	return starlark.Call(thread, fn, starlark.Tuple{starlark.String(arg)}, nil)
}

var errProbePanicked = probeError("factory probe panicked")

type probeError string

func (e probeError) Error() string { return string(e) }
