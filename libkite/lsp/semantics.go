package lsp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/project-starkite/starkite/libkite"
)

// This file is the semantic half of the server. Everything here runs the same
// code the runtime runs, so a diagnostic it produces is a diagnostic `kite run`
// would produce. It is not error tolerant, and it is not meant to be: when the
// buffer does not parse, the structural half covers the gap.

// analysis is the result of one authoritative pass over a document.
type analysis struct {
	file        *syntax.File // nil when the document does not parse
	diagnostics []Diagnostic
}

// analyze parses and resolves a document.
//
// It uses syntax.Parse with the same options `kite validate` uses, so the
// messages an editor shows are the messages the command line shows.
func (s *Server) analyze(d *document) analysis {
	var out analysis

	file, err := syntax.Parse(d.filename(), d.text, 0)
	if err != nil {
		out.diagnostics = s.syntaxDiagnostics(d, err)
		return out
	}
	out.file = file

	// Resolution needs to know which names the runtime provides. Asking the
	// catalog rather than a fixed list means a module added to the registry
	// stops being reported as undefined with no change here.
	cat := s.catalog()
	isPredeclared := func(name string) bool {
		_, ok := cat.lookup("", name)
		return ok
	}
	isUniversal := func(name string) bool {
		return starlark_universe[name]
	}

	if err := resolve.File(file, isPredeclared, isUniversal); err != nil {
		out.diagnostics = append(out.diagnostics, s.resolveDiagnostics(d, err)...)
	}
	return out
}

// filename returns a name for error messages.
func (d *document) filename() string {
	if d.path != "" {
		return d.path
	}
	return d.uri
}

// syntaxDiagnostics converts a parse error into diagnostics.
func (s *Server) syntaxDiagnostics(d *document, err error) []Diagnostic {
	var list []Diagnostic

	var syntaxErr syntax.Error
	if errors.As(err, &syntaxErr) {
		return []Diagnostic{s.diagnosticAt(d, syntaxErr.Pos, syntaxErr.Msg, "kite")}
	}
	// Some errors arrive as a plain message with the position rendered in the
	// text. Report them on the first line rather than dropping them.
	list = append(list, Diagnostic{
		Range:    Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		Severity: severityError,
		Source:   "kite",
		Message:  err.Error(),
	})
	return list
}

// resolveDiagnostics converts resolver errors into diagnostics. These are the
// undefined-name and misplaced-statement errors that Starlark enforces.
func (s *Server) resolveDiagnostics(d *document, err error) []Diagnostic {
	var list resolve.ErrorList
	if !errors.As(err, &list) {
		return []Diagnostic{{
			Range:    Range{},
			Severity: severityError,
			Source:   "kite",
			Message:  err.Error(),
		}}
	}
	out := make([]Diagnostic, 0, len(list))
	for _, e := range list {
		out = append(out, s.diagnosticAt(d, e.Pos, e.Msg, "kite"))
	}
	return out
}

// diagnosticAt converts a Starlark position — 1-based line, 1-based rune
// column — into an LSP range covering the token that starts there.
func (s *Server) diagnosticAt(d *document, pos syntax.Position, msg, source string) Diagnostic {
	offset := d.offsetOfStarlarkPos(pos)
	end := offset
	// Extend across the identifier under the position so the squiggle has
	// width rather than sitting between two characters.
	for end < len(d.text) && isIdentByte(d.text[end]) {
		end++
	}
	if end == offset && end < len(d.text) {
		end++
	}
	return Diagnostic{
		Range:    d.rangeOf(offset, end),
		Severity: severityError,
		Source:   source,
		Message:  msg,
	}
}

// offsetOfStarlarkPos converts a 1-based line and rune column to a byte offset.
func (d *document) offsetOfStarlarkPos(pos syntax.Position) int {
	line := int(pos.Line) - 1
	if line < 0 {
		line = 0
	}
	if line >= len(d.lineStarts) {
		return len(d.text)
	}
	offset := d.lineStarts[line]
	col := int(pos.Col) - 1
	for i := 0; i < col && offset < len(d.text); i++ {
		if d.text[offset] == '\n' {
			break
		}
		_, size := decodeRune(d.text[offset:])
		offset += size
	}
	return offset
}

func decodeRune(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 0
	}
	if b[0] < 0x80 {
		return rune(b[0]), 1
	}
	for size := 2; size <= 4 && size <= len(b); size++ {
		if size == len(b) || b[size]&0xC0 != 0x80 {
			return rune(b[0]), size
		}
	}
	return rune(b[0]), 1
}

