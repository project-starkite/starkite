// Package lsp implements a Language Server Protocol server for Starkite
// scripts.
//
// The server divides work between two parsers. gotreesitter answers every
// structural question on each keystroke, because it tolerates a broken buffer
// and reparses incrementally. go.starlark.net and libkite answer every
// semantic question, because they are the same code the runtime uses — so a
// diagnostic the editor shows can never disagree with `kite run`.
//
// This file carries the wire protocol: JSON-RPC 2.0 over Content-Length
// framed stdio, plus the subset of LSP structures the server exchanges. The
// types are hand-written rather than pulled from a protocol library so the
// server adds no dependency beyond the parser it needs.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// ---------- JSON-RPC framing ----------

// request is an inbound JSON-RPC message. A message with no ID is a
// notification and takes no reply.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is an outbound reply to a request.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *responseError  `json:"error,omitempty"`
}

// notification is an outbound message that takes no reply.
type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type responseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC and LSP error codes the server returns.
const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
)

// conn reads and writes Content-Length framed JSON-RPC messages.
type conn struct {
	r  *bufio.Reader
	w  io.Writer
	mu sync.Mutex // serializes writes; the read loop may reply out of order
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReaderSize(r, 64*1024), w: w}
}

// read returns the next message body. It returns io.EOF when the client
// closes the stream.
func (c *conn) read() ([]byte, error) {
	length := -1
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length %q: %w", value, err)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("message without Content-Length header")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// write frames and sends one message.
func (c *conn) write(msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

func (c *conn) reply(id json.RawMessage, result any) error {
	return c.write(response{JSONRPC: "2.0", ID: id, Result: result})
}

func (c *conn) replyError(id json.RawMessage, code int, msg string) error {
	return c.write(response{JSONRPC: "2.0", ID: id, Error: &responseError{Code: code, Message: msg}})
}

func (c *conn) notify(method string, params any) error {
	return c.write(notification{JSONRPC: "2.0", Method: method, Params: params})
}

// ---------- LSP structures ----------

// Position is a zero-based line and UTF-16 code-unit offset. The UTF-16
// column is the protocol default and is what editors send.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a half-open span between two positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type didOpenParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   VersionedTextDocumentIdentifier `json:"textDocument"`
	ContentChanges []contentChange                 `json:"contentChanges"`
}

// contentChange is one edit. Range is nil for a full-document replacement.
type contentChange struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

type didSaveParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         *string                `json:"text,omitempty"`
}

type didCloseParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type textDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

type documentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type foldingRangeParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type semanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type documentLinkParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// ---------- diagnostics ----------

type DiagnosticSeverity int

const (
	severityError   DiagnosticSeverity = 1
	severityWarning DiagnosticSeverity = 2
	severityHint    DiagnosticSeverity = 4
)

type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     int          `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// ---------- completion ----------

type CompletionItemKind int

// Only the kinds this server produces.
const (
	kindFunction CompletionItemKind = 3
	kindField    CompletionItemKind = 5
	kindVariable CompletionItemKind = 6
	kindModule   CompletionItemKind = 9
	kindProperty CompletionItemKind = 10
	kindKeyword  CompletionItemKind = 14
	kindConstant CompletionItemKind = 21
)

type CompletionItem struct {
	Label         string             `json:"label"`
	Kind          CompletionItemKind `json:"kind,omitempty"`
	Detail        string             `json:"detail,omitempty"`
	Documentation *MarkupContent     `json:"documentation,omitempty"`
	InsertText    string             `json:"insertText,omitempty"`
	SortText      string             `json:"sortText,omitempty"`
}

type completionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// ---------- hover and signature help ----------

type MarkupContent struct {
	Kind  string `json:"kind"`  // "markdown" or "plaintext"
	Value string `json:"value"` //
}

type hoverResult struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type ParameterInformation struct {
	Label string `json:"label"`
}

type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation *MarkupContent         `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

type signatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

// ---------- symbols, folding, links ----------

type SymbolKind int

const (
	symbolFunction SymbolKind = 12
	symbolVariable SymbolKind = 13
	symbolConstant SymbolKind = 14
)

type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

type FoldingRange struct {
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Kind      string `json:"kind,omitempty"`
}

type DocumentLink struct {
	Range   Range  `json:"range"`
	Target  string `json:"target,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

type semanticTokens struct {
	Data []uint32 `json:"data"`
}

// ---------- initialize ----------

type initializeParams struct {
	RootURI    string `json:"rootUri"`
	RootPath   string `json:"rootPath"`
	ClientInfo *struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"clientInfo,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	Capabilities serverCapabilities `json:"capabilities"`
	ServerInfo   serverInfo         `json:"serverInfo"`
}

type completionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
}

type signatureHelpOptions struct {
	TriggerCharacters   []string `json:"triggerCharacters,omitempty"`
	RetriggerCharacters []string `json:"retriggerCharacters,omitempty"`
}

type semanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

type semanticTokensOptions struct {
	Legend semanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
}

type serverCapabilities struct {
	TextDocumentSync       int                    `json:"textDocumentSync"` // 1 = full, 2 = incremental
	CompletionProvider     *completionOptions     `json:"completionProvider,omitempty"`
	HoverProvider          bool                   `json:"hoverProvider"`
	SignatureHelpProvider  *signatureHelpOptions  `json:"signatureHelpProvider,omitempty"`
	DefinitionProvider     bool                   `json:"definitionProvider"`
	DocumentSymbolProvider bool                   `json:"documentSymbolProvider"`
	FoldingRangeProvider   bool                   `json:"foldingRangeProvider"`
	DocumentLinkProvider   *struct{}              `json:"documentLinkProvider,omitempty"`
	SemanticTokensProvider *semanticTokensOptions `json:"semanticTokensProvider,omitempty"`
}
