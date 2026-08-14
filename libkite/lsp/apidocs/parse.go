// Package apidocs turns Starkite's API reference documents into signature and
// prose entries the language server can attach to hover and signature help.
//
// The reference documents under docs/references/api are written as markdown
// tables in a consistent shape:
//
//	| `fs.path(path)`   | `Path`   | Create a Path from a string |
//	| `p.read_text()`   | `string` | Read file as text           |
//
// The first cell is a call or property, the second its type, the third its
// description. That is the only source in the repository that carries
// parameter names, because a Starlark builtin does not record its own arity.
// The runtime registry supplies the authoritative set of names; these entries
// supply what the names mean.
package apidocs

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one documented call or property.
type Entry struct {
	// Module is the reference file the entry came from, without extension —
	// "fs", "http", "k8s".
	Module string

	// Receiver is the text left of the dot in the first cell. It is the
	// module name for a module-level function ("fs" in `fs.path(...)`), a
	// short variable for an object method ("p" in `p.read_text()`), or empty
	// for a bare global.
	Receiver string

	// Name is the member name — "path", "read_text".
	Name string

	// Signature is the full first cell — "fs.path(path)".
	Signature string

	// Params are the parameter names parsed out of the signature.
	Params []string

	// Returns is the second cell — "Path", "string".
	Returns string

	// Doc is the third cell.
	Doc string

	// IsProperty is true when the first cell carries no parentheses, which
	// marks a property table rather than a method table.
	IsProperty bool
}

// ParseDir reads every markdown file in dir and returns the entries it finds.
func ParseDir(dir string) ([]Entry, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)

	var all []Entry
	for _, path := range matches {
		module := strings.TrimSuffix(filepath.Base(path), ".md")
		if module == "index" {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		entries, err := parseFile(module, f)
		f.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
	}
	return all, nil
}

// columns records what each cell of the current table means. The reference
// documents do not use one fixed column order — some tables lead with Method,
// some with Property, and the description is not always third — so the header
// row is read rather than assumed.
type columns struct {
	returns int    // index of the type/returns column, -1 when absent
	doc     int    // index of the description column, -1 when absent
	subject string // the first column's own header: "method", "property", …
}

func (c columns) valid() bool { return c.returns >= 0 || c.doc >= 0 }

// readHeader interprets a table header row.
func readHeader(cells []string) (columns, bool) {
	cols := columns{returns: -1, doc: -1}
	if len(cells) < 2 {
		return cols, false
	}
	for i, cell := range cells {
		name := strings.ToLower(strings.TrimSpace(cell))
		switch {
		case i == 0:
			cols.subject = name
		case cols.returns < 0 && (name == "type" || name == "returns" || name == "return" || name == "result"):
			cols.returns = i
		case cols.doc < 0 && (name == "description" || name == "meaning" || name == "notes" || name == "purpose"):
			cols.doc = i
		}
	}
	// A header must name its first column with a known subject, otherwise it
	// is a prose table that happens to use pipes.
	switch cols.subject {
	case "method", "property", "function", "field", "attribute", "global", "alias", "constant", "name", "call":
		return cols, true
	}
	return cols, false
}

// isSeparatorRow reports whether a row is the |---|---| rule under a header.
func isSeparatorRow(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		t := strings.TrimSpace(c)
		if t == "" {
			return false
		}
		if strings.Trim(t, ":- ") != "" {
			return false
		}
	}
	return true
}

func parseFile(module string, r interface{ Read([]byte) (int, error) }) ([]Entry, error) {
	var out []Entry
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	inFence := false
	cur := columns{returns: -1, doc: -1}
	inTable := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Never read table syntax out of a fenced code block.
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			inTable = false
			continue
		}

		cells := splitRow(trimmed)
		if isSeparatorRow(cells) {
			continue
		}
		if cols, ok := readHeader(cells); ok {
			cur, inTable = cols, true
			continue
		}
		if !inTable || !cur.valid() {
			continue
		}
		entry, ok := parseRow(module, cells, cur)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out, scanner.Err()
}

