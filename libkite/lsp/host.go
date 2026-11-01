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
	"github.com/project-starkite/starkite/libkite/lsp/apidocs"
)

// Host is the Starkite dialect, as the language server sees it.
type Host struct {
	newRegistry func(*libkite.ModuleConfig) *libkite.Registry

	once    sync.Once
	catalog *catalog
	docs    *docIndex
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
		h.docs = newDocIndex(apidocs.Table)
		h.catalog = buildCatalog(h.newRegistry(&libkite.ModuleConfig{}), h.docs)
	})
}

func (h *Host) Globals() []starlsp.Symbol {
	h.build()
	return h.catalog.globals
}

func (h *Host) Members(owner string) []starlsp.Symbol {
	h.build()
	return h.catalog.members[owner]
}

// ResolveType maps a receiver expression to the module or object type whose
// members it exposes.
//
// The inference is deliberately shallow: a module name owns itself, and a call
// to a known factory owns the type that factory returns. That covers how
// Starkite scripts are written. The server has already followed a bare
// identifier to its binding before asking, so `p = fs.path(...)` then `p.`
// arrives here as the call.
func (h *Host) ResolveType(req starlsp.TypeRequest) (string, bool) {
	h.build()
	expr := strings.TrimSpace(req.Expr)
	if expr == "" {
		return "", false
	}

	if h.catalog.isModule(expr) {
		return expr, true
	}

	callee, isCall := calleeOf(expr)
	if !isCall {
		return "", false
	}
	if typ, ok := h.catalog.factoryReturns[callee]; ok {
		return typ, true
	}
	// A method call on an object that itself returns an object:
	// fs.path("x").parent.read_text() and similar chains.
	if owner, member, ok := splitLast(callee); ok {
		if base, found := req.Resolve(owner); found {
			if typ, ok := h.catalog.factoryReturns[base+"."+member]; ok {
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
	entry, ok := h.docs.lookupMember(owner, name)
	if !ok && owner == "" {
		entry, ok = h.docs.lookupGlobal(name)
	}
	if !ok {
		return starlsp.Symbol{}, false
	}
	return starlsp.Symbol{
		Name:      name,
		Owner:     owner,
		Signature: entry.signature,
		Returns:   entry.returns,
		Doc:       entry.doc,
	}, true
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
