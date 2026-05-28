package genai

import (
	"errors"
	"reflect"
	"testing"

	"go.starlark.net/starlark"
)

// --- ai.tool() and inference ---

func TestTool_InferName_UsesFunctionName(t *testing.T) {
	_, fake, globals := loadModule(t)
	_ = fake

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
def deploy_app(name):
    "Deploy an application."
    return {"ok": True}
tool = ai.tool(deploy_app)
name = tool.name
desc = tool.description
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "name", "deploy_app")
	assertString(t, got, "desc", "Deploy an application.")
}

func TestTool_InferParams_FromDefaults(t *testing.T) {
	m := New()
	globals, err := m.Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
def f(a, b = 1, c = "", d = True, e = 0.5):
    return None
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// Test inferToolSchema directly by extracting the *Function.
	fn := globalsAsFunction(t, got, "f")
	to, err := inferToolSchema(fn)
	if err != nil {
		t.Fatalf("inferToolSchema: %v", err)
	}

	props := to.params["properties"].(map[string]any)
	assertSchemaType(t, props, "a", "string") // required → fallback string
	assertSchemaType(t, props, "b", "integer")
	assertSchemaType(t, props, "c", "string")
	assertSchemaType(t, props, "d", "boolean")
	assertSchemaType(t, props, "e", "number")

	req, _ := to.params["required"].([]string)
	if !reflect.DeepEqual(req, []string{"a"}) {
		t.Errorf("required = %v, want [a]", req)
	}
}

func TestTool_RejectsVarargs(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def f(*args): return args
ai.tool(f)
`, globals)
	if err == nil || !contains(err.Error(), "*args") {
		t.Fatalf("expected *args rejection, got %v", err)
	}
}

func TestTool_RejectsKwargs(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def f(**kw): return kw
ai.tool(f)
`, globals)
	if err == nil || !contains(err.Error(), "**kwargs") {
		t.Fatalf("expected **kwargs rejection, got %v", err)
	}
}

func TestTool_ExplicitOverride(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
def f(x):
    "original docstring"
    return x
tool = ai.tool(f,
    description = "new description",
    params = {"type": "object", "properties": {"x": {"type": "integer"}}, "required": ["x"]},
)
name = tool.name
desc = tool.description
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "name", "f")
	assertString(t, got, "desc", "new description")
}

func TestTool_PartialOverride_DescriptionOnly(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
def f(x):
    "inferred doc"
    return x
t = ai.tool(f, description = "custom")
desc = t.description
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "desc", "custom")
}

func TestTool_PartialOverride_ParamsOnly(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
def f(x):
    "keep this doc"
    return x
t = ai.tool(f, params = {"type":"object","properties":{"x":{"type":"integer"}}})
desc = t.description
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "desc", "keep this doc")
}

// --- ai.generate(tools=) ---

func TestGenerate_PlainFnInToolsList(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def check(service):
    "Check service."
    return {"ok": True}
ai.generate("probe", model = "openai/x", tools = [check])
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	call := fake.calls[0]
	if len(call.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(call.Tools))
	}
	if call.Tools[0].name != "check" {
		t.Errorf("tool name = %q, want %q", call.Tools[0].name, "check")
	}
	if call.Tools[0].description != "Check service." {
		t.Errorf("tool desc = %q", call.Tools[0].description)
	}
}

func TestGenerate_MixedExplicitAndShorthand(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def a(x): return x
def b(y):
    "with doc"
    return y
explicit = ai.tool(a, description = "explicit a", params = {"type":"object","properties":{"x":{"type":"string"}}})
ai.generate("hi", model = "openai/x", tools = [explicit, b])
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	call := fake.calls[0]
	if len(call.Tools) != 2 {
		t.Fatalf("want 2 tools, got %d", len(call.Tools))
	}
	if call.Tools[0].description != "explicit a" {
		t.Errorf("tool[0] desc = %q", call.Tools[0].description)
	}
	if call.Tools[1].description != "with doc" {
		t.Errorf("tool[1] desc = %q", call.Tools[1].description)
	}
}

