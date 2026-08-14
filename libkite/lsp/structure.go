package lsp

import (
	"strings"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This file is the structural half of the server. Everything here reads the
// tree-sitter tree and nothing here decides whether a script is correct. It
// runs on every keystroke and must stay useful while the buffer is broken,
// which is exactly what the tree-sitter parse guarantees and what the Starlark
// parser cannot offer.

// Starlark node types this server reads. They are grammar constants, verified
// against the checked-in starlark grammar rather than assumed.
const (
	ntModule       = "module"
	ntExprStmt     = "expression_statement"
	ntAssignment   = "assignment"
	ntCall         = "call"
	ntAttribute    = "attribute"
	ntIdentifier   = "identifier"
	ntArgumentList = "argument_list"
	ntString       = "string"
	ntStringStart  = "string_start"
	ntStringEnd    = "string_end"
	ntFuncDef      = "function_definition"
	ntBlock        = "block"
	ntIfStmt       = "if_statement"
	ntForStmt      = "for_statement"
	ntWhileStmt    = "while_statement"
	ntComment      = "comment"
)

// childByField returns a node's field-named child, falling back to a positional
// named child when the grammar does not expose the field.
func childByField(n *gotreesitter.Node, field string, fallback int, lang *gotreesitter.Language) *gotreesitter.Node {
	if n == nil {
		return nil
	}
	if c := n.ChildByFieldName(field, lang); c != nil {
		return c
	}
	if fallback >= 0 && fallback < n.NamedChildCount() {
		return n.NamedChild(fallback)
	}
	return nil
}

// namedChildrenOfType returns every direct named child with the given type.
func namedChildrenOfType(n *gotreesitter.Node, typ string, lang *gotreesitter.Language) []*gotreesitter.Node {
	var out []*gotreesitter.Node
	if n == nil {
		return out
	}
	for i := 0; i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c != nil && c.Type(lang) == typ {
			out = append(out, c)
		}
	}
	return out
}

// walk visits every node depth first. Returning false from fn skips a subtree.
func walk(n *gotreesitter.Node, fn func(*gotreesitter.Node) bool) {
	if n == nil || !fn(n) {
		return
	}
	for i := 0; i < n.ChildCount(); i++ {
		walk(n.Child(i), fn)
	}
}

// nodeAt returns the innermost named node covering the byte offset.
func nodeAt(root *gotreesitter.Node, offset int) *gotreesitter.Node {
	var best *gotreesitter.Node
	walk(root, func(n *gotreesitter.Node) bool {
		start, end := int(n.StartByte()), int(n.EndByte())
		if offset < start || offset > end {
			return false
		}
		if n.IsNamed() {
			best = n
		}
		return true
	})
	return best
}

// ---------- document symbols ----------

// documentSymbols builds the outline: every function definition, plus the
// top-level bindings that make a Starkite script readable at a glance.
func (s *Server) documentSymbols(d *document) []DocumentSymbol {
	if d.tree == nil {
		return nil
	}
	root := d.tree.RootNode()
	if root == nil {
		return nil
	}
	var out []DocumentSymbol
	for i := 0; i < root.NamedChildCount(); i++ {
		child := root.NamedChild(i)
		if child == nil {
			continue
		}
		switch child.Type(s.lang) {
		case ntFuncDef:
			out = append(out, s.functionSymbol(d, child))
		case ntExprStmt:
			for _, assign := range namedChildrenOfType(child, ntAssignment, s.lang) {
				if sym, ok := s.assignmentSymbol(d, assign); ok {
					out = append(out, sym)
				}
			}
		}
	}
	return out
}

func (s *Server) functionSymbol(d *document, fn *gotreesitter.Node) DocumentSymbol {
	name := childByField(fn, "name", 0, s.lang)
	params := childByField(fn, "parameters", 1, s.lang)

	detail := ""
	if params != nil {
		detail = d.nodeText(params)
	}
	sym := DocumentSymbol{
		Name:           d.nodeText(name),
		Detail:         detail,
		Kind:           symbolFunction,
		Range:          d.nodeRange(fn),
		SelectionRange: d.nodeRange(name),
	}
	if sym.Name == "" {
		sym.Name = "<anonymous>"
		sym.SelectionRange = sym.Range
	}
	// Nested definitions are children in the outline.
	if body := childByField(fn, "body", -1, s.lang); body != nil {
		for _, nested := range namedChildrenOfType(body, ntFuncDef, s.lang) {
			sym.Children = append(sym.Children, s.functionSymbol(d, nested))
		}
	}
	return sym
}

