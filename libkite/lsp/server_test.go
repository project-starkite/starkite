package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/loader"
)

// session drives a real JSON-RPC conversation against the server, over pipes,
// exactly as an editor would. Testing through the wire rather than calling
// handlers directly is what proves the protocol layer works too.
type session struct {
	t       *testing.T
	toSrv   *io.PipeWriter
	fromSrv *bufferedReader
	nextID  int
	done    chan error
}

func newSession(t *testing.T) *session {
	t.Helper()

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	srv, err := New(Options{
		NewRegistry: loader.NewDefaultRegistry,
		In:          serverReader,
		Out:         serverWriter,
		Log:         io.Discard,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	s := &session{
		t:       t,
		toSrv:   clientWriter,
		fromSrv: newBufferedReader(clientReader),
		nextID:  1,
		done:    make(chan error, 1),
	}
	go func() { s.done <- srv.Run() }()

	t.Cleanup(func() {
		s.notify("exit", nil)
		clientWriter.Close()
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Log("server did not stop within 5s")
		}
	})

	s.request("initialize", initializeParams{})
	s.notify("initialized", struct{}{})
	return s
}

// request sends a request and returns its result payload.
func (s *session) request(method string, params any) json.RawMessage {
	s.t.Helper()
	id := s.nextID
	s.nextID++

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		s.t.Fatalf("marshal %s: %v", method, err)
	}
	s.send(body)

	// Skip server-initiated notifications until the matching reply arrives.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		msg := s.receive()
		var envelope struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *responseError  `json:"error"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			s.t.Fatalf("unmarshal reply to %s: %v", method, err)
		}
		if envelope.ID == nil || *envelope.ID != id {
			continue
		}
		if envelope.Error != nil {
			s.t.Fatalf("%s returned error: %s", method, envelope.Error.Message)
		}
		return envelope.Result
	}
	s.t.Fatalf("timed out waiting for a reply to %s", method)
	return nil
}

func (s *session) notify(method string, params any) {
	s.t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		s.t.Fatalf("marshal %s: %v", method, err)
	}
	s.send(body)
}

func (s *session) send(body []byte) {
	s.t.Helper()
	if _, err := fmt.Fprintf(s.toSrv, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		s.t.Fatalf("write: %v", err)
	}
}

func (s *session) receive() []byte {
	s.t.Helper()
	body, err := s.fromSrv.readMessage()
	if err != nil {
		s.t.Fatalf("read: %v", err)
	}
	return body
}

// waitDiagnostics reads until a publishDiagnostics notification for uri.
func (s *session) waitDiagnostics(uri string) []Diagnostic {
	s.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		msg := s.receive()
		var envelope struct {
			Method string                   `json:"method"`
			Params publishDiagnosticsParams `json:"params"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}
		if envelope.Method == "textDocument/publishDiagnostics" && envelope.Params.URI == uri {
			return envelope.Params.Diagnostics
		}
	}
	s.t.Fatalf("timed out waiting for diagnostics on %s", uri)
	return nil
}

// open sends didOpen and returns the diagnostics the server publishes for it.
func (s *session) open(uri, text string) []Diagnostic {
	s.t.Helper()
	s.notify("textDocument/didOpen", didOpenParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "starlark", Version: 1, Text: text},
	})
	return s.waitDiagnostics(uri)
}

// bufferedReader reads Content-Length framed messages from a stream.
type bufferedReader struct{ c *conn }

func newBufferedReader(r io.Reader) *bufferedReader {
	return &bufferedReader{c: newConn(r, io.Discard)}
}

func (b *bufferedReader) readMessage() ([]byte, error) { return b.c.read() }

// ---------- position helpers ----------

// at returns the position of the caret marked by "|" in src, and src without
// the marker. Writing tests against a visible caret beats counting columns.
func at(t *testing.T, src string) (string, Position) {
	t.Helper()
	idx := strings.Index(src, "|")
	if idx < 0 {
		t.Fatalf("test source has no caret marker")
	}
	clean := src[:idx] + src[idx+1:]
	line := strings.Count(src[:idx], "\n")
	lineStart := strings.LastIndex(src[:idx], "\n") + 1
	return clean, Position{Line: line, Character: idx - lineStart}
}

