package lsp_test

import (
	"bufio"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/M31-Labs/starlsp"

	"github.com/project-starkite/starkite/libkite/loader"
	"github.com/project-starkite/starkite/libkite/lsp"
)

func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slashed := filepath.ToSlash(abs)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}

func lspHost(t *testing.T) *lsp.Host {
	t.Helper()
	return lsp.NewHost(loader.NewDefaultRegistry)
}

func resolveWith(t *testing.T, h *lsp.Host, expr string) (string, bool) {
	t.Helper()
	var req starlsp.TypeRequest
	req = starlsp.TypeRequest{
		Expr: expr,
		Resolve: func(sub string) (string, bool) {
			return h.ResolveType(starlsp.TypeRequest{
				Expr:    sub,
				Resolve: req.Resolve,
			})
		},
	}
	return h.ResolveType(req)
}

func TestStructInferDirectLiteralAndMerge(t *testing.T) {
	h := lspHost(t)

	owner, ok := resolveWith(t, h, `struct(host = "db.internal", port = 5432, tls = True)`)
	if !ok {
		t.Fatal("expected direct struct literal to resolve")
	}
	got := names(h.Members(owner))
	for _, want := range []string{"host", "port", "tls"} {
		if _, exists := got[want]; !exists {
			t.Fatalf("missing struct member %q in %v", want, got)
		}
	}
	if got["host"].Returns != "string" {
		t.Errorf("host.Returns = %q, want string", got["host"].Returns)
	}
	if sym, docOK := h.Document(owner, "port"); !docOK || sym.Returns != "int" {
		t.Errorf("Document(port) = (%+v, %v), want Returns=int", sym, docOK)
	}

	mergedOwner, ok := resolveWith(t, h, `(struct(region = "us-central1", zone = "a") + struct(zone = "b", project = "edge"))`)
	if !ok {
		t.Fatal("expected struct merge expression to resolve")
	}
	mergedMembers := names(h.Members(mergedOwner))
	for _, want := range []string{"region", "zone", "project"} {
		if _, exists := mergedMembers[want]; !exists {
			t.Fatalf("merged struct missing %q", want)
		}
	}
	if mergedMembers["zone"].Returns != "string" {
		t.Errorf("merged zone Returns = %q, want string", mergedMembers["zone"].Returns)
	}
}

