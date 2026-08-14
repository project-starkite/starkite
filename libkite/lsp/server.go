package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/lsp/apidocs"
)

// Version is reported to the client at initialize.
const Version = "0.1.0"

// Options configures a server.
type Options struct {
	// NewRegistry builds the module registry the catalog introspects. Each
	// edition passes its own, so kitecloud completes k8s and kitecmd does not.
	NewRegistry func(*libkite.ModuleConfig) *libkite.Registry

	// In and Out are the client transport. Both default to stdio.
	In  io.Reader
	Out io.Writer

	// Log receives server-side messages. It must not be the same stream as
	// Out, because anything written there corrupts the protocol.
	Log io.Writer
}

// Server is a Starkite language server.
type Server struct {
	opts Options
	conn *conn
	docs *documentStore
	lang *gotreesitter.Language

	catalogOnce catalogOnce
	docIndex    *docIndex

	mu       sync.Mutex
	shutdown bool
}

// New builds a server. It does not touch the transport until Run is called.
func New(opts Options) (*Server, error) {
	if opts.In == nil {
		opts.In = os.Stdin
	}
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Log == nil {
		opts.Log = os.Stderr
	}
	if opts.NewRegistry == nil {
		return nil, errors.New("lsp: Options.NewRegistry is required")
	}

	entry := grammars.DetectLanguageByName("starlark")
	if entry == nil || entry.Language == nil {
		return nil, errors.New("lsp: the starlark grammar is not compiled into this binary")
	}
	lang := entry.Language()
	if lang == nil {
		return nil, errors.New("lsp: the starlark grammar failed to load")
	}

	return &Server{
		opts:     opts,
		conn:     newConn(opts.In, opts.Out),
		docs:     newDocumentStore(lang),
		lang:     lang,
		docIndex: newDocIndex(apidocs.Table),
	}, nil
}

// catalog returns the completable surface, building it on first use so a
// server that is started and never used pays nothing.
func (s *Server) catalog() *catalog {
	return s.catalogOnce.get(func() *catalog {
		registry := s.opts.NewRegistry(&libkite.ModuleConfig{})
		return buildCatalog(registry, s.docIndex)
	})
}

func (s *Server) logf(format string, args ...any) {
	fmt.Fprintf(s.opts.Log, "kite lsp: "+format+"\n", args...)
}

// Run serves the client until the stream closes or the client exits.
func (s *Server) Run() error {
	for {
		body, err := s.conn.read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			_ = s.conn.replyError(nil, errParse, err.Error())
			continue
		}
		if err := s.dispatch(req); err != nil {
			if errors.Is(err, errExit) {
				return nil
			}
			s.logf("handling %s: %v", req.Method, err)
		}
	}
}

// errExit ends the serve loop on the client's exit notification.
var errExit = errors.New("exit")

// isNotification reports whether a message expects no reply.
func (r request) isNotification() bool { return len(r.ID) == 0 }

func (s *Server) dispatch(req request) error {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "initialized":
		return nil
	case "shutdown":
		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()
		return s.conn.reply(req.ID, nil)
	case "exit":
		return errExit

	case "textDocument/didOpen":
		return s.handleDidOpen(req)
	case "textDocument/didChange":
		return s.handleDidChange(req)
	case "textDocument/didSave":
		return s.handleDidSave(req)
	case "textDocument/didClose":
		return s.handleDidClose(req)

	case "textDocument/completion":
		return s.handlePositional(req, func(d *document, p Position) any {
			return s.complete(d, p)
		})
	case "textDocument/hover":
		return s.handlePositional(req, func(d *document, p Position) any {
			if h := s.hover(d, p); h != nil {
				return h
			}
			return nil
		})
	case "textDocument/signatureHelp":
		return s.handlePositional(req, func(d *document, p Position) any {
			if h := s.signatureHelp(d, p); h != nil {
				return h
			}
			return nil
		})
	case "textDocument/definition":
		return s.handlePositional(req, func(d *document, p Position) any {
			return s.definitionOf(d, d.offsetOf(p))
		})

	case "textDocument/documentSymbol":
		return s.handleDocument(req, func(d *document) any { return s.documentSymbols(d) })
	case "textDocument/foldingRange":
		return s.handleDocument(req, func(d *document) any { return s.foldingRanges(d) })
	case "textDocument/documentLink":
		return s.handleDocument(req, func(d *document) any { return s.documentLinks(d) })
	case "textDocument/semanticTokens/full":
		return s.handleDocument(req, func(d *document) any { return s.semanticTokensFor(d) })

	default:
		if req.isNotification() {
			return nil
		}
		return s.conn.replyError(req.ID, errMethodNotFound, "unsupported method "+req.Method)
	}
}

func (s *Server) handleInitialize(req request) error {
	var params initializeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.conn.replyError(req.ID, errInvalidParams, err.Error())
		}
	}

	// Build the catalog now rather than on the first keystroke, so the first
	// completion is not the slow one.
	go func() {
		cat := s.catalog()
		s.logf("catalog ready: %d global names, %d owners", len(cat.globals), len(cat.members))
	}()

	return s.conn.reply(req.ID, initializeResult{
		ServerInfo: serverInfo{Name: "kite lsp", Version: Version},
		Capabilities: serverCapabilities{
			TextDocumentSync:       2, // incremental
			CompletionProvider:     &completionOptions{TriggerCharacters: []string{".", "\"", "'"}},
			HoverProvider:          true,
			SignatureHelpProvider:  &signatureHelpOptions{TriggerCharacters: []string{"(", ","}, RetriggerCharacters: []string{","}},
			DefinitionProvider:     true,
			DocumentSymbolProvider: true,
			FoldingRangeProvider:   true,
			DocumentLinkProvider:   &struct{}{},
			SemanticTokensProvider: &semanticTokensOptions{
				Legend: semanticTokensLegend{TokenTypes: semanticTokenTypes, TokenModifiers: []string{}},
				Full:   true,
			},
		},
	})
}