// parseRow parses one markdown table row into an entry, using the column
// meanings read from the table's header. It returns false for anything whose
// first cell is not a backticked call or property.
func parseRow(module string, cells []string, cols columns) (Entry, bool) {
	if len(cells) < 2 {
		return Entry{}, false
	}
	// A documented member is always written as code in the first cell.
	if !strings.Contains(cells[0], "`") {
		return Entry{}, false
	}
	first := stripCode(cells[0])
	if first == "" {
		return Entry{}, false
	}

	receiver, name, signature, params, isProperty, ok := splitSignature(first)
	if !ok {
		return Entry{}, false
	}

	entry := Entry{
		Module:     module,
		Receiver:   receiver,
		Name:       name,
		Signature:  signature,
		Params:     params,
		IsProperty: isProperty,
	}
	if cols.returns >= 0 && cols.returns < len(cells) {
		entry.Returns = stripCode(cells[cols.returns])
	}
	if cols.doc >= 0 && cols.doc < len(cells) {
		entry.Doc = strings.TrimSpace(cells[cols.doc])
	}
	// A bare name under a Property or Field heading belongs to the object the
	// section documents, not to the global namespace. Marking the receiver
	// keeps it out of global lookups; the exact object is resolved later from
	// the module's factory return type.
	if entry.Receiver == "" && (cols.subject == "property" || cols.subject == "field" || cols.subject == "attribute") {
		entry.Receiver = ObjectReceiver
	}
	return entry, true
}

// ObjectReceiver marks an entry that belongs to a module's object rather than
// to the module namespace or the global namespace.
const ObjectReceiver = "_obj"

// splitRow splits a markdown table row into its cells.
func splitRow(row string) []string {
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// stripCode removes surrounding backticks and whitespace from a cell.
func stripCode(cell string) string {
	cell = strings.TrimSpace(cell)
	cell = strings.Trim(cell, "`")
	return strings.TrimSpace(cell)
}

// splitSignature decomposes "fs.path(path)" into its receiver, name,
// parameters, and whether it is a property rather than a call.
func splitSignature(sig string) (receiver, name, signature string, params []string, isProperty bool, ok bool) {
	signature = sig

	head := sig
	var argText string
	if open := strings.Index(sig, "("); open >= 0 {
		if !strings.HasSuffix(strings.TrimSpace(sig), ")") {
			return "", "", "", nil, false, false
		}
		head = sig[:open]
		argText = strings.TrimSuffix(sig[open+1:], ")")
	} else {
		isProperty = true
	}

	// Some property tables write the member with a leading dot — ".data".
	head = strings.TrimPrefix(strings.TrimSpace(head), ".")
	if head == "" {
		return "", "", "", nil, false, false
	}
	// Reject prose that happened to be backticked.
	if strings.ContainsAny(head, " \t/\\") {
		return "", "", "", nil, false, false
	}

	if dot := strings.LastIndex(head, "."); dot >= 0 {
		receiver = head[:dot]
		name = head[dot+1:]
	} else {
		name = head
	}
	if name == "" {
		return "", "", "", nil, false, false
	}

	params = parseParams(argText)
	return receiver, name, signature, params, isProperty, true
}

// parseParams splits an argument list, respecting nesting and quotes so that
// a default value containing a comma does not split a parameter in two.
func parseParams(argText string) []string {
	argText = strings.TrimSpace(argText)
	if argText == "" {
		return nil
	}
	var (
		params []string
		depth  int
		quote  rune
		cur    strings.Builder
	)
	flush := func() {
		p := strings.TrimSpace(cur.String())
		cur.Reset()
		if p != "" {
			params = append(params, p)
		}
	}
	for _, r := range argText {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			cur.WriteRune(r)
		case r == '(' || r == '[' || r == '{':
			depth++
			cur.WriteRune(r)
		case r == ')' || r == ']' || r == '}':
			depth--
			cur.WriteRune(r)
		case r == ',' && depth == 0:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return params
}
