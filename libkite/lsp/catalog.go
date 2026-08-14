package lsp

import (
	"sort"
	"strings"
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// symbol is one completable name.
type symbol struct {
	Name      string // "read_text", "path", "fs"
	Qualified string // "fs.path", "Path.read_text", "" for globals
	Owner     string // module or object type that holds it, "" for globals
	Kind      CompletionItemKind
	Detail    string // short right-hand text in the completion list
	Signature string // "fs.path(path)" when documented
	Doc       string // prose from the API reference, when documented
	IsTry     bool   // a try_ variant, which returns a Result
}

// catalog is the completable surface of a Starkite runtime.
//
// Every name in it is read out of a live libkite.Registry rather than written
// by hand, so the catalog cannot drift from the binary it ships in: a module
// added to the registry appears in completion with no second edit. Signatures
// and prose are joined in from the API reference documents, which carry the
// parameter lists that Starlark builtins do not.
type catalog struct {
	// globals are names visible with no qualifier: predeclared builtins,
	// module namespaces, and the global aliases modules export.
	globals []symbol

	// members maps an owner — a module name like "fs", or an object type
	// like "fs.path" — to the names reachable through a dot on it.
	members map[string][]symbol

	// factoryReturns maps a call like "fs.path" to the object type it
	// produces, so completion after `p = fs.path("x")` then `p.` works.
	factoryReturns map[string]string

	// moduleDescriptions holds each module's own Description().
	moduleDescriptions map[string]string
}

// starlarkKeywords are completed alongside the runtime surface. Starlark's
// grammar is fixed, so this list is a language constant rather than drift-prone
// configuration.
var starlarkKeywords = []string{
	"and", "break", "continue", "def", "elif", "else", "for", "if",
	"in", "lambda", "load", "not", "or", "pass", "return", "while",
}

// buildCatalog introspects a registry into a completable surface.
//
// It performs three passes:
//
//  1. Module namespaces and their members, via Registry.LoadAll and each
//     value's AttrNames. TryModule.AttrNames already emits the try_ variants.
//  2. Global aliases and predeclared names, via GetAliases and Predeclared.
//  3. Object methods, by calling each module's factory with a harmless
//     argument and reading AttrNames off the result. This reaches the Path,
//     Response, and source objects that hold most of the real API surface and
//     are not otherwise visible from the module namespace.
func buildCatalog(reg *libkite.Registry, docs *docIndex) *catalog {
	c := &catalog{
		members:            make(map[string][]symbol),
		factoryReturns:     make(map[string]string),
		moduleDescriptions: make(map[string]string),
	}

	loaded, _ := reg.LoadAll() // a load error still yields the modules that worked

	// ---- pass 1: modules and their members ----
	for _, name := range reg.Names() {
		mod := string(name)
		if m, ok := reg.Get(name); ok {
			c.moduleDescriptions[mod] = m.Description()
		}
		c.globals = append(c.globals, symbol{
			Name:   mod,
			Kind:   kindModule,
			Detail: c.moduleDescriptions[mod],
		})

		value, ok := loaded[mod]
		if !ok {
			continue
		}
		c.members[mod] = attrSymbols(value, mod, docs)
	}

	// ---- pass 2: aliases and predeclared names ----
	seen := make(map[string]bool, len(c.globals))
	for _, g := range c.globals {
		seen[g.Name] = true
	}
	for name, value := range reg.GetAliases() {
		if seen[name] {
			continue
		}
		seen[name] = true
		c.globals = append(c.globals, globalSymbol(name, value, docs))
	}
	for name, value := range reg.Predeclared() {
		if seen[name] {
			continue
		}
		seen[name] = true
		c.globals = append(c.globals, globalSymbol(name, value, docs))
	}
	for _, kw := range starlarkKeywords {
		if seen[kw] {
			continue
		}
		seen[kw] = true
		c.globals = append(c.globals, symbol{Name: kw, Kind: kindKeyword, Detail: "keyword"})
	}

	// ---- pass 3: object methods behind factories ----
	c.probeFactories(reg, loaded, docs)

	sort.Slice(c.globals, func(i, j int) bool { return c.globals[i].Name < c.globals[j].Name })
	return c
}

// globalSymbol classifies one top-level name.
func globalSymbol(name string, value starlark.Value, docs *docIndex) symbol {
	s := symbol{Name: name, Kind: kindFunction, IsTry: strings.HasPrefix(name, "try_")}
	if _, ok := value.(*starlark.Builtin); !ok {
		s.Kind = kindVariable
	}
	if d, ok := docs.lookupGlobal(name); ok {
		s.Signature, s.Doc = d.signature, d.doc
		s.Detail = d.returns
	}
	return s
}

// attrSymbols reads the attribute names off a value and classifies each.
func attrSymbols(value starlark.Value, owner string, docs *docIndex) []symbol {
	h, ok := value.(starlark.HasAttrs)
	if !ok {
		return nil
	}
	names := h.AttrNames()
	sort.Strings(names)

	out := make([]symbol, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		// Guard against a module that registers an explicit try_ member and
		// then has the try_ prefix applied a second time. os does exactly
		// this today, which yields try_try_exec and duplicate try_exec.
		if seen[n] || strings.HasPrefix(n, "try_try_") {
			continue
		}
		seen[n] = true

		s := symbol{
			Name:      n,
			Owner:     owner,
			Qualified: owner + "." + n,
			Kind:      kindFunction,
			IsTry:     strings.HasPrefix(n, "try_"),
		}
		if attr, err := h.Attr(n); err == nil && attr != nil {
			if _, isBuiltin := attr.(*starlark.Builtin); !isBuiltin {
				s.Kind = kindProperty
			}
		}
		if d, ok := docs.lookupMember(owner, n); ok {
			s.Signature, s.Doc, s.Detail = d.signature, d.doc, d.returns
		}
		if s.IsTry && s.Detail == "" {
			s.Detail = "Result"
		}
		out = append(out, s)
	}
	return out
}

// factoryProbe describes one safe constructor call.
//
// Each argument is inert: it names a path that is never read, or a URL that is
// never fetched. Starkite's factory functions only record configuration — the
// documentation is explicit that they perform no I/O, and that failures surface
// on the subsequent method call — so probing them has no side effect.
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

// probeFactories calls each known factory once and records the methods on the
// object it returns.
func (c *catalog) probeFactories(reg *libkite.Registry, loaded starlark.StringDict, docs *docIndex) {
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
// misbehaving module cannot take down the server at startup.
func safeCall(thread *starlark.Thread, fn starlark.Value, arg string) (result starlark.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			result, err = nil, errProbePanicked
		}
	}()
	return starlark.Call(thread, fn, starlark.Tuple{starlark.String(arg)}, nil)
}