func (s *Server) handleDidOpen(req request) error {
	var params didOpenParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}
	d := s.docs.open(params.TextDocument.URI, params.TextDocument.Version, params.TextDocument.Text)
	s.publishDiagnostics(d)
	return nil
}

func (s *Server) handleDidChange(req request) error {
	var params didChangeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}
	d := s.docs.change(params.TextDocument.URI, params.TextDocument.Version, params.ContentChanges)
	if d == nil {
		return nil
	}
	s.publishDiagnostics(d)
	return nil
}

func (s *Server) handleDidSave(req request) error {
	var params didSaveParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}
	if d, ok := s.docs.get(params.TextDocument.URI); ok {
		s.publishDiagnostics(d)
	}
	return nil
}

func (s *Server) handleDidClose(req request) error {
	var params didCloseParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return err
	}
	s.docs.close(params.TextDocument.URI)
	// Clear anything the client is still showing for a file we no longer track.
	return s.conn.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

func (s *Server) handlePositional(req request, fn func(*document, Position) any) error {
	var params textDocumentPositionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.conn.replyError(req.ID, errInvalidParams, err.Error())
	}
	d, ok := s.docs.get(params.TextDocument.URI)
	if !ok {
		return s.conn.reply(req.ID, nil)
	}
	return s.conn.reply(req.ID, fn(d, params.Position))
}

func (s *Server) handleDocument(req request, fn func(*document) any) error {
	var params documentSymbolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.conn.replyError(req.ID, errInvalidParams, err.Error())
	}
	d, ok := s.docs.get(params.TextDocument.URI)
	if !ok {
		return s.conn.reply(req.ID, nil)
	}
	return s.conn.reply(req.ID, fn(d))
}

// publishDiagnostics sends the merged view of both halves.
//
// The authoritative parser runs whenever it can. Only when it cannot parse at
// all does the tree-sitter view stand in, because two parsers reporting the
// same error twice is worse than one reporting it once.
func (s *Server) publishDiagnostics(d *document) {
	diags := s.diagnosticsFor(d)
	if diags == nil {
		diags = []Diagnostic{}
	}
	err := s.conn.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         d.uri,
		Version:     d.version,
		Diagnostics: diags,
	})
	if err != nil {
		s.logf("publishing diagnostics: %v", err)
	}
}

// diagnosticsFor merges the two halves for one document.
func (s *Server) diagnosticsFor(d *document) []Diagnostic {
	semantic := s.analyze(d)
	structural := s.conventionDiagnostics(d)

	if semantic.file == nil {
		// The buffer does not parse. Report the authoritative message, and
		// add the tree's own error spans, which are usually closer to the
		// caret than the parser's first failure.
		out := semantic.diagnostics
		for _, diag := range s.structuralDiagnostics(d) {
			if diag.Source == "kite (syntax)" {
				out = append(out, diag)
			}
		}
		return dedupeDiagnostics(append(out, structural...))
	}
	return dedupeDiagnostics(append(semantic.diagnostics, structural...))
}

// dedupeDiagnostics drops repeats at the same position with the same text.
func dedupeDiagnostics(in []Diagnostic) []Diagnostic {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, d := range in {
		key := fmt.Sprintf("%d:%d:%s", d.Range.Start.Line, d.Range.Start.Character, d.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, d)
	}
	return out
}

// Probe renders what this binary's server would offer, without a client.
//
// It exists so the catalog can be checked directly: a reviewer, or anyone
// debugging a missing completion, can see exactly which modules and members
// the running edition exposes rather than inferring it from an editor.
func (s *Server) Probe() string {
	cat := s.catalog()

	var b strings.Builder
	fmt.Fprintf(&b, "kite lsp %s\n\n", Version)

	fmt.Fprintf(&b, "grammar        starlark (gotreesitter)\n")
	fmt.Fprintf(&b, "documented     %d signatures\n", len(apidocs.Table))
	fmt.Fprintf(&b, "globals        %d\n", len(cat.globals))

	owners := make([]string, 0, len(cat.members))
	total := 0
	for owner, members := range cat.members {
		owners = append(owners, owner)
		total += len(members)
	}
	sort.Strings(owners)
	fmt.Fprintf(&b, "owners         %d (%d members)\n\n", len(owners), total)

	for _, owner := range owners {
		note := ""
		if cat.isModule(owner) {
			note = "module"
		} else {
			note = "object"
		}
		fmt.Fprintf(&b, "  %-22s %-7s %d members\n", owner, note, len(cat.members[owner]))
	}

	if len(cat.factoryReturns) > 0 {
		fmt.Fprintf(&b, "\nfactory return types\n")
		calls := make([]string, 0, len(cat.factoryReturns))
		for call := range cat.factoryReturns {
			calls = append(calls, call)
		}
		sort.Strings(calls)
		for _, call := range calls {
			fmt.Fprintf(&b, "  %-22s -> %s\n", call+"()", cat.factoryReturns[call])
		}
	}
	return b.String()
}

// ---------- small filesystem helpers ----------

func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// splitModuleRev splits "name@rev" into its parts, matching the cache layout.
func splitModuleRev(dirName string) (name, rev string) {
	if i := strings.LastIndex(dirName, "@"); i > 0 {
		return dirName[:i], dirName[i+1:]
	}
	return dirName, ""
}

// IsStarkiteScript reports whether a path looks like a script this server
// should handle.
func IsStarkiteScript(path string) bool {
	return filepath.Ext(path) == ".star"
}
