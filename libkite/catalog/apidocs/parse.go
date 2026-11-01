// Package apidocs turns Starkite's API reference documents into signature and
// prose entries the language server and documentation engines can attach.
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
	"io"
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

// columns records what each cell of the current table means.
type columns struct {
	returns int    // index of the type/returns column, -1 when absent
	doc     int    // index of the description column, -1 when absent
	subject string // the first column's own header: "method", "property", …
}

func (c columns) valid() bool { return c.returns >= 0 || c.doc >= 0 }

// readHeader interprets a table header row.
func readHeader(cells []string) (columns, bool) {
	if len(cells) < 2 {
		return columns{}, false
	}
	cols := columns{returns: -1, doc: -1}
	for i, cell := range cells {
		h := strings.ToLower(strings.TrimSpace(cell))
		switch {
		case i == 0:
			cols.subject = h
		case cols.returns < 0 && (h == "type" || h == "returns" || h == "return type"):
			cols.returns = i
		case cols.doc < 0 && (h == "description" || h == "details" || h == "summary"):
			cols.doc = i
		}
	}
	if cols.doc < 0 && len(cells) >= 3 && cols.returns == 1 {
		cols.doc = 2
	}
	return cols, cols.valid()
}

func parseFile(module string, r io.Reader) ([]Entry, error) {
	s := bufio.NewScanner(r)
	var (
		entries []Entry
		cols    columns
		inTable bool
	)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			inTable = false
			continue
		}

		cells := splitRow(line)
		if len(cells) == 0 {
			inTable = false
			continue
		}

		if isSeparatorRow(cells) {
			continue
		}

		if !inTable {
			if c, ok := readHeader(cells); ok {
				cols = c
				inTable = true
				continue
			}
		}

		entry, ok := parseRow(module, cols, cells)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries, s.Err()
}

func parseRow(module string, cols columns, cells []string) (Entry, bool) {
	if len(cells) == 0 {
		return Entry{}, false
	}
	rawFirst := cleanCell(cells[0])
	if rawFirst == "" {
		return Entry{}, false
	}

	receiver, name, params, isProperty := parseSignature(rawFirst)
	if name == "" {
		return Entry{}, false
	}

	var returns string
	if cols.returns >= 0 && cols.returns < len(cells) {
		returns = cleanCell(cells[cols.returns])
	}
	var doc string
	if cols.doc >= 0 && cols.doc < len(cells) {
		doc = strings.TrimSpace(cells[cols.doc])
	}

	return Entry{
		Module:     module,
		Receiver:   receiver,
		Name:       name,
		Signature:  rawFirst,
		Params:     params,
		Returns:    returns,
		Doc:        doc,
		IsProperty: isProperty,
	}, true
}

func parseSignature(raw string) (receiver, name string, params []string, isProperty bool) {
	open := strings.Index(raw, "(")
	close := strings.LastIndex(raw, ")")
	if open < 0 || close < open {
		isProperty = true
		receiver, name = splitMember(raw)
		return receiver, name, nil, isProperty
	}

	receiver, name = splitMember(raw[:open])
	params = splitParams(raw[open+1 : close])
	return receiver, name, params, false
}

func splitMember(head string) (receiver, name string) {
	head = strings.TrimSpace(head)
	dot := strings.LastIndex(head, ".")
	if dot < 0 {
		return "", head
	}
	return head[:dot], head[dot+1:]
}

func splitParams(inside string) []string {
	inside = strings.TrimSpace(inside)
	if inside == "" {
		return nil
	}
	var (
		params []string
		start  int
		depth  int
	)
	for i := 0; i < len(inside); i++ {
		switch inside[i] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			depth--
		case ',':
			if depth == 0 {
				if p := cleanParam(inside[start:i]); p != "" {
					params = append(params, p)
				}
				start = i + 1
			}
		}
	}
	if p := cleanParam(inside[start:]); p != "" {
		params = append(params, p)
	}
	return params
}

func cleanParam(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "[]*")
	raw = strings.TrimSpace(raw)
	if eq := strings.Index(raw, "="); eq >= 0 {
		raw = strings.TrimSpace(raw[:eq])
	}
	if colon := strings.Index(raw, ":"); colon >= 0 {
		raw = strings.TrimSpace(raw[:colon])
	}
	return raw
}

func cleanCell(cell string) string {
	cell = strings.TrimSpace(cell)
	cell = strings.TrimPrefix(cell, "`")
	cell = strings.TrimSuffix(cell, "`")
	return strings.TrimSpace(cell)
}

func splitRow(line string) []string {
	trimmed := strings.Trim(line, "|")
	parts := strings.Split(trimmed, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isSeparatorRow(cells []string) bool {
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		cleaned := strings.Trim(c, "-: ")
		if cleaned != "" {
			return false
		}
	}
	return true
}