func TestGenerate_InvalidOnToolError(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.generate("x", model="openai/x", on_tool_error="wat")`, globals)
	if err == nil || !contains(err.Error(), "on_tool_error") {
		t.Fatalf("expected on_tool_error error, got %v", err)
	}
}

func TestGenerate_StreamWithToolsRejected(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def f(x): return x
ai.generate("x", model="openai/x", stream=True, tools=[f])
`, globals)
	if err == nil || !contains(err.Error(), "stream=True with tools") {
		t.Fatalf("expected stream+tools error, got %v", err)
	}
}

func TestGenerate_ToolsMustBeFuncOrTool(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.generate("x", model="openai/x", tools=[42])`, globals)
	if err == nil || !contains(err.Error(), "function or ai.Tool") {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestGenerate_MaxIterations_DefaultsTo10(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.generate("x", model="openai/x")`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if fake.calls[0].MaxTurns != 10 {
		t.Errorf("MaxTurns = %d, want 10", fake.calls[0].MaxTurns)
	}
}

func TestGenerate_MaxIterations_Override(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.generate("x", model="openai/x", max_iterations=3)`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if fake.calls[0].MaxTurns != 3 {
		t.Errorf("MaxTurns = %d, want 3", fake.calls[0].MaxTurns)
	}
}

// --- Tool dispatch (invokeToolCallback) ---

func TestToolDispatch_CallbackReceivesParsedInput(t *testing.T) {
	thread := makeThread(t)
	// Build a real Starlark function via ExecFile, then reach into globals.
	src := `
def tool_fn(name, count):
    return {"saw_name": name, "saw_count": count}
`
	globals, err := starlark.ExecFile(thread, "t.star", src, nil)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	fn := globals["tool_fn"].(*starlark.Function)
	tool := &Tool{name: "tool_fn", fn: fn}

	result, err := invokeToolCallback(tool, map[string]any{"name": "nginx", "count": int64(5)}, thread, "halt")
	if err != nil {
		t.Fatalf("invokeToolCallback: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	if m["saw_name"] != "nginx" {
		t.Errorf("saw_name = %v", m["saw_name"])
	}
	if m["saw_count"] != int64(5) {
		t.Errorf("saw_count = %v", m["saw_count"])
	}
}

func TestToolDispatch_OnToolError_Halt(t *testing.T) {
	thread := makeThread(t)
	src := `
def bad():
    fail("boom")
`
	globals, err := starlark.ExecFile(thread, "t.star", src, nil)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	fn := globals["bad"].(*starlark.Function)
	tool := &Tool{name: "bad", fn: fn}

	_, err = invokeToolCallback(tool, map[string]any{}, thread, "halt")
	if err == nil {
		t.Fatal("expected error in halt mode")
	}
	if !contains(err.Error(), "boom") {
		t.Errorf("err does not contain 'boom': %v", err)
	}
}

func TestToolDispatch_OnToolError_Feedback(t *testing.T) {
	thread := makeThread(t)
	src := `
def bad():
    fail("boom")
`
	globals, err := starlark.ExecFile(thread, "t.star", src, nil)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	fn := globals["bad"].(*starlark.Function)
	tool := &Tool{name: "bad", fn: fn}

	result, err := invokeToolCallback(tool, map[string]any{}, thread, "feedback")
	if err != nil {
		t.Fatalf("expected no Go-level error in feedback mode, got %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map", result)
	}
	msg, _ := m["error"].(string)
	if !contains(msg, "boom") {
		t.Errorf("error payload does not contain 'boom': %q", msg)
	}
}

// --- helpers ---

func globalsAsFunction(t *testing.T, globals starlark.StringDict, name string) *starlark.Function {
	t.Helper()
	v, ok := globals[name]
	if !ok {
		t.Fatalf("no %q in globals", name)
	}
	fn, ok := v.(*starlark.Function)
	if !ok {
		t.Fatalf("%q is %T, not *starlark.Function", name, v)
	}
	return fn
}

func assertSchemaType(t *testing.T, props map[string]any, key, wantType string) {
	t.Helper()
	entry, ok := props[key].(map[string]any)
	if !ok {
		t.Fatalf("props[%q] missing or not a map: %v", key, props[key])
	}
	gotType, _ := entry["type"].(string)
	if gotType != wantType {
		t.Errorf("props[%q].type = %q, want %q", key, gotType, wantType)
	}
}

// Silence "imported and not used" when tests above drop the errors import
// from prior iterations.
var _ = errors.New