func (s *Server) assignmentSymbol(d *document, assign *gotreesitter.Node) (DocumentSymbol, bool) {
	target := childByField(assign, "left", 0, s.lang)
	if target == nil || target.Type(s.lang) != ntIdentifier {
		return DocumentSymbol{}, false
	}
	name := d.nodeText(target)
	if name == "" {
		return DocumentSymbol{}, false
	}
	kind := symbolVariable
	if name == strings.ToUpper(name) {
		kind = symbolConstant
	}
	return DocumentSymbol{
		Name:           name,
		Kind:           kind,
		Range:          d.nodeRange(assign),
		SelectionRange: d.nodeRange(target),
	}, true
}

// ---------- folding ----------

// foldingRanges folds every suite: function bodies, loops, and conditionals.
func (s *Server) foldingRanges(d *document) []FoldingRange {
	if d.tree == nil {
		return nil
	}
	var out []FoldingRange
	walk(d.tree.RootNode(), func(n *gotreesitter.Node) bool {
		switch n.Type(s.lang) {
		case ntBlock:
			start := int(n.StartPoint().Row)
			end := int(n.EndPoint().Row)
			// A block's fold starts on the line that introduces it.
			if start > 0 {
				start--
			}
			if end > start {
				out = append(out, FoldingRange{StartLine: start, EndLine: end})
			}
		case ntComment:
			// Consecutive comment lines fold as one region; handled below.
		}
		return true
	})
	out = append(out, s.commentFolds(d)...)
	return out
}

// commentFolds groups runs of two or more consecutive comment lines.
func (s *Server) commentFolds(d *document) []FoldingRange {
	if d.tree == nil {
		return nil
	}
	var comments []*gotreesitter.Node
	walk(d.tree.RootNode(), func(n *gotreesitter.Node) bool {
		if n.Type(s.lang) == ntComment {
			comments = append(comments, n)
		}
		return true
	})

	var out []FoldingRange
	i := 0
	for i < len(comments) {
		startRow := int(comments[i].StartPoint().Row)
		endRow := startRow
		j := i + 1
		for j < len(comments) && int(comments[j].StartPoint().Row) == endRow+1 {
			endRow = int(comments[j].StartPoint().Row)
			j++
		}
		if endRow > startRow {
			out = append(out, FoldingRange{StartLine: startRow, EndLine: endRow, Kind: "comment"})
		}
		i = j
	}
	return out
}

// ---------- structural diagnostics ----------

// structuralDiagnostics reports what the tree alone can prove, which is what
// keeps the editor useful between keystrokes. Nothing here claims a script is
// semantically wrong; the authoritative pass does that.
func (s *Server) structuralDiagnostics(d *document) []Diagnostic {
	if d.tree == nil {
		return nil
	}
	root := d.tree.RootNode()
	var out []Diagnostic

	// Only report tree-sitter's own error nodes while the buffer does not
	// parse for the real parser. Once it parses, starlark-go's message is
	// better and this one would be a duplicate.
	if root.HasError() {
		walk(root, func(n *gotreesitter.Node) bool {
			switch {
			case n.IsError():
				out = append(out, Diagnostic{
					Range:    d.nodeRange(n),
					Severity: severityError,
					Source:   "kite (syntax)",
					Message:  "unexpected input",
				})
				return false
			case n.IsMissing():
				out = append(out, Diagnostic{
					Range:    d.nodeRange(n),
					Severity: severityError,
					Source:   "kite (syntax)",
					Message:  "missing " + n.Type(s.lang),
				})
			}
			return true
		})
	}

	out = append(out, s.conventionDiagnostics(d)...)
	return out
}

