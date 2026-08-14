package lsp

import (
	"sort"
	"strings"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// This file joins the two halves. Each handler asks the structural half where
// the caret is and the semantic half what exists there.

// ---------- completion ----------

func (s *Server) complete(d *document, pos Position) completionList {
	offset := d.offsetOf(pos)
	ctx := s.contextAt(d, offset)

	if ctx.InComment {
		return completionList{}
	}
	if ctx.InString {
		return s.completeInString(d, ctx)
	}

	cat := s.catalog()
	var items []CompletionItem

	if ctx.AfterDot {
		owner, ok := s.resolveOwner(d, ctx.Receiver)
		if !ok {
			// The receiver is not a name the runtime knows. Offering the whole
			// global namespace here would be worse than offering nothing.
			return completionList{}
		}
		for _, sym := range cat.membersOf(owner) {
			items = append(items, completionItemFor(sym))
		}
		return completionList{Items: items}
	}

	for _, sym := range cat.globals {
		items = append(items, completionItemFor(sym))
	}
	items = append(items, s.documentLocals(d, offset)...)
	return completionList{Items: items}
}

// completeInString offers module identities inside a load() reference and
// nothing anywhere else, because identifier completion inside a string literal
// is noise.
func (s *Server) completeInString(d *document, ctx cursorContext) completionList {
	if ctx.Call == nil {
		return completionList{}
	}
	fn := childByField(ctx.Call, "function", 0, s.lang)
	if fn == nil || d.nodeText(fn) != "load" || ctx.ArgIndex != 0 {
		return completionList{}
	}
	var items []CompletionItem
	for _, identity := range s.installedModules() {
		items = append(items, CompletionItem{
			Label:  identity,
			Kind:   kindModule,
			Detail: "installed module",
		})
	}
	return completionList{Items: items}
}

// documentLocals offers the names bound in this file, which the registry
// cannot know about.
func (s *Server) documentLocals(d *document, offset int) []CompletionItem {
	if d.tree == nil {
		return nil
	}
	seen := make(map[string]bool)
	var items []CompletionItem

	for _, sym := range s.documentSymbols(d) {
		if seen[sym.Name] {
			continue
		}
		seen[sym.Name] = true
		kind := kindVariable
		detail := "defined in this file"
		if sym.Kind == symbolFunction {
			kind = kindFunction
			detail = "def " + sym.Name + sym.Detail
		}
		items = append(items, CompletionItem{
			Label:    sym.Name,
			Kind:     kind,
			Detail:   detail,
			SortText: "0" + sym.Name, // local names sort above the runtime surface
		})
	}
	return items
}

func completionItemFor(sym symbol) CompletionItem {
	item := CompletionItem{
		Label:  sym.Name,
		Kind:   sym.Kind,
		Detail: sym.Detail,
	}
	if sym.Signature != "" {
		item.Detail = sym.Signature
		if sym.Detail != "" {
			item.Detail += " → " + sym.Detail
		}
	}
	if sym.Doc != "" {
		item.Documentation = &MarkupContent{Kind: "markdown", Value: sym.Doc}
	}
	// A try_ variant is a fallback, not the first thing anyone wants to see.
	if sym.IsTry {
		item.SortText = "9" + sym.Name
	}
	return item
}

// installedModules lists the identities present in the module cache, for
// completion inside a load() reference.
func (s *Server) installedModules() []string {
	root := moduleCacheRoot()
	if root == "" {
		return nil
	}
	entries, err := readDirNames(root)
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, name := range entries {
		base, _ := splitModuleRev(name)
		if base == "" || seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
	}
	sort.Strings(out)
	return out
}

// ---------- hover ----------

func (s *Server) hover(d *document, pos Position) *hoverResult {
	offset := d.offsetOf(pos)
	if d.tree == nil {
		return nil
	}
	node := nodeAt(d.tree.RootNode(), offset)
	if node == nil {
		return nil
	}

	owner, name, rng, ok := s.nameUnderCursor(d, node)
	if !ok {
		return nil
	}
	sym, found := s.catalog().lookup(owner, name)
	if !found {
		return nil
	}
	return &hoverResult{
		Contents: MarkupContent{Kind: "markdown", Value: renderHover(sym)},
		Range:    &rng,
	}
}

// nameUnderCursor identifies the member the caret sits on and what owns it.
func (s *Server) nameUnderCursor(d *document, node *gotreesitter.Node) (owner, name string, rng Range, ok bool) {
	if node.Type(s.lang) != ntIdentifier {
		return "", "", Range{}, false
	}
	name = d.nodeText(node)
	rng = d.nodeRange(node)

	parent := node.Parent()
	if parent != nil && parent.Type(s.lang) == ntAttribute {
		object := childByField(parent, "object", 0, s.lang)
		// The caret is on the member, not on the object it hangs off.
		if object != nil && object.StartByte() != node.StartByte() {
			if resolved, found := s.resolveOwner(d, d.nodeText(object)); found {
				return resolved, name, rng, true
			}
			return "", "", Range{}, false
		}
	}
	return "", name, rng, true
}

func renderHover(sym symbol) string {
	var b strings.Builder
	signature := sym.Signature
	if signature == "" {
		if sym.Qualified != "" {
			signature = sym.Qualified
		} else {
			signature = sym.Name
		}
	}
	b.WriteString("```python\n")
	b.WriteString(signature)
	if sym.Detail != "" && !strings.Contains(signature, "→") {
		b.WriteString("  → " + sym.Detail)
	}
	b.WriteString("\n```")
	if sym.Doc != "" {
		b.WriteString("\n\n" + sym.Doc)
	}
	if sym.IsTry {
		b.WriteString("\n\nReturns a `Result` — check `.ok`, then read `.value` or `.error`.")
	}
	return b.String()
}

// ---------- signature help ----------

func (s *Server) signatureHelp(d *document, pos Position) *signatureHelp {
	offset := d.offsetOf(pos)
	ctx := s.contextAt(d, offset)
	if ctx.Call == nil {
		return nil
	}
	callee := childByField(ctx.Call, "function", 0, s.lang)
	if callee == nil {
		return nil
	}

	owner, name := "", d.nodeText(callee)
	if callee.Type(s.lang) == ntAttribute {
		object := childByField(callee, "object", 0, s.lang)
		member := childByField(callee, "attribute", 1, s.lang)
		if object == nil || member == nil {
			return nil
		}
		resolved, found := s.resolveOwner(d, d.nodeText(object))
		if !found {
			return nil
		}
		owner, name = resolved, d.nodeText(member)
	}

	sym, found := s.catalog().lookup(owner, name)
	if !found || sym.Signature == "" {
		return nil
	}
	info := SignatureInformation{Label: sym.Signature}
	if sym.Doc != "" {
		info.Documentation = &MarkupContent{Kind: "markdown", Value: sym.Doc}
	}
	for _, p := range signatureParams(sym.Signature) {
		info.Parameters = append(info.Parameters, ParameterInformation{Label: p})
	}

	active := ctx.ArgIndex
	if active < 0 {
		active = 0
	}
	if active >= len(info.Parameters) && len(info.Parameters) > 0 {
		active = len(info.Parameters) - 1
	}
	return &signatureHelp{
		Signatures:      []SignatureInformation{info},
		ActiveSignature: 0,
		ActiveParameter: active,
	}
}

// signatureParams splits a rendered signature back into parameter labels.
func signatureParams(signature string) []string {
	open := strings.Index(signature, "(")
	if open < 0 || !strings.HasSuffix(signature, ")") {
		return nil
	}
	return splitArgs(signature[open+1 : len(signature)-1])
}

func splitArgs(argText string) []string {
	argText = strings.TrimSpace(argText)
	if argText == "" {
		return nil
	}
	var (
		out   []string
		depth int
		cur   strings.Builder
	)
	flush := func() {
		if p := strings.TrimSpace(cur.String()); p != "" {
			out = append(out, p)
		}
		cur.Reset()
	}
	for _, r := range argText {
		switch r {
		case '(', '[', '{':
			depth++
			cur.WriteRune(r)
		case ')', ']', '}':
			depth--
			cur.WriteRune(r)
		case ',':
			if depth == 0 {
				flush()
				continue
			}
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// ---------- semantic tokens ----------

// semanticTokenTypes is the legend sent at initialize. The order is the wire
// encoding, so it must not be reordered without changing the constants below.
var semanticTokenTypes = []string{
	"namespace", "function", "method", "variable", "parameter",
	"property", "keyword", "string", "number", "comment",
}

const (
	stNamespace = iota
	stFunction
	stMethod
	stVariable
	stParameter
	stProperty
	stKeyword
	stString
	stNumber
	stComment
)

// semanticTokensFor encodes the whole document.
//
// The classification uses the runtime catalog rather than the grammar's
// highlight query alone, so a module namespace reads as a namespace and a
// registry member reads as a method — a distinction a generic Starlark
// highlighter cannot make.
func (s *Server) semanticTokensFor(d *document) semanticTokens {
	if d.tree == nil {
		return semanticTokens{Data: []uint32{}}
	}
	type token struct {
		line, col, length, kind int
	}
	var tokens []token
	cat := s.catalog()

	walk(d.tree.RootNode(), func(n *gotreesitter.Node) bool {
		if !n.IsNamed() {
			return true
		}
		kind := -1
		switch n.Type(s.lang) {
		case ntComment:
			kind = stComment
		case ntString:
			kind = stString
		case "integer", "float":
			kind = stNumber
		case ntIdentifier:
			kind = s.classifyIdentifier(d, n, cat)
		}
		if kind < 0 {
			return true
		}
		start, end := n.StartPoint(), n.EndPoint()
		// A multi-line token cannot be encoded as one entry; skip it rather
		// than emit a length that runs past the end of its line.
		if start.Row != end.Row {
			return true
		}
		tokens = append(tokens, token{
			line:   int(start.Row),
			col:    int(start.Column),
			length: int(end.Column - start.Column),
			kind:   kind,
		})
		return true
	})

	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		return tokens[i].col < tokens[j].col
	})

	data := make([]uint32, 0, len(tokens)*5)
	prevLine, prevCol := 0, 0
	for _, t := range tokens {
		deltaLine := t.line - prevLine
		deltaCol := t.col
		if deltaLine == 0 {
			deltaCol = t.col - prevCol
		}
		data = append(data, uint32(deltaLine), uint32(deltaCol), uint32(t.length), uint32(t.kind), 0)
		prevLine, prevCol = t.line, t.col
	}
	return semanticTokens{Data: data}
}

// classifyIdentifier decides what an identifier means, preferring what the
// runtime says over what the shape suggests.
func (s *Server) classifyIdentifier(d *document, n *gotreesitter.Node, cat *catalog) int {
	name := d.nodeText(n)
	if name == "" {
		return -1
	}
	parent := n.Parent()

	if parent != nil {
		switch parent.Type(s.lang) {
		case ntFuncDef:
			if name := childByField(parent, "name", 0, s.lang); name != nil && name.StartByte() == n.StartByte() {
				return stFunction
			}
		case "parameters":
			return stParameter
		case ntAttribute:
			object := childByField(parent, "object", 0, s.lang)
			if object != nil && object.StartByte() == n.StartByte() {
				if cat.isModule(name) {
					return stNamespace
				}
				return stVariable
			}
			return stMethod
		}
	}
	if cat.isModule(name) {
		return stNamespace
	}
	if sym, ok := cat.lookup("", name); ok {
		if sym.Kind == kindKeyword {
			return stKeyword
		}
		return stFunction
	}
	return stVariable
}