func TestStructInferNestedChainingAndCatalogReturn(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.star")
	src := `def build_env():
    """Return nested environment struct."""
    return struct(
        project = "edge-prod",
        config_file = fs.path("/etc/edge.yaml"),
        db = struct(
            primary = struct(host = "10.0.0.10", port = 5432),
        ),
    )

cfg = build_env()
p = cfg.config_file
`
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	h := lspHost(t)
	hCombined := starlsp.Hosts(starlsp.NewVanilla(), h)

	// Run full JSON-RPC server so starlsp builds a real *starlsp.Document for broken and complete lines.
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	srv, err := starlsp.New(starlsp.Options{
		Host: hCombined,
		In:   serverReader,
		Out:  serverWriter,
		Log:  io.Discard,
	})
	if err != nil {
		t.Fatalf("starlsp.New: %v", err)
	}
	go func() { _ = srv.Run() }()
	t.Cleanup(func() {
		clientWriter.Close()
		clientReader.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	ch := startRPCReader(clientReader)
	mainURI := fileURI(mainPath)
	rpcInitAndOpen(t, clientWriter, ch, mainURI, src)

	// Trigger completion on `cfg.` by editing with incomplete line `cfg.`.
	updated := src + "cfg."
	items := rpcCompleteAfterChange(t, clientWriter, ch, mainURI, 2, updated, len(strings.Split(updated, "\n"))-1, len("cfg."))
	labels := completionLabels(items)
	for _, want := range []string{"project", "config_file", "db"} {
		if !labels[want] {
			t.Errorf("cfg. completion missing %q; got labels=%v", want, labels)
		}
	}

	// Trigger completion on deep chain `cfg.db.primary.` (incomplete expression still parses surrounding defs).
	deepSrc := src + "cfg.db.primary."
	deepItems := rpcCompleteAfterChange(t, clientWriter, ch, mainURI, 3, deepSrc, len(strings.Split(deepSrc, "\n"))-1, len("cfg.db.primary."))
	deepLabels := completionLabels(deepItems)
	if !deepLabels["host"] || !deepLabels["port"] {
		t.Errorf("cfg.db.primary. completion missing host/port; got=%v", deepLabels)
	}

	// Verify catalog factory stored inside a struct field (`cfg.config_file.`) exposes fs.path methods.
	pathSrc := src + "cfg.config_file."
	pathItems := rpcCompleteAfterChange(t, clientWriter, ch, mainURI, 4, pathSrc, len(strings.Split(pathSrc, "\n"))-1, len("cfg.config_file."))
	pathLabels := completionLabels(pathItems)
	if !pathLabels["read_text"] || !pathLabels["exists"] {
		t.Errorf("cfg.config_file. completion missing fs.path members; got=%v", pathLabels)
	}
}

func TestStructInferLoadedModuleFactoriesHoverAndSignature(t *testing.T) {
	dir := t.TempDir()
	gcpDir := filepath.Join(dir, "gcp")
	if err := os.MkdirAll(gcpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	helpersStar := `def make_gke(cluster_name, zone="us-central1-a"):
    """Build GKE cluster descriptor."""
    return struct(name=cluster_name, zone=zone)
`
	if err := os.WriteFile(filepath.Join(dir, "helpers.star"), []byte(helpersStar), 0o644); err != nil {
		t.Fatal(err)
	}

	gcpMain := `load("../helpers.star", "make_gke")

def _config(project, region="us-central1"):
    """Configure GCP environment bindings."""
    base = struct(project=project, region=region)
    return base + struct(
        gke=make_gke("edge-hub"),
        auth_file=fs.path("~/.config/gcloud"),
    )

gcp = struct(
    config=_config,
    provider="google",
)
`
	if err := os.WriteFile(filepath.Join(gcpDir, "main.star"), []byte(gcpMain), 0o644); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(dir, "deploy.star")
	mainSrc := `load("./gcp", g = "gcp")

def main():
    cfg = g.config("my-proj")
    _ = g.provider
    _ = cfg.gke.name
    _ = cfg.project
`
	if err := os.WriteFile(mainPath, []byte(mainSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	kh := lspHost(t)
	h := starlsp.Hosts(starlsp.NewVanilla(), kh)
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	srv, err := starlsp.New(starlsp.Options{
		Host: h,
		In:   serverReader,
		Out:  serverWriter,
		Log:  io.Discard,
	})
	if err != nil {
		t.Fatalf("starlsp.New: %v", err)
	}
	go func() { _ = srv.Run() }()
	t.Cleanup(func() {
		clientWriter.Close()
		clientReader.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	ch := startRPCReader(clientReader)
	uri := fileURI(mainPath)
	rpcInitAndOpen(t, clientWriter, ch, uri, mainSrc)

	// 1. Hover on `g.config` should surface signature & docstring from gcp/main.star `_config`.
	hoverMD := rpcHover(t, clientWriter, ch, uri, 3, strings.Index("    cfg = g.config(\"my-proj\")", "config"))
	if !strings.Contains(hoverMD, "config(project, region=\"us-central1\")") {
		t.Errorf("hover missing signature, got:\n%s", hoverMD)
	}
	if !strings.Contains(hoverMD, "Configure GCP environment bindings.") {
		t.Errorf("hover missing docstring, got:\n%s", hoverMD)
	}

	// 2. Completion inside def main() on broken half-typed `    cfg.gke.` across transitive load (`helpers.star`).
	srcWithGkeDot := mainSrc + "    cfg.gke.\n"
	items := rpcCompleteAfterChange(t, clientWriter, ch, uri, 2, srcWithGkeDot, len(strings.Split(srcWithGkeDot, "\n"))-2, len("    cfg.gke."))
	labels := completionLabels(items)
	if !labels["name"] || !labels["zone"] {
		t.Errorf("cfg.gke. completion missing name/zone; got=%v", labels)
	}

	// 3. SignatureHelp on `g.config(` returns active parameter list.
	srcWithCall := mainSrc + "    g.config()"
	sigLine := len(strings.Split(srcWithCall, "\n")) - 1
	sigText := rpcSignatureHelp(t, clientWriter, ch, uri, 3, srcWithCall, sigLine, len("    g.config("))
	if !strings.Contains(sigText, "config(project, region=\"us-central1\")") {
		t.Errorf("signatureHelp missing config signature, got %q", sigText)
	}
}

func TestStructInferCyclesTerminate(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.star")
	bPath := filepath.Join(dir, "b.star")
	cPath := filepath.Join(dir, "c.star")

	// Circular load between a.star and b.star
	_ = os.WriteFile(aPath, []byte("load(\"./b.star\", \"x\")\ny = x\n"), 0o644)
	_ = os.WriteFile(bPath, []byte("load(\"./a.star\", \"y\")\nx = y\n"), 0o644)
	// Self-assign and mutually recursive defs in c.star
	cSrc := `load("./a.star", "y")
a = a
def f():
    return g()
def g():
    return f()
`
	_ = os.WriteFile(cPath, []byte(cSrc), 0o644)

	kh := lspHost(t)
	h := starlsp.Hosts(starlsp.NewVanilla(), kh)
	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()
	srv, _ := starlsp.New(starlsp.Options{Host: h, In: serverReader, Out: serverWriter, Log: io.Discard})
	go func() { _ = srv.Run() }()
	t.Cleanup(func() {
		clientWriter.Close()
		clientReader.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	ch := startRPCReader(clientReader)
	uri := fileURI(cPath)
	rpcInitAndOpen(t, clientWriter, ch, uri, cSrc)

	for i, expr := range []string{"a.", "y.", "f()."} {
		src := cSrc + expr
		items := rpcCompleteAfterChange(t, clientWriter, ch, uri, i+2, src, len(strings.Split(src, "\n"))-1, len(expr))
		if len(items) != 0 {
			t.Errorf("expected empty completions for cyclic %q, got %+v", expr, items)
		}
	}
}

func TestStructInferConcurrentRace(t *testing.T) {
	h := lspHost(t)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := range 40 {
				expr := `struct(alpha = 1, nested = struct(beta = "ok"))`
				if (id+i)%2 == 0 {
					expr = `(struct(a = 1) + struct(b = fs.path("/tmp")))`
				}
				owner, ok := resolveWith(t, h, expr)
				if !ok {
					t.Errorf("concurrent ResolveType failed for %s", expr)
					return
				}
				members := h.Members(owner)
				if len(members) == 0 {
					t.Errorf("empty members for %s", owner)
					return
				}
				_, _ = h.Document(owner, members[0].Name)
			}
		}(g)
	}
	wg.Wait()
}

var nextRPCID = 100

type rpcResp struct {
	ID     *int            `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  any             `json:"error"`
}

func startRPCReader(r io.Reader) <-chan rpcResp {
	ch := make(chan rpcResp, 64)
	go func() {
		defer close(ch)
		br := bufio.NewReader(r)
		for {
			msgBytes, err := readRPCMessage(br)
			if err != nil {
				return
			}
			var resp rpcResp
			if err := json.Unmarshal(msgBytes, &resp); err == nil && resp.ID != nil {
				ch <- resp
			}
		}
	}()
	return ch
}

func rpcInitAndOpen(t *testing.T, w io.Writer, ch <-chan rpcResp, uri, src string) {
	t.Helper()
	id := nextRPCID
	nextRPCID++
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "initialize",
		"params":  map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	_ = waitRPCResponse(t, ch, id)
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "starlark",
				"version":    1,
				"text":       src,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func rpcCompleteAfterChange(t *testing.T, w io.Writer, ch <-chan rpcResp, uri string, ver int, text string, line, char int) []starlsp.CompletionItem {
	t.Helper()
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": ver},
			"contentChanges": []map[string]any{{"text": text}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	id := nextRPCID
	nextRPCID++
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "textDocument/completion",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": char},
		},
	}); err != nil {
		t.Fatal(err)
	}
	raw := waitRPCResponse(t, ch, id)
	var list struct {
		Items []starlsp.CompletionItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("unmarshal completion: %v (%s)", err, raw)
	}
	return list.Items
}

func rpcHover(t *testing.T, w io.Writer, ch <-chan rpcResp, uri string, line, char int) string {
	t.Helper()
	id := nextRPCID
	nextRPCID++
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": char},
		},
	}); err != nil {
		t.Fatal(err)
	}
	raw := waitRPCResponse(t, ch, id)
	var res struct {
		Contents starlsp.MarkupContent `json:"contents"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal hover: %v (%s)", err, raw)
	}
	return res.Contents.Value
}

func rpcSignatureHelp(t *testing.T, w io.Writer, ch <-chan rpcResp, uri string, ver int, text string, line, char int) string {
	t.Helper()
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didChange",
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": ver},
			"contentChanges": []map[string]any{{"text": text}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	id := nextRPCID
	nextRPCID++
	if err := sendRPCMessage(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "textDocument/signatureHelp",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": line, "character": char},
		},
	}); err != nil {
		t.Fatal(err)
	}
	raw := waitRPCResponse(t, ch, id)
	var res struct {
		Signatures []struct {
			Label string `json:"label"`
		} `json:"signatures"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal signatureHelp: %v (%s)", err, raw)
	}
	if len(res.Signatures) == 0 {
		return ""
	}
	return res.Signatures[0].Label
}

func waitRPCResponse(t *testing.T, ch <-chan rpcResp, id int) json.RawMessage {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for RPC response %d", id)
			return nil
		case resp, ok := <-ch:
			if !ok {
				t.Fatalf("RPC stream closed while waiting for response %d", id)
				return nil
			}
			if resp.ID != nil && *resp.ID == id {
				if resp.Error != nil {
					t.Fatalf("RPC %d error: %v", id, resp.Error)
				}
				return resp.Result
			}
		}
	}
}

func completionLabels(items []starlsp.CompletionItem) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, it := range items {
		out[it.Label] = true
	}
	return out
}