// conventionDiagnostics reports the two Starkite conventions that are
// detectable from shape alone.
func (s *Server) conventionDiagnostics(d *document) []Diagnostic {
	root := d.tree.RootNode()
	if root == nil || root.Type(s.lang) != ntModule {
		return nil
	}
	var out []Diagnostic

	definesMain := false
	for _, fn := range namedChildrenOfType(root, ntFuncDef, s.lang) {
		if d.nodeText(childByField(fn, "name", 0, s.lang)) == "main" {
			definesMain = true
			break
		}
	}

	for i := 0; i < root.NamedChildCount(); i++ {
		stmt := root.NamedChild(i)
		if stmt == nil {
			continue
		}
		switch stmt.Type(s.lang) {
		case ntIfStmt, ntForStmt, ntWhileStmt:
			// Starlark restricts if and for to function bodies. The runtime
			// rejects this; saying so while typing saves a run.
			out = append(out, Diagnostic{
				Range:    d.nodeRange(stmt),
				Severity: severityError,
				Source:   "kite",
				Message: "Starlark allows " + strings.TrimSuffix(stmt.Type(s.lang), "_statement") +
					" statements only inside a function body — move this into a function such as main()",
			})
		case ntExprStmt:
			if !definesMain {
				continue
			}
			// The runtime skips its automatic entry-point call when the
			// script calls main() itself, and logs that it did. Surfacing it
			// here explains the log before the user sees it.
			for _, call := range namedChildrenOfType(stmt, ntCall, s.lang) {
				fn := childByField(call, "function", 0, s.lang)
				if fn != nil && fn.Type(s.lang) == ntIdentifier && d.nodeText(fn) == "main" {
					out = append(out, Diagnostic{
						Range:    d.nodeRange(call),
						Severity: severityHint,
						Source:   "kite",
						Message:  "the runtime detects this top-level call and skips its automatic main() invocation",
					})
				}
			}
		}
	}
	return out
}

// ---------- document links ----------

// documentLinks turns each load() target into a link. Resolution of the target
// to a path is the semantic half's job; this half only finds the literals.
func (s *Server) documentLinks(d *document) []DocumentLink {
	if d.tree == nil {
		return nil
	}
	var out []DocumentLink
	for _, call := range s.loadCalls(d) {
		target := s.loadTargetNode(d, call)
		if target == nil {
			continue
		}
		literal := stringLiteralText(d, target, s.lang)
		if literal == "" {
			continue
		}
		link := DocumentLink{
			Range:   d.nodeRange(target),
			Tooltip: "kite module: " + literal,
		}
		if resolved, ok := s.resolveLoadTarget(d, literal); ok {
			link.Target = pathToURI(resolved)
		}
		out = append(out, link)
	}
	return out
}

// loadCalls returns every load() call expression in the document.
func (s *Server) loadCalls(d *document) []*gotreesitter.Node {
	var out []*gotreesitter.Node
	walk(d.tree.RootNode(), func(n *gotreesitter.Node) bool {
		if n.Type(s.lang) != ntCall {
			return true
		}
		fn := childByField(n, "function", 0, s.lang)
		if fn != nil && fn.Type(s.lang) == ntIdentifier && d.nodeText(fn) == "load" {
			out = append(out, n)
		}
		return true
	})
	return out
}

// loadTargetNode returns the first argument of a load() call, which is the
// module reference.
func (s *Server) loadTargetNode(d *document, call *gotreesitter.Node) *gotreesitter.Node {
	args := childByField(call, "arguments", 1, s.lang)
	if args == nil {
		return nil
	}
	for i := 0; i < args.NamedChildCount(); i++ {
		c := args.NamedChild(i)
		if c != nil && c.Type(s.lang) == ntString {
			return c
		}
	}
	return nil
}

// stringLiteralText returns a string node's content without its quotes.
func stringLiteralText(d *document, n *gotreesitter.Node, lang *gotreesitter.Language) string {
	if n == nil || n.Type(lang) != ntString {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c == nil {
			continue
		}
		switch c.Type(lang) {
		case ntStringStart, ntStringEnd:
			continue
		default:
			sb.WriteString(d.nodeText(c))
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}
	// A grammar that does not split the literal leaves the quotes on.
	return strings.Trim(d.nodeText(n), `"'`)
}