// starlark_universe is the set of names the Starlark specification defines for
// every dialect, independent of what a host application predeclares.
var starlark_universe = map[string]bool{
	"None": true, "True": true, "False": true,
	"abs": true, "any": true, "all": true, "bool": true, "bytes": true,
	"chr": true, "dict": true, "dir": true, "enumerate": true, "fail": true,
	"float": true, "getattr": true, "hasattr": true, "hash": true, "int": true,
	"len": true, "list": true, "max": true, "min": true, "ord": true,
	"print": true, "range": true, "repr": true, "reversed": true, "set": true,
	"sorted": true, "str": true, "tuple": true, "type": true, "zip": true,
}

// ---------- definition ----------

// definitionOf resolves the identifier at an offset to where it is bound.
//
// In-file bindings come from the resolver, which already knows the difference
// between a local, a global, and a free variable — reimplementing that on the
// tree would be a second, disagreeing answer to a question Starlark already
// answers.
func (s *Server) definitionOf(d *document, offset int) []Location {
	a := s.analyze(d)
	if a.file == nil {
		return nil
	}

	var target *syntax.Ident
	syntax.Walk(a.file, func(n syntax.Node) bool {
		ident, ok := n.(*syntax.Ident)
		if !ok {
			return true
		}
		start, end := ident.Span()
		if offset >= d.offsetOfStarlarkPos(start) && offset <= d.offsetOfStarlarkPos(end) {
			target = ident
		}
		return true
	})
	if target == nil {
		return nil
	}

	binding, ok := target.Binding.(*resolve.Binding)
	if !ok || binding == nil || binding.First == nil {
		return nil
	}
	start, end := binding.First.Span()
	return []Location{{
		URI:   d.uri,
		Range: d.rangeOf(d.offsetOfStarlarkPos(start), d.offsetOfStarlarkPos(end)),
	}}
}

// ---------- load() resolution ----------

// resolveLoadTarget maps a load() reference to a path on disk.
//
// The reference grammar is the runtime's, documented on the loader: a path
// reference resolves relative to the caller, and a bare "namespace/name" is an
// installed identity resolved from the version-addressed module cache, pinned
// by mod.lock when one governs the caller.
func (s *Server) resolveLoadTarget(d *document, target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || d.path == "" {
		return "", false
	}
	callerDir := filepath.Dir(d.path)

	if isPathReference(target) {
		candidate := target
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(callerDir, candidate)
		}
		return resolveModuleDir(candidate)
	}

	// A bare .star name without a path prefix is an error in the runtime, so
	// it is not a link here either.
	if strings.HasSuffix(target, ".star") {
		return "", false
	}
	if !strings.Contains(target, "/") {
		return "", false // a built-in module has no file
	}
	return s.resolveInstalledModule(callerDir, target)
}

func isPathReference(target string) bool {
	return strings.HasPrefix(target, "./") ||
		strings.HasPrefix(target, "../") ||
		filepath.IsAbs(target)
}

// resolveModuleDir accepts a directory holding main.star, or a .star file.
func resolveModuleDir(candidate string) (string, bool) {
	if info, err := os.Stat(candidate); err == nil {
		if info.IsDir() {
			entry := filepath.Join(candidate, libkite.EntryFile)
			if _, err := os.Stat(entry); err == nil {
				return entry, true
			}
			return "", false
		}
		return candidate, true
	}
	// A directory reference written without its .star suffix.
	if _, err := os.Stat(candidate + ".star"); err == nil {
		return candidate + ".star", true
	}
	return "", false
}

// resolveInstalledModule finds "namespace/name" in the module cache, honouring
// the revision mod.lock pins when the caller is governed by one.
func (s *Server) resolveInstalledModule(callerDir, identity string) (string, bool) {
	root := moduleCacheRoot()
	if root == "" {
		return "", false
	}
	name := identity
	if idx := strings.LastIndex(identity, "/"); idx >= 0 {
		name = identity[idx+1:]
	}

	if rev, ok := lockedRevision(callerDir, identity); ok {
		pinned := filepath.Join(root, fmt.Sprintf("%s@%s", name, rev))
		if entry, ok := resolveModuleDir(pinned); ok {
			return entry, true
		}
	}

	// No lock, or the pinned revision is not present: accept any revision of
	// the module that is installed.
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirName, _ := libkite.SplitModuleRev(e.Name())
		if dirName != name {
			continue
		}
		if entry, ok := resolveModuleDir(filepath.Join(root, e.Name())); ok {
			return entry, true
		}
	}
	return "", false
}

// lockedRevision reads the revision mod.lock pins for an identity, searching
// upwards from the caller for the lockfile that governs it.
func lockedRevision(startDir, identity string) (string, bool) {
	dir := startDir
	for i := 0; i < 32; i++ {
		lock, err := libkite.LoadLock(dir)
		if err == nil && lock != nil {
			if m, ok := lock.Modules[identity]; ok && m.Rev != "" {
				return m.Rev, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// moduleCacheRoot is where installed modules live.
func moduleCacheRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".starkite", "modules")
}