func labels(items []CompletionItem) map[string]CompletionItem {
	out := make(map[string]CompletionItem, len(items))
	for _, item := range items {
		out[item.Label] = item
	}
	return out
}

// ---------- tests ----------

func TestInitializeAdvertisesEveryCapability(t *testing.T) {
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	srv, err := New(Options{
		NewRegistry: loader.NewDefaultRegistry,
		In:          serverReader, Out: serverWriter, Log: io.Discard,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go srv.Run()
	defer clientWriter.Close()

	s := &session{t: t, toSrv: clientWriter, fromSrv: newBufferedReader(clientReader), nextID: 1, done: make(chan error, 1)}
	raw := s.request("initialize", initializeParams{})

	var result initializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	caps := result.Capabilities

	if caps.TextDocumentSync != 2 {
		t.Errorf("TextDocumentSync = %d, want 2 (incremental)", caps.TextDocumentSync)
	}
	for name, present := range map[string]bool{
		"completion":     caps.CompletionProvider != nil,
		"hover":          caps.HoverProvider,
		"signatureHelp":  caps.SignatureHelpProvider != nil,
		"definition":     caps.DefinitionProvider,
		"documentSymbol": caps.DocumentSymbolProvider,
		"foldingRange":   caps.FoldingRangeProvider,
		"documentLink":   caps.DocumentLinkProvider != nil,
		"semanticTokens": caps.SemanticTokensProvider != nil,
	} {
		if !present {
			t.Errorf("capability %q not advertised", name)
		}
	}
}

func TestCompletionAfterModuleDotUsesTheRegistry(t *testing.T) {
	s := newSession(t)
	src, pos := at(t, "def main():\n    fs.|\n")
	uri := "file:///tmp/kite-lsp-test/complete_module.star"
	s.open(uri, src)

	raw := s.request("textDocument/completion", textDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
	})
	var list completionList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal completion: %v", err)
	}
	got := labels(list.Items)

	// fs exposes exactly the Path factory plus its try_ variant.
	for _, want := range []string{"path", "try_path"} {
		if _, ok := got[want]; !ok {
			t.Errorf("completion after `fs.` missing %q; got %v", want, keysOf(got))
		}
	}
	// A member of a different module must not leak in.
	if _, leaked := got["printf"]; leaked {
		t.Errorf("completion after `fs.` offered the global `printf`")
	}
}

func TestCompletionAfterObjectDotFollowsTheFactoryReturn(t *testing.T) {
	s := newSession(t)
	// p is bound from fs.path(...), so the caret after `p.` must offer Path
	// methods — a name set the module namespace alone does not contain.
	src, pos := at(t, "def main():\n    p = fs.path(\"/etc/hosts\")\n    p.|\n")
	uri := "file:///tmp/kite-lsp-test/complete_object.star"
	s.open(uri, src)

	raw := s.request("textDocument/completion", textDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
	})
	var list completionList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal completion: %v", err)
	}
	got := labels(list.Items)

	for _, want := range []string{"read_text", "write_text", "exists", "try_read_text", "parent"} {
		if _, ok := got[want]; !ok {
			t.Errorf("completion after `p.` missing Path method %q", want)
		}
	}
	if item, ok := got["read_text"]; ok && item.Detail == "" {
		t.Errorf("read_text has no detail; the documentation join produced nothing")
	}
}

func TestCompletionSuppressedInsideCommentsAndStrings(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/suppressed.star"

	for name, source := range map[string]string{
		"comment": "def main():\n    # fs.|\n",
		"string":  "def main():\n    x = \"fs.|\n",
	} {
		src, pos := at(t, source)
		s.open(uri, src)
		raw := s.request("textDocument/completion", textDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     pos,
		})
		var list completionList
		if err := json.Unmarshal(raw, &list); err != nil {
			t.Fatalf("%s: unmarshal completion: %v", name, err)
		}
		if len(list.Items) != 0 {
			t.Errorf("%s: completion offered %d items inside a %s", name, len(list.Items), name)
		}
	}
}