// errProbePanicked marks a factory probe that panicked rather than returned.
var errProbePanicked = probeError("factory probe panicked")

type probeError string

func (e probeError) Error() string { return string(e) }

// membersOf returns the completable names reachable through a dot on owner.
// owner may be a module name or an object type.
func (c *catalog) membersOf(owner string) []symbol {
	return c.members[owner]
}

// returnTypeOf reports the object type a factory call produces.
func (c *catalog) returnTypeOf(call string) (string, bool) {
	t, ok := c.factoryReturns[call]
	return t, ok
}

// isModule reports whether name is a registered module namespace.
func (c *catalog) isModule(name string) bool {
	_, ok := c.moduleDescriptions[name]
	return ok
}

// lookup finds a symbol by its dotted path, for hover and signature help.
func (c *catalog) lookup(owner, name string) (symbol, bool) {
	if owner == "" {
		for _, g := range c.globals {
			if g.Name == name {
				return g, true
			}
		}
		return symbol{}, false
	}
	for _, m := range c.members[owner] {
		if m.Name == name {
			return m, true
		}
	}
	return symbol{}, false
}

// catalogOnce guards lazy construction so the registry is only loaded when a
// client actually connects.
type catalogOnce struct {
	once sync.Once
	val  *catalog
}

func (c *catalogOnce) get(build func() *catalog) *catalog {
	c.once.Do(func() { c.val = build() })
	return c.val
}
