// Package lsp adapts Starkite to the starlsp language server.
//
// The language server itself lives in github.com/M31-Labs/starlsp, which knows
// the Starlark specification and nothing else. This package supplies the parts
// that are specific to Starkite: the module registry as a set of completable
// names, the API reference documents as signatures and prose, the module
// loader's rules for resolving load(), and the runtime's own conventions as
// lints.
//
// Nothing here parses Starlark or speaks the protocol. That division is the
// point: a bug in completion is a bug in this package, and a bug in hover
// positioning is a bug in the server.
package lsp

import (
	"strings"
	"sync"

	"github.com/M31-Labs/starlsp"
	"go.starlark.net/syntax"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/catalog"
)

// Host is the Starkite dialect, as the language server sees it.
type Host struct {
	newRegistry func(*libkite.ModuleConfig) *libkite.Registry

	once sync.Once
	cat  *catalog.Catalog
}

// NewHost builds the Starkite host. The registry factory is the edition's own,
// so kitecloud completes the k8s module and kitecmd does not.
func NewHost(newRegistry func(*libkite.ModuleConfig) *libkite.Registry) *Host {
	return &Host{newRegistry: newRegistry}
}

func (h *Host) Name() string { return "starkite" }

// build introspects the registry once, on first use, so a server that starts
// and is never used pays nothing.
func (h *Host) build() {
	h.once.Do(func() {
		h.cat = catalog.Build(h.newRegistry(&libkite.ModuleConfig{}))
	})
}

func (h *Host) Globals() []starlsp.Symbol {
	h.build()
	syms := h.cat.Globals()
	out := make([]starlsp.Symbol, len(syms))
	for i, s := range syms {
		out[i] = toLspSymbol(s)
	}
	return out
}

func (h *Host) Members(owner string) []starlsp.Symbol {
	h.build()
	syms := h.cat.Members(owner)
	out := make([]starlsp.Symbol, len(syms))
	for i, s := range syms {
		out[i] = toLspSymbol(s)
	}
	return out
}

// ResolveType maps a receiver expression to the module or object type whose
// members it exposes.
func (h *Host) ResolveType(req starlsp.TypeRequest) (string, bool) {
	h.build()
	expr := strings.TrimSpace(req.Expr)
	if expr == "" {
		return "", false
	}

	if h.cat.IsModule(expr) {
		return expr, true
	}

	callee, isCall := calleeOf(expr)
	if !isCall {
		return "", false
	}
	if typ, ok := h.cat.FactoryReturns(callee); ok {
		return typ, true
	}
	// A method call on an object that itself returns an object:
	// fs.path("x").parent.read_text() and similar chains.
	if owner, member, ok := splitLast(callee); ok {
		if base, found := req.Resolve(owner); found {
			if typ, ok := h.cat.FactoryReturns(base + "." + member); ok {
				return typ, true
			}
		}
	}
	return "", false
}

// Document supplies prose and a signature for a name whose value carries
// neither, which is every Starlark builtin.
func (h *Host) Document(owner, name string) (starlsp.Symbol, bool) {
	h.build()
	var sym catalog.Symbol
	var ok bool
	if owner != "" {
		sym, ok = h.cat.LookupMember(owner, name)
	} else {
		sym, ok = h.cat.LookupGlobal(name)
	}
	if !ok {
		return starlsp.Symbol{}, false
	}
	return toLspSymbol(sym), true
}

func toLspSymbol(s catalog.Symbol) starlsp.Symbol {
	return starlsp.Symbol{
		Name:      s.Name,
		Owner:     s.Owner,
		Kind:      toLspKind(s.Kind),
		Doc:       s.Doc,
		Signature: s.Signature,
		Returns:   s.Returns,
		SortText:  s.SortText,
	}
}

func toLspKind(k catalog.Kind) starlsp.Kind {
	switch k {
	case catalog.KindNamespace:
		return starlsp.KindNamespace
	case catalog.KindFunction:
		return starlsp.KindFunction
	case catalog.KindVariable:
		return starlsp.KindVariable
	case catalog.KindProperty:
		return starlsp.KindProperty
	default:
		return starlsp.KindVariable
	}
}

// Lint reports the Starkite conventions the language does not know about.
func (h *Host) Lint(doc *starlsp.Document, file *syntax.File) []starlsp.Diagnostic {
	return h.conventionLints(doc, file)
}

// calleeOf returns the callee text of a call expression.
func calleeOf(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasSuffix(expr, ")") {
		return "", false
	}
	depth := 0
	for i := len(expr) - 1; i >= 0; i-- {
		switch expr[i] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			depth--
			if depth == 0 {
				callee := strings.TrimSpace(expr[:i])
				return callee, callee != ""
			}
		}
	}
	return "", false
}

// splitLast splits "fs.path" into "fs" and "path".
func splitLast(dotted string) (owner, member string, ok bool) {
	dot := strings.LastIndex(dotted, ".")
	if dot <= 0 || dot == len(dotted)-1 {
		return "", "", false
	}
	return dotted[:dot], dotted[dot+1:], true
}

var (
	_ starlsp.Host           = (*Host)(nil)
	_ starlsp.MemberProvider = (*Host)(nil)
	_ starlsp.TypeResolver   = (*Host)(nil)
	_ starlsp.LoadResolver   = (*Host)(nil)
	_ starlsp.Linter         = (*Host)(nil)
	_ starlsp.Documenter     = (*Host)(nil)
)
