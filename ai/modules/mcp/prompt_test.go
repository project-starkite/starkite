package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"
)

func buildPromptDict(t *testing.T, items map[string]starlark.Callable) *starlark.Dict {
	t.Helper()
	d := starlark.NewDict(len(items))
	for k, v := range items {
		if err := d.SetKey(starlark.String(k), v); err != nil {
			t.Fatalf("SetKey: %v", err)
		}
	}
	return d
}

func TestCoercePrompts_MustBeDict(t *testing.T) {
	_, err := coercePrompts(starlark.NewList(nil))
	if err == nil || !strings.Contains(err.Error(), "must be a dict") {
		t.Errorf("got %v", err)
	}
}

func TestCoercePrompts_RejectsVarargs(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(*args): return "x"`, "f")
	d := buildPromptDict(t, map[string]starlark.Callable{"p": fns[0]})
	_, err := coercePrompts(d)
	if err == nil || !strings.Contains(err.Error(), "*args") {
		t.Errorf("got %v", err)
	}
}

func TestCoercePrompts_RejectsKwargs(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(**kw): return "x"`, "f")
	d := buildPromptDict(t, map[string]starlark.Callable{"p": fns[0]})
	_, err := coercePrompts(d)
	if err == nil || !strings.Contains(err.Error(), "**kwargs") {
		t.Errorf("got %v", err)
	}
}

func TestBuildPromptEntry_InfersArgumentsFromSignature(t *testing.T) {
	fns := buildCallablesFromSource(t, `
def tmpl(app, count = "1"):
    "Deploy template."
    return "x"
`, "tmpl")
	e, err := buildPromptEntry("deploy", fns[0])
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if e.description != "Deploy template." {
		t.Errorf("description = %q", e.description)
	}
	if len(e.arguments) != 2 {
		t.Fatalf("arguments len = %d, want 2", len(e.arguments))
	}
	if e.arguments[0].Name != "app" || !e.arguments[0].Required {
		t.Errorf("arguments[0] = %+v (want app, required)", e.arguments[0])
	}
	if e.arguments[1].Name != "count" || e.arguments[1].Required {
		t.Errorf("arguments[1] = %+v (want count, optional)", e.arguments[1])
	}
	if len(e.paramNames) != 2 || e.paramNames[0] != "app" || e.paramNames[1] != "count" {
		t.Errorf("paramNames = %v", e.paramNames)
	}
}

func TestBuildPromptHandler_PassesStringArgs(t *testing.T) {
	fns := buildCallablesFromSource(t, `
def tmpl(app, count = "1"):
    return "Deploy " + app + " x" + count
`, "tmpl")
	e, err := buildPromptEntry("deploy", fns[0])
	if err != nil {
		t.Fatalf("buildPromptEntry: %v", err)
	}
	rt := newTestRuntime(t)
	h := buildPromptHandler(e, rt)

	res, err := h(context.Background(), &mcpsdk.GetPromptRequest{
		Params: &mcpsdk.GetPromptParams{
			Name: "deploy",
			Arguments: map[string]string{
				"app":   "nginx",
				"count": "3",
			},
		},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(res.Messages))
	}
	if res.Messages[0].Role != "user" {
		t.Errorf("Role = %q", res.Messages[0].Role)
	}
	text, _ := res.Messages[0].Content.(*mcpsdk.TextContent)
	if text == nil || text.Text != "Deploy nginx x3" {
		t.Errorf("text = %v", text)
	}
}

func TestBuildPromptHandler_NonStringReturn_Errors(t *testing.T) {
	fns := buildCallablesFromSource(t, `
def tmpl():
    return 42
`, "tmpl")
	e, err := buildPromptEntry("n", fns[0])
	if err != nil {
		t.Fatalf("buildPromptEntry: %v", err)
	}
	rt := newTestRuntime(t)
	h := buildPromptHandler(e, rt)

	_, err = h(context.Background(), &mcpsdk.GetPromptRequest{
		Params: &mcpsdk.GetPromptParams{Name: "n"},
	})
	if err == nil || !strings.Contains(err.Error(), "must return a string") {
		t.Errorf("got %v", err)
	}
}

func TestBuildPromptHandler_DropsUndeclaredArgs(t *testing.T) {
	fns := buildCallablesFromSource(t, `
def tmpl(app):
    return app
`, "tmpl")
	e, err := buildPromptEntry("p", fns[0])
	if err != nil {
		t.Fatalf("buildPromptEntry: %v", err)
	}
	rt := newTestRuntime(t)
	h := buildPromptHandler(e, rt)

	// Supplying an extra "bonus" key should be ignored silently.
	res, err := h(context.Background(), &mcpsdk.GetPromptRequest{
		Params: &mcpsdk.GetPromptParams{
			Name:      "p",
			Arguments: map[string]string{"app": "nginx", "bonus": "ignored"},
		},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	text, _ := res.Messages[0].Content.(*mcpsdk.TextContent)
	if text.Text != "nginx" {
		t.Errorf("text = %q", text.Text)
	}
}
