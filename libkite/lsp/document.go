package lsp

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// document holds one open buffer, its line index, and its parse tree.
//
// The tree is produced by gotreesitter and is always available, even when the
// buffer does not parse as valid Starlark. That is the whole reason the
// structural half of this server uses tree-sitter: a language server is most
// useful exactly when the buffer is half-typed.
type document struct {
	uri     string
	path    string // filesystem path, empty for non-file URIs
	version int
	text    []byte

	// lineStarts[i] is the byte offset where line i begins.
	lineStarts []int

	tree *gotreesitter.Tree
}

// documentStore is the set of open buffers. Every exported method is safe for
// concurrent use; the JSON-RPC loop is single-reader but replies may race.
type documentStore struct {
	mu   sync.RWMutex
	docs map[string]*document

	parserMu sync.Mutex
	parser   *gotreesitter.Parser
	lang     *gotreesitter.Language
}

func newDocumentStore(lang *gotreesitter.Language) *documentStore {
	return &documentStore{
		docs:   make(map[string]*document),
		parser: gotreesitter.NewParser(lang),
		lang:   lang,
	}
}

// open records a newly opened buffer and parses it.
func (s *documentStore) open(uri string, version int, text string) *document {
	d := &document{
		uri:     uri,
		path:    uriToPath(uri),
		version: version,
		text:    []byte(text),
	}
	d.indexLines()
	s.reparse(d)

	s.mu.Lock()
	s.docs[uri] = d
	s.mu.Unlock()
	return d
}

// change applies edits to an open buffer and reparses it. A change with no
// range replaces the whole document.
func (s *documentStore) change(uri string, version int, changes []contentChange) *document {
	s.mu.Lock()
	d, ok := s.docs[uri]
	s.mu.Unlock()
	if !ok {
		return nil
	}

	for _, ch := range changes {
		if ch.Range == nil {
			d.text = []byte(ch.Text)
			d.indexLines()
			continue
		}
		start := d.offsetOf(ch.Range.Start)
		end := d.offsetOf(ch.Range.End)
		if start > end {
			start, end = end, start
		}
		next := make([]byte, 0, len(d.text)-(end-start)+len(ch.Text))
		next = append(next, d.text[:start]...)
		next = append(next, ch.Text...)
		next = append(next, d.text[end:]...)
		d.text = next
		d.indexLines()
	}
	d.version = version
	s.reparse(d)
	return d
}

func (s *documentStore) close(uri string) {
	s.mu.Lock()
	delete(s.docs, uri)
	s.mu.Unlock()
}

func (s *documentStore) get(uri string) (*document, bool) {
	s.mu.RLock()
	d, ok := s.docs[uri]
	s.mu.RUnlock()
	return d, ok
}

// reparse rebuilds the tree for d.
//
// This is a full reparse rather than an incremental one. Starlark scripts are
// small — the largest in the Starkite repository is a few hundred lines — and
// a full parse stays well inside a keystroke budget. Wiring Tree.Edit for
// incremental reuse is a contained follow-up if profiling ever asks for it.
func (s *documentStore) reparse(d *document) {
	s.parserMu.Lock()
	defer s.parserMu.Unlock()
	tree, err := s.parser.Parse(d.text)
	if err != nil {
		// A parser error is not a syntax error: the tree-sitter parse is
		// error tolerant and returns a tree with ERROR nodes instead. Keep
		// the previous tree so structural features degrade rather than fail.
		return
	}
	d.tree = tree
}

// indexLines recomputes the line offset table.
func (d *document) indexLines() {
	starts := d.lineStarts[:0]
	if starts == nil {
		starts = make([]int, 0, 256)
	}
	starts = append(starts, 0)
	for i, b := range d.text {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	d.lineStarts = starts
}

// lineCount returns the number of lines in the document.
func (d *document) lineCount() int { return len(d.lineStarts) }

// lineText returns the bytes of line n without its terminator.
func (d *document) lineText(n int) []byte {
	if n < 0 || n >= len(d.lineStarts) {
		return nil
	}
	start := d.lineStarts[n]
	end := len(d.text)
	if n+1 < len(d.lineStarts) {
		end = d.lineStarts[n+1]
	}
	line := d.text[start:end]
	return []byte(strings.TrimRight(string(line), "\r\n"))
}

// offsetOf converts an LSP position — line plus UTF-16 code-unit column — into
// a byte offset into the document.
func (d *document) offsetOf(p Position) int {
	if p.Line < 0 {
		return 0
	}
	if p.Line >= len(d.lineStarts) {
		return len(d.text)
	}
	lineStart := d.lineStarts[p.Line]
	lineEnd := len(d.text)
	if p.Line+1 < len(d.lineStarts) {
		lineEnd = d.lineStarts[p.Line+1]
	}
	line := d.text[lineStart:lineEnd]

	// Walk the line converting UTF-8 runes to UTF-16 code units until the
	// requested column is reached.
	units := 0
	off := 0
	for off < len(line) {
		if units >= p.Character {
			break
		}
		r, size := utf8.DecodeRune(line[off:])
		if r == utf8.RuneError && size <= 1 {
			off++
			units++
			continue
		}
		if r == '\n' {
			break
		}
		units += utf16Len(r)
		off += size
	}
	return lineStart + off
}

// positionOf converts a byte offset into an LSP position.
func (d *document) positionOf(offset int) Position {
	if offset < 0 {
		offset = 0
	}
	if offset > len(d.text) {
		offset = len(d.text)
	}
	// Binary search for the line containing offset.
	lo, hi := 0, len(d.lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if d.lineStarts[mid] <= offset {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	lineStart := d.lineStarts[lo]
	units := 0
	for off := lineStart; off < offset; {
		r, size := utf8.DecodeRune(d.text[off:])
		if r == utf8.RuneError && size <= 1 {
			off++
			units++
			continue
		}
		units += utf16Len(r)
		off += size
	}
	return Position{Line: lo, Character: units}
}

// rangeOf converts a byte span into an LSP range.
func (d *document) rangeOf(startByte, endByte int) Range {
	return Range{Start: d.positionOf(startByte), End: d.positionOf(endByte)}
}

// nodeRange converts a tree-sitter node's byte span into an LSP range.
func (d *document) nodeRange(n *gotreesitter.Node) Range {
	if n == nil {
		return Range{}
	}
	return d.rangeOf(int(n.StartByte()), int(n.EndByte()))
}

// nodeText returns the source text a node covers.
func (d *document) nodeText(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}
	start, end := int(n.StartByte()), int(n.EndByte())
	if start < 0 || end > len(d.text) || start > end {
		return ""
	}
	return string(d.text[start:end])
}

// utf16Len is the number of UTF-16 code units a rune occupies.
func utf16Len(r rune) int {
	if r > 0xFFFF {
		return len(utf16.Encode([]rune{r}))
	}
	return 1
}

// uriToPath converts a file:// URI to a filesystem path. It returns "" for
// any other scheme, which callers treat as "not on disk".
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		// file:///C:/x -> C:/x
		p = strings.TrimPrefix(p, "/")
	}
	if decoded, err := url.PathUnescape(p); err == nil {
		p = decoded
	}
	return filepath.FromSlash(p)
}

// pathToURI converts a filesystem path to a file:// URI.
func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed}
	return u.String()
}