func TestHoverJoinsRuntimeNameToDocumentedSignature(t *testing.T) {
	s := newSession(t)
	src, pos := at(t, "def main():\n    p = fs.pa|th(\"/etc/hosts\")\n")
	uri := "file:///tmp/kite-lsp-test/hover.star"
	s.open(uri, src)

	raw := s.request("textDocument/hover", textDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     pos,
	})
	if string(raw) == "null" {
		t.Fatal("hover returned null for fs.path")
	}
	var got hoverResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal hover: %v", err)
	}
	if !strings.Contains(got.Contents.Value, "path") {
		t.Errorf("hover text does not mention the member: %q", got.Contents.Value)
	}
	if got.Range == nil {
		t.Error("hover has no range, so the editor cannot underline the symbol")
	}
}

func TestDocumentSymbolsOutlineFunctionsAndBindings(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/symbols.star"
	s.open(uri, "NAME = \"world\"\n\ndef greet(who):\n    printf(\"%s\", who)\n\ndef main():\n    greet(NAME)\n")

	raw := s.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	var symbols []DocumentSymbol
	if err := json.Unmarshal(raw, &symbols); err != nil {
		t.Fatalf("unmarshal symbols: %v", err)
	}

	found := make(map[string]SymbolKind, len(symbols))
	for _, sym := range symbols {
		found[sym.Name] = sym.Kind
	}
	if found["greet"] != symbolFunction || found["main"] != symbolFunction {
		t.Errorf("functions missing from the outline: %v", found)
	}
	if found["NAME"] != symbolConstant {
		t.Errorf("NAME = %v, want constant (all-caps binding)", found["NAME"])
	}
}

func TestDiagnosticsComeFromTheRuntimeParser(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/broken.star"
	diags := s.open(uri, "def main(:\n    pass\n")

	if len(diags) == 0 {
		t.Fatal("a syntax error produced no diagnostics")
	}
	var fromRuntime bool
	for _, d := range diags {
		if d.Source == "kite" {
			fromRuntime = true
		}
		if d.Range.Start.Line != 0 {
			continue
		}
	}
	if !fromRuntime {
		t.Errorf("no diagnostic attributed to the runtime parser; got %+v", diags)
	}
}

func TestCleanScriptHasNoDiagnostics(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/clean.star"
	diags := s.open(uri, "def main():\n    printf(\"%s\\n\", runtime.platform())\n")

	for _, d := range diags {
		if d.Severity == severityError {
			t.Errorf("clean script produced an error diagnostic: %s at %v", d.Message, d.Range.Start)
		}
	}
}

func TestUndefinedNameIsReportedButRegistryNamesAreNot(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/undefined.star"
	diags := s.open(uri, "def main():\n    printf(\"%s\", runtime.platform())\n    no_such_builtin()\n")

	var sawUndefined bool
	for _, d := range diags {
		if strings.Contains(d.Message, "no_such_builtin") {
			sawUndefined = true
		}
		if strings.Contains(d.Message, "printf") || strings.Contains(d.Message, "runtime") {
			t.Errorf("a registry-provided name was reported as undefined: %s", d.Message)
		}
	}
	if !sawUndefined {
		t.Errorf("an undefined name produced no diagnostic; got %+v", diags)
	}
}

func TestControlFlowOutsideFunctionIsReported(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/toplevel_if.star"
	diags := s.open(uri, "x = 1\nif x:\n    print(\"no\")\n")

	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, "only inside a function body") {
			found = true
		}
	}
	if !found {
		t.Errorf("a top-level if produced no convention diagnostic; got %+v", diags)
	}
}

func TestTopLevelMainCallIsHinted(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/mainhint.star"
	diags := s.open(uri, "def main():\n    print(\"hi\")\n\nmain()\n")

	var hinted bool
	for _, d := range diags {
		if d.Severity == severityHint && strings.Contains(d.Message, "automatic main() invocation") {
			hinted = true
		}
	}
	if !hinted {
		t.Errorf("a top-level main() call produced no hint; got %+v", diags)
	}
}

