package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"
)

// buildCallablesFromSource executes a Starlark script and returns the named
// globals as starlark.Callable values. Used by both resource and prompt tests.
func buildCallablesFromSource(t *testing.T, src string, names ...string) []starlark.Callable {
	t.Helper()
	thread := &starlark.Thread{Name: "test"}
	got, err := starlark.ExecFile(thread, "t.star", src, nil)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	out := make([]starlark.Callable, 0, len(names))
	for _, n := range names {
		v, ok := got[n]
		if !ok {
			t.Fatalf("no %q in globals", n)
		}
		c, ok := v.(starlark.Callable)
		if !ok {
			t.Fatalf("%q is %T, not Callable", n, v)
		}
		out = append(out, c)
	}
	return out
}

func buildResourceDict(t *testing.T, items map[string]starlark.Callable) *starlark.Dict {
	t.Helper()
	d := starlark.NewDict(len(items))
	for k, v := range items {
		if err := d.SetKey(starlark.String(k), v); err != nil {
			t.Fatalf("SetKey: %v", err)
		}
	}
	return d
}

func TestCoerceResources_MustBeDict(t *testing.T) {
	_, err := coerceResources(starlark.NewList(nil))
	if err == nil || !strings.Contains(err.Error(), "must be a dict") {
		t.Errorf("got %v", err)
	}
}

func TestCoerceResources_KeyMustBeString(t *testing.T) {
	d := starlark.NewDict(1)
	fns := buildCallablesFromSource(t, `def f(): return "x"`, "f")
	_ = d.SetKey(starlark.MakeInt(1), fns[0])
	_, err := coerceResources(d)
	if err == nil || !strings.Contains(err.Error(), "must be strings") {
		t.Errorf("got %v", err)
	}
}

func TestCoerceResources_ValueMustBeCallable(t *testing.T) {
	d := starlark.NewDict(1)
	_ = d.SetKey(starlark.String("x"), starlark.MakeInt(42))
	_, err := coerceResources(d)
	if err == nil || !strings.Contains(err.Error(), "must be callable") {
		t.Errorf("got %v", err)
	}
}

func TestCoerceResources_ArityTooHigh(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(a, b): return "x"`, "f")
	d := buildResourceDict(t, map[string]starlark.Callable{"x": fns[0]})
	_, err := coerceResources(d)
	if err == nil || !strings.Contains(err.Error(), "0 or 1 parameter") {
		t.Errorf("got %v", err)
	}
}

func TestCoerceResources_ArityZero(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(): return "x"`, "f")
	d := buildResourceDict(t, map[string]starlark.Callable{"x": fns[0]})
	entries, err := coerceResources(d)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if entries[0].arity != 0 {
		t.Errorf("arity = %d, want 0", entries[0].arity)
	}
	if entries[0].uri != "starkite://resources/x" {
		t.Errorf("uri = %q", entries[0].uri)
	}
}

func TestCoerceResources_ArityOne(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(uri): return uri`, "f")
	d := buildResourceDict(t, map[string]starlark.Callable{"x": fns[0]})
	entries, err := coerceResources(d)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if entries[0].arity != 1 {
		t.Errorf("arity = %d, want 1", entries[0].arity)
	}
}

func TestCoerceResources_RejectsVarargs(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(*args): return "x"`, "f")
	d := buildResourceDict(t, map[string]starlark.Callable{"x": fns[0]})
	_, err := coerceResources(d)
	if err == nil || !strings.Contains(err.Error(), "*args") {
		t.Errorf("got %v", err)
	}
}

func TestBuildResourceHandler_ZeroArity_NoArgCall(t *testing.T) {
	fns := buildCallablesFromSource(t, `def f(): return "hello"`, "f")
	r := &resourceEntry{name: "r", uri: "starkite://resources/r", mimeType: "text/plain", fn: fns[0], arity: 0}
	rt := newTestRuntime(t)
	h := buildResourceHandler(r, rt)

	res, err := h(context.Background(), &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: r.uri},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.Contents[0].Text != "hello" {
		t.Errorf("Text = %q", res.Contents[0].Text)
	}
	if res.Contents[0].MIMEType != "text/plain" {
		t.Errorf("MIMEType = %q", res.Contents[0].MIMEType)
	}
	if res.Contents[0].URI != r.uri {
		t.Errorf("URI = %q", res.Contents[0].URI)
	}
}

func TestBuildResourceHandler_OneArity_PassesURI(t *testing.T) {
	fns := buildCallablesFromSource(t, `
def f(uri):
    return "got:" + uri
`, "f")
	r := &resourceEntry{name: "r", uri: "starkite://resources/r", mimeType: "text/plain", fn: fns[0], arity: 1}
	rt := newTestRuntime(t)
	h := buildResourceHandler(r, rt)

	res, err := h(context.Background(), &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: "starkite://resources/r"},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.Contents[0].Text != "got:starkite://resources/r" {
		t.Errorf("Text = %q", res.Contents[0].Text)
	}
}

func TestExtractResourceContent_StringReturn(t *testing.T) {
	text, mime := extractResourceContent(starlark.String("hello"), "text/plain")
	if text != "hello" {
		t.Errorf("text = %q", text)
	}
	if mime != "text/plain" {
		t.Errorf("mime = %q", mime)
	}
}

func TestExtractResourceContent_DictReturn_CustomMIME(t *testing.T) {
	d := starlark.NewDict(2)
	_ = d.SetKey(starlark.String("content"), starlark.String(`{"a":1}`))
	_ = d.SetKey(starlark.String("mime_type"), starlark.String("application/json"))
	text, mime := extractResourceContent(d, "text/plain")
	if text != `{"a":1}` {
		t.Errorf("text = %q", text)
	}
	if mime != "application/json" {
		t.Errorf("mime = %q", mime)
	}
}

func TestExtractResourceContent_DictReturn_MissingMIME_UsesDefault(t *testing.T) {
	d := starlark.NewDict(1)
	_ = d.SetKey(starlark.String("content"), starlark.String("x"))
	text, mime := extractResourceContent(d, "text/plain")
	if text != "x" {
		t.Errorf("text = %q", text)
	}
	if mime != "text/plain" {
		t.Errorf("mime = %q", mime)
	}
}

func TestBuildResourceHandler_DictReturn_SetsCustomMIME(t *testing.T) {
	fns := buildCallablesFromSource(t, `
def f():
    return {"content": "raw json", "mime_type": "application/json"}
`, "f")
	r := &resourceEntry{name: "r", uri: "u", mimeType: "text/plain", fn: fns[0], arity: 0}
	rt := newTestRuntime(t)
	h := buildResourceHandler(r, rt)

	res, err := h(context.Background(), &mcpsdk.ReadResourceRequest{
		Params: &mcpsdk.ReadResourceParams{URI: "u"},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if res.Contents[0].Text != "raw json" {
		t.Errorf("Text = %q", res.Contents[0].Text)
	}
	if res.Contents[0].MIMEType != "application/json" {
		t.Errorf("MIMEType = %q", res.Contents[0].MIMEType)
	}
}
