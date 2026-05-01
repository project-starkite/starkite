package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/ai/modules/genai"
	"github.com/project-starkite/starkite/libkite"
)

// buildToolFromSource compiles a Starlark script defining functions and returns
// *genai.Tool values for the given names, via genai.CoerceTools.
func buildToolFromSource(t *testing.T, src string, toolNames ...string) []*genai.Tool {
	t.Helper()
	// Load the ai module so ai.tool is available if tests need it.
	genaiMod := genai.New()
	globals, err := genaiMod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("load genai: %v", err)
	}
	thread := &starlark.Thread{Name: "tool-test"}
	exec, err := starlark.ExecFile(thread, "t.star", src, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	items := make([]starlark.Value, 0, len(toolNames))
	for _, name := range toolNames {
		v, ok := exec[name]
		if !ok {
			t.Fatalf("no %q in script globals", name)
		}
		items = append(items, v)
	}
	list := starlark.NewList(items)
	tools, err := genai.CoerceTools(list)
	if err != nil {
		t.Fatalf("CoerceTools: %v", err)
	}
	return tools
}

// newTestRuntime returns a minimal libkite.Runtime for handler tests.
func newTestRuntime(t *testing.T) *libkite.Runtime {
	t.Helper()
	rt, err := libkite.New(&libkite.Config{})
	if err != nil {
		t.Fatalf("New runtime: %v", err)
	}
	return rt
}

// rawArgs JSON-encodes kwargs for a CallToolParamsRaw request.
func rawArgs(t *testing.T, args map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return b
}

func TestBuildToolHandler_HappyPath(t *testing.T) {
	tools := buildToolFromSource(t, `
def greet(name):
    return "hi " + name
`, "greet")
	rt := newTestRuntime(t)
	handler := buildToolHandler(tools[0], rt)

	res, err := handler(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{
			Name:      "greet",
			Arguments: rawArgs(t, map[string]any{"name": "world"}),
		},
	})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if res.IsError {
		t.Errorf("IsError = true, want false")
	}
	if len(res.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(res.Content))
	}
	text, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("Content[0] type = %T, want *TextContent", res.Content[0])
	}
	if text.Text != "hi world" {
		t.Errorf("text = %q, want %q", text.Text, "hi world")
	}
}

func TestBuildToolHandler_StarlarkError_ReturnsMCPError(t *testing.T) {
	tools := buildToolFromSource(t, `
def bad():
    fail("boom")
`, "bad")
	rt := newTestRuntime(t)
	handler := buildToolHandler(tools[0], rt)

	res, err := handler(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{Name: "bad", Arguments: rawArgs(t, map[string]any{})},
	})
	if err != nil {
		t.Fatalf("handler err = %v (expected nil for feedback mode)", err)
	}
	if !res.IsError {
		t.Errorf("IsError = false, want true")
	}
	text, _ := res.Content[0].(*mcpsdk.TextContent)
	if !strings.Contains(text.Text, "boom") {
		t.Errorf("text = %q, want to contain boom", text.Text)
	}
}

func TestBuildToolHandler_DictResult_JSONEncoded(t *testing.T) {
	tools := buildToolFromSource(t, `
def info():
    return {"ok": True, "count": 42}
`, "info")
	rt := newTestRuntime(t)
	handler := buildToolHandler(tools[0], rt)

	res, err := handler(context.Background(), &mcpsdk.CallToolRequest{
		Params: &mcpsdk.CallToolParamsRaw{Name: "info", Arguments: rawArgs(t, map[string]any{})},
	})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	text, _ := res.Content[0].(*mcpsdk.TextContent)
	// JSON key order is deterministic (alphabetical) for encoding/json on maps.
	if !strings.Contains(text.Text, `"ok":true`) || !strings.Contains(text.Text, `"count":42`) {
		t.Errorf("text = %q, expected JSON with ok+count", text.Text)
	}
}

func TestSerializeText_StringPassthrough(t *testing.T) {
	if got := serializeText("hello"); got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestSerializeText_DictEncoded(t *testing.T) {
	got := serializeText(map[string]any{"x": 1})
	if !strings.Contains(got, `"x":1`) {
		t.Errorf("got %q, expected JSON", got)
	}
}

func TestErrorResult_ShapesMCPResult(t *testing.T) {
	res := errorResult(strErr("something broke"))
	if !res.IsError {
		t.Error("IsError = false")
	}
	text, _ := res.Content[0].(*mcpsdk.TextContent)
	if text.Text != "something broke" {
		t.Errorf("text = %q, want 'something broke'", text.Text)
	}
}

type strErr string

func (s strErr) Error() string { return string(s) }
