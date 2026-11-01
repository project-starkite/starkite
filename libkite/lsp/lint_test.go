//go:build lsp

package lsp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/M31-Labs/starlsp"

	"github.com/project-starkite/starkite/libkite/loader"
	kitelsp "github.com/project-starkite/starkite/libkite/lsp"
)

type rpcMessage struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     *int            `json:"id"`
}

type publishDiagnosticsParams struct {
	URI         string               `json:"uri"`
	Diagnostics []starlsp.Diagnostic `json:"diagnostics"`
}

func readRPCMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			l, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, err
			}
			length = l
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("no Content-Length header found")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func sendRPCMessage(w io.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}

func collectKiteDiagnostics(t *testing.T, src string) []starlsp.Diagnostic {
	t.Helper()

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	h := starlsp.Hosts(starlsp.NewVanilla(), kitelsp.NewHost(loader.NewDefaultRegistry))
	srv, err := starlsp.New(starlsp.Options{
		Host: h,
		In:   serverReader,
		Out:  serverWriter,
		Log:  io.Discard,
	})
	if err != nil {
		t.Fatalf("starlsp.New: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.Run()
	}()

	t.Cleanup(func() {
		clientWriter.Close()
		clientReader.Close()
		serverWriter.Close()
		serverReader.Close()
	})

	reader := bufio.NewReader(clientReader)

	// 1. Initialize
	if err := sendRPCMessage(clientWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	}); err != nil {
		t.Fatalf("send initialize: %v", err)
	}

	// Wait for initialize response
	for {
		msgBytes, err := readRPCMessage(reader)
		if err != nil {
			t.Fatalf("read initialize response: %v", err)
		}
		var msg rpcMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if msg.ID != nil && *msg.ID == 1 {
			break
		}
	}

	// 2. Initialized notification
	if err := sendRPCMessage(clientWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
	}); err != nil {
		t.Fatalf("send initialized: %v", err)
	}

	// 3. DidOpen
	uri := "file:///test/script.star"
	if err := sendRPCMessage(clientWriter, map[string]any{
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
		t.Fatalf("send didOpen: %v", err)
	}

	// 4. Wait for publishDiagnostics
	timeout := time.After(5 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for publishDiagnostics")
		default:
			msgBytes, err := readRPCMessage(reader)
			if err != nil {
				t.Fatalf("read diagnostics: %v", err)
			}
			var msg rpcMessage
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				continue
			}
			if msg.Method == "textDocument/publishDiagnostics" {
				var params publishDiagnosticsParams
				if err := json.Unmarshal(msg.Params, &params); err != nil {
					t.Fatalf("unmarshal diagnostics params: %v", err)
				}
				if params.URI == uri {
					var kiteDiags []starlsp.Diagnostic
					for _, d := range params.Diagnostics {
						if d.Source == "kite" {
							kiteDiags = append(kiteDiags, d)
						}
					}
					return kiteDiags
				}
			}
		}
	}
}

func TestLinterTopLevelMain(t *testing.T) {
	src := `def main():
    pass

main()
`
	diags := collectKiteDiagnostics(t, src)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "skips its automatic main() invocation") {
			found = true
			if d.Severity != starlsp.SeverityHint {
				t.Errorf("expected severity Hint (4), got %v", d.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected top-level main() diagnostic, got %v", diags)
	}
}

func TestLinterUnassignedTryCall(t *testing.T) {
	src := `os.try_exec(["ls"])
`
	diags := collectKiteDiagnostics(t, src)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "result of try_ call is discarded") {
			found = true
			if d.Severity != starlsp.SeverityWarning {
				t.Errorf("expected severity Warning (2), got %v", d.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected discarded try_ call diagnostic, got %v", diags)
	}
}

func TestLinterAssignedUncheckedTryCall(t *testing.T) {
	src := `res = os.try_exec(["ls"])
print(res)
`
	diags := collectKiteDiagnostics(t, src)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "read res.ok before res.value") {
			found = true
			if d.Severity != starlsp.SeverityWarning {
				t.Errorf("expected severity Warning (2), got %v", d.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected unchecked try_ call diagnostic, got %v", diags)
	}
}

func TestLinterCheckedTryCallPasses(t *testing.T) {
	src := `res = os.try_exec(["ls"])
if res.ok:
    print(res.value)
`
	diags := collectKiteDiagnostics(t, src)
	for _, d := range diags {
		if strings.Contains(d.Message, "read res.ok before res.value") ||
			strings.Contains(d.Message, "result of try_ call is discarded") {
			t.Errorf("unexpected try_ diagnostic on properly checked result: %v", d)
		}
	}
}
