package lsp

import (
	"github.com/M31-Labs/starlsp"
	"go.starlark.net/syntax"
)

// conventionLints reports the Starkite conventions that the Starlark
// specification knows nothing about.
//
// The language server already reports what the language forbids — a top-level
// if or for, an undefined name. What is left is the runtime's own behaviour:
// the automatic entry point, and the try_ error pattern.
//
// These run on the resolved syntax tree rather than on the tree-sitter tree,
// because the runtime's own detection runs on the same tree. When the file
// does not parse, file is nil and there is nothing honest to say.
func (h *Host) conventionLints(doc *starlsp.Document, file *syntax.File) []starlsp.Diagnostic {
	if file == nil {
		return nil
	}

	var (
		out         []starlsp.Diagnostic
		definesMain bool
		mainCalls   []*syntax.CallExpr
	)

	for _, stmt := range file.Stmts {
		switch s := stmt.(type) {
		case *syntax.DefStmt:
			if s.Name != nil && s.Name.Name == "main" {
				definesMain = true
			}
		case *syntax.ExprStmt:
			call, ok := s.X.(*syntax.CallExpr)
			if !ok {
				continue
			}
			if ident, ok := call.Fn.(*syntax.Ident); ok && ident.Name == "main" {
				mainCalls = append(mainCalls, call)
			}
		}
	}

	// The runtime detects a top-level main() call and skips its automatic
	// invocation, logging that it did. Saying so here explains the log before
	// the user meets it.
	if definesMain {
		for _, call := range mainCalls {
			out = append(out, starlsp.Diagnostic{
				Range:    doc.SpanRange(call),
				Severity: starlsp.SeverityHint,
				Source:   "kite",
				Message:  "the runtime detects this top-level call and skips its automatic main() invocation",
			})
		}
	}

	out = append(out, h.tryResultLints(doc, file)...)
	return out
}

// tryResultLints reports a try_ call whose Result is never checked.
//
// A try_ variant returns a Result rather than raising, which is only useful if
// something reads ok. Binding one and then using it as though it were the
// value is the mistake the pattern invites, and it fails at run time with a
// confusing message about a Result having no such attribute.
func (h *Host) tryResultLints(doc *starlsp.Document, file *syntax.File) []starlsp.Diagnostic {
	var out []starlsp.Diagnostic

	syntax.Walk(file, func(n syntax.Node) bool {
		assign, ok := n.(*syntax.AssignStmt)
		if !ok || assign.Op != syntax.EQ {
			return true
		}
		call, ok := assign.RHS.(*syntax.CallExpr)
		if !ok || !isTryCall(call) {
			return true
		}
		target, ok := assign.LHS.(*syntax.Ident)
		if !ok {
			return true
		}
		if resultIsChecked(file, target) {
			return true
		}
		out = append(out, starlsp.Diagnostic{
			Range:    doc.SpanRange(assign.LHS),
			Severity: starlsp.SeverityWarning,
			Source:   "kite",
			Message: "a try_ call returns a Result: read " + target.Name +
				".ok before " + target.Name + ".value, or call the plain variant to raise on failure",
		})
		return true
	})
	return out
}

// isTryCall reports whether a call names a try_ variant.
func isTryCall(call *syntax.CallExpr) bool {
	switch fn := call.Fn.(type) {
	case *syntax.Ident:
		return hasTryPrefix(fn.Name)
	case *syntax.DotExpr:
		return fn.Name != nil && hasTryPrefix(fn.Name.Name)
	}
	return false
}

func hasTryPrefix(name string) bool {
	return len(name) > 4 && name[:4] == "try_"
}

// resultIsChecked reports whether anything reads .ok on the bound name.
//
// Reading .ok is the contract. Reading .error or passing the Result on are
// also legitimate, so both count as checked; only a binding nothing inspects
// is reported.
func resultIsChecked(file *syntax.File, target *syntax.Ident) bool {
	checked := false
	syntax.Walk(file, func(n syntax.Node) bool {
		if checked {
			return false
		}
		dot, ok := n.(*syntax.DotExpr)
		if !ok || dot.Name == nil {
			return true
		}
		ident, ok := dot.X.(*syntax.Ident)
		if !ok || ident.Name != target.Name {
			return true
		}
		switch dot.Name.Name {
		case "ok", "error":
			checked = true
			return false
		}
		return true
	})
	return checked
}