func TestStructureSurvivesABrokenBuffer(t *testing.T) {
	// The point of the tree-sitter half: an outline is still available when
	// the authoritative parser cannot produce anything at all.
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/halftyped.star"
	s.open(uri, "def ready():\n    pass\n\ndef broken(:\n")

	raw := s.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	var symbols []DocumentSymbol
	if err := json.Unmarshal(raw, &symbols); err != nil {
		t.Fatalf("unmarshal symbols: %v", err)
	}
	var sawReady bool
	for _, sym := range symbols {
		if sym.Name == "ready" {
			sawReady = true
		}
	}
	if !sawReady {
		t.Errorf("the outline lost a valid function because a later one is broken: %+v", symbols)
	}
}

func TestSemanticTokensEncodeAsQuintuples(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/tokens.star"
	s.open(uri, "# a comment\ndef main():\n    fs.path(\"/tmp/x\")\n")

	raw := s.request("textDocument/semanticTokens/full", semanticTokensParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	var tokens semanticTokens
	if err := json.Unmarshal(raw, &tokens); err != nil {
		t.Fatalf("unmarshal tokens: %v", err)
	}
	if len(tokens.Data) == 0 {
		t.Fatal("no semantic tokens produced")
	}
	if len(tokens.Data)%5 != 0 {
		t.Fatalf("token data length %d is not a multiple of 5", len(tokens.Data))
	}
	// The first token is the comment on line 0.
	if tokens.Data[0] != 0 || tokens.Data[3] != uint32(stComment) {
		t.Errorf("first token = line delta %d kind %d, want line 0 kind comment(%d)",
			tokens.Data[0], tokens.Data[3], stComment)
	}
}

func TestIncrementalEditKeepsPositionsCorrect(t *testing.T) {
	s := newSession(t)
	uri := "file:///tmp/kite-lsp-test/incremental.star"
	s.open(uri, "def main():\n    pass\n")

	// Replace "pass" with a call, exactly as an editor sends it.
	s.notify("textDocument/didChange", didChangeParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []contentChange{{
			Range: &Range{Start: Position{Line: 1, Character: 4}, End: Position{Line: 1, Character: 8}},
			Text:  "fs.",
		}},
	})
	s.waitDiagnostics(uri)

	raw := s.request("textDocument/completion", textDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 1, Character: 7},
	})
	var list completionList
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal completion: %v", err)
	}
	if _, ok := labels(list.Items)["path"]; !ok {
		t.Errorf("after an incremental edit, `fs.` did not complete; got %v", keysOf(labels(list.Items)))
	}
}

func TestUTF16PositionsHandleNonASCII(t *testing.T) {
	store := newDocumentStore(mustStarlark(t))
	// "é" is one UTF-16 unit; the emoji is a surrogate pair, so it is two.
	d := store.open("file:///tmp/kite-lsp-test/utf16.star", 1, "x = \"é🚀\"\ny = 1\n")

	end := d.positionOf(len(d.text))
	if end.Line != 2 {
		t.Errorf("end position line = %d, want 2", end.Line)
	}
	// Round-tripping every offset must be stable.
	for offset := 0; offset <= len(d.text); offset++ {
		pos := d.positionOf(offset)
		if back := d.offsetOf(pos); back != offset {
			// Offsets inside a multi-byte rune have no position of their own,
			// so only whole-rune boundaries must round-trip.
			if isRuneStart(d.text, offset) {
				t.Errorf("offset %d round-tripped to %d via %+v", offset, back, pos)
			}
		}
	}
}

func isRuneStart(b []byte, offset int) bool {
	if offset >= len(b) {
		return true
	}
	return b[offset]&0xC0 != 0x80
}

func mustStarlark(t *testing.T) *gotreesitter.Language {
	t.Helper()
	srv, err := New(Options{NewRegistry: func(*libkite.ModuleConfig) *libkite.Registry {
		return loader.NewDefaultRegistry(&libkite.ModuleConfig{})
	}, In: strings.NewReader(""), Out: io.Discard, Log: io.Discard})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv.lang
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
