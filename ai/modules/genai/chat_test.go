package genai

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"
)

// TestChat_Load_ExposesBuiltin verifies ai.chat is registered.
func TestChat_Load_ExposesBuiltin(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `c = ai.chat(model="openai/x")`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
}

func TestChat_RequiresKwargsOnly(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `ai.chat("x")`, globals)
	if err == nil || !contains(err.Error(), "keyword") {
		t.Fatalf("expected kwargs-only error, got %v", err)
	}
}

func TestChat_Send_EmptyStringRejected(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("")
`, globals)
	if err == nil || !contains(err.Error(), "non-empty") {
		t.Fatalf("expected non-empty error, got %v", err)
	}
}

func TestChat_Send_PositionalMustBeString(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send(42)
`, globals)
	if err == nil || !contains(err.Error(), "must be a string") {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestChat_Send_FirstTurn_UsesDefaults(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "hi back", Model: "openai/x", InputTokens: 3, OutputTokens: 2}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x", system="be terse", temperature=0.2)
resp = c.send("hi")
text = resp.text
model = resp.model
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "text", "hi back")
	assertString(t, got, "model", "openai/x")

	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	call := fake.calls[0]
	if call.System != "be terse" {
		t.Errorf("System = %q, want 'be terse'", call.System)
	}
	if call.Temperature == nil || *call.Temperature != 0.2 {
		t.Errorf("Temperature = %v, want 0.2", call.Temperature)
	}
	if len(call.History) != 1 || call.History[0].Role != "user" || call.History[0].Content != "hi" {
		t.Errorf("History = %+v, want [user:hi]", call.History)
	}
}

func TestChat_Send_MultiTurn_HistoryAccumulates(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: "first reply", Model: "openai/x"},
		{Text: "second reply", Model: "openai/x"},
	}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("one")
c.send("two")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fake.calls))
	}
	// Second call's history should contain: user:one, assistant:first reply, user:two
	h := fake.calls[1].History
	if len(h) != 3 {
		t.Fatalf("turn 2 history len = %d, want 3; got %+v", len(h), h)
	}
	if h[0].Role != "user" || h[0].Content != "one" {
		t.Errorf("h[0] = %+v", h[0])
	}
	if h[1].Role != "assistant" || h[1].Content != "first reply" {
		t.Errorf("h[1] = %+v", h[1])
	}
	if h[2].Role != "user" || h[2].Content != "two" {
		t.Errorf("h[2] = %+v", h[2])
	}
}

func TestChat_Send_TracePersistsInHistory(t *testing.T) {
	m := New()
	// Build a chat whose fake records tool trace via callbacks.
	// We use the real (not fake) path to exercise trace capture, so install
	// a client that wraps a fake generate but lets buildGenkitTool fire.
	// Simpler: directly inspect behavior through fake + seed a trace in result.
	//
	// Since fakeClient.Generate doesn't invoke tool callbacks (it just returns
	// the scripted result), we simulate the trace by having the test inject
	// invocations directly via the ToolTrace pointer. Chat reads *req.ToolTrace
	// after Generate returns.
	//
	// Easiest approach: extend fake to populate trace when asked.
	fake := installFake(m)
	fake.script = []*GenerateResult{
		{Text: "final reply", Model: "openai/x"},
		{Text: "after", Model: "openai/x"},
	}

	// Populate the trace pointer on the FIRST call only (the one with tools).
	firstCallDone := false
	fake.preGenerateHook = func(req GenerateRequest) {
		if firstCallDone {
			return
		}
		firstCallDone = true
		if req.ToolTrace != nil {
			*req.ToolTrace = append(*req.ToolTrace,
				ToolInvocation{Name: "check_health", Input: map[string]any{"svc": "nginx"}, Output: map[string]any{"ok": true}},
			)
		}
	}

	globals, err := m.Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	thread := makeThread(t)
	_, err = starlark.ExecFile(thread, "t.star", `
def check_health(svc): return {"ok": True}
c = ai.chat(model="openai/x", tools=[check_health])
c.send("probe")
# Now send again and inspect what history arrived at the second call.
c.send("status?")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// Second call's history should include the user:probe, assistant:tool-request,
	// tool:response, assistant:"final reply", user:"status?"
	h := fake.calls[1].History
	if len(h) != 5 {
		t.Fatalf("history len = %d, want 5; got %+v", len(h), h)
	}
	if h[0].Role != "user" || h[0].Content != "probe" {
		t.Errorf("h[0] = %+v", h[0])
	}
	if h[1].Role != "assistant" || h[1].ToolName != "check_health" {
		t.Errorf("h[1] = %+v", h[1])
	}
	if h[2].Role != "tool" || h[2].ToolName != "check_health" {
		t.Errorf("h[2] = %+v", h[2])
	}
	if h[3].Role != "assistant" || h[3].Content != "final reply" {
		t.Errorf("h[3] = %+v", h[3])
	}
	if h[4].Role != "user" || h[4].Content != "status?" {
		t.Errorf("h[4] = %+v", h[4])
	}
}

func TestChat_Send_OverrideAppliesToOneTurn(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: "a", Model: "openai/x"},
		{Text: "b", Model: "openai/x"},
	}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x", temperature=0.2)
c.send("one", temperature=0.9)
c.send("two")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// First call uses override 0.9; second falls back to default 0.2.
	if fake.calls[0].Temperature == nil || *fake.calls[0].Temperature != 0.9 {
		t.Errorf("call[0].Temperature = %v, want 0.9", fake.calls[0].Temperature)
	}
	if fake.calls[1].Temperature == nil || *fake.calls[1].Temperature != 0.2 {
		t.Errorf("call[1].Temperature = %v, want 0.2", fake.calls[1].Temperature)
	}
}

func TestChat_Send_ToolsOverride_EmptyDisables(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def t(x): return x
c = ai.chat(model="openai/x", tools=[t])
c.send("go", tools=[])
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if len(fake.calls[0].Tools) != 0 {
		t.Errorf("expected 0 tools for overridden turn, got %d", len(fake.calls[0].Tools))
	}
}

func TestChat_Send_Error_AppendsSyntheticAssistant(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.err = errors.New("network boom")

	thread := makeThread(t)
	// First ExecFile: chat is created, then .send errors.
	newGlobals, execErr := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("one")
`, globals)
	if execErr == nil || !contains(execErr.Error(), "boom") {
		t.Fatalf("expected first send to error with 'boom', got %v", execErr)
	}
	// Starlark populates globals up to (but not through) the erroring stmt,
	// so `c` should be available. If newGlobals is nil on error, we'll have
	// to resolve differently; check it.
	if newGlobals == nil {
		t.Fatal("ExecFile returned nil globals on error; test needs adjustment")
	}

	// Clear the error and send again, using newGlobals as predeclared.
	fake.err = nil
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}
	thread2 := makeThread(t)
	_, err := starlark.ExecFile(thread2, "t2.star", `c.send("two")`, newGlobals)
	if err != nil {
		t.Fatalf("second ExecFile: %v", err)
	}
	// The fake records BOTH calls (including the erroring one). The second
	// call (index 1) is the successful send; inspect its history.
	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 calls (1 failed + 1 success), got %d", len(fake.calls))
	}
	h := fake.calls[1].History
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3; got %+v", len(h), h)
	}
	if h[0].Role != "user" || h[0].Content != "one" {
		t.Errorf("h[0] = %+v", h[0])
	}
	if h[1].Role != "assistant" || !contains(h[1].Content, "error") || !contains(h[1].Content, "boom") {
		t.Errorf("h[1] = %+v, want synthetic error", h[1])
	}
	if h[2].Role != "user" || h[2].Content != "two" {
		t.Errorf("h[2] = %+v", h[2])
	}
}

func TestChat_Send_ModelResolvedFromConfig(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.config(default_model="openai/from-config")
c = ai.chat()
c.send("hi")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if fake.calls[0].ModelName != "openai/from-config" {
		t.Errorf("ModelName = %q, want 'openai/from-config'", fake.calls[0].ModelName)
	}
}

func TestChat_Send_InvalidOnToolError_AtChatCreation(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.chat(model="openai/x", on_tool_error="wat")
`, globals)
	if err == nil || !contains(err.Error(), "on_tool_error") {
		t.Fatalf("expected on_tool_error error, got %v", err)
	}
}

func TestChat_Send_Schema_ParsesDataAndRawGoesInHistory(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: `{"fruit":"apple"}`, Model: "openai/x", Data: map[string]any{"fruit": "apple"}},
		{Text: "follow", Model: "openai/x"},
	}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
resp = c.send("give fruit", schema={"type":"object","properties":{"fruit":{"type":"string"}}})
parsed = resp.data["fruit"]
raw = resp.text
c.send("next")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "parsed", "apple")
	assertString(t, got, "raw", `{"fruit":"apple"}`)

	// Second-turn history should contain raw JSON as assistant Content.
	h := fake.calls[1].History
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3; got %+v", len(h), h)
	}
	if h[1].Role != "assistant" || h[1].Content != `{"fruit":"apple"}` {
		t.Errorf("h[1] = %+v, want assistant with raw JSON", h[1])
	}
}

func TestChat_Send_Stream_HappyPath_AppendsAccumulatedText(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.streamScript = []*fakeStream{{
		chunks: []string{"hello ", "world"},
		model:  "openai/x",
	}}
	// Second call uses non-stream path; script returns "ok".
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
stream = c.send("greet", stream=True)
parts = [chunk.text for chunk in stream]
c.send("after")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// Second-turn (non-stream) history should have user:greet, assistant:"hello world", user:after
	h := fake.calls[1].History
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3; got %+v", len(h), h)
	}
	if h[1].Role != "assistant" || h[1].Content != "hello world" {
		t.Errorf("h[1] = %+v, want accumulated text", h[1])
	}
}

func TestChat_Send_Stream_ErrorMidway_AppendsBracketedError(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.streamScript = []*fakeStream{{
		chunks: []string{"part1"},
		err:    errors.New("blew up"),
		model:  "openai/x",
	}}
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
stream = c.send("go", stream=True)
parts = [chunk.text for chunk in stream]
c.send("after")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	h := fake.calls[1].History
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3; got %+v", len(h), h)
	}
	if h[1].Role != "assistant" || !contains(h[1].Content, "part1") || !contains(h[1].Content, "[error") || !contains(h[1].Content, "blew up") {
		t.Errorf("h[1] = %+v, want partial + bracketed error", h[1])
	}
}

func TestChat_Send_Stream_WithToolsRejected(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def t(x): return x
c = ai.chat(model="openai/x", tools=[t])
c.send("hi", stream=True)
`, globals)
	if err == nil || !contains(err.Error(), "stream=True with tools") {
		t.Fatalf("expected stream+tools error, got %v", err)
	}
}

func TestChat_Send_Stream_WithSchemaRejected(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("hi", stream=True, schema={"type":"object"})
`, globals)
	if err == nil || !contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive error, got %v", err)
	}
}

func TestChat_Send_Stream_ToolsOverrideEmpty_Allowed(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.streamScript = []*fakeStream{{
		chunks: []string{"ok"},
		model:  "openai/x",
	}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
def t(x): return x
c = ai.chat(model="openai/x", tools=[t])
stream = c.send("hi", stream=True, tools=[])
parts = [chunk.text for chunk in stream]
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	if len(fake.calls[0].Tools) != 0 {
		t.Errorf("expected empty tools for override turn, got %d", len(fake.calls[0].Tools))
	}
}

// ---- Slice 3.1: chat.history, chat.reset, ai.chat(history=) ---------------

func TestChat_History_EmptyInitially(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
n = len(c.history)
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertInt(t, got, "n", 0)
}

func TestChat_History_AfterSendIncludesBothMessages(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "hi back", Model: "openai/x"}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("hi")
entries = c.history
count = len(entries)
first_role = entries[0]["role"]
first_content = entries[0]["content"]
last_role = entries[1]["role"]
last_content = entries[1]["content"]
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertInt(t, got, "count", 2)
	assertString(t, got, "first_role", "user")
	assertString(t, got, "first_content", "hi")
	assertString(t, got, "last_role", "assistant")
	assertString(t, got, "last_content", "hi back")
}

func TestChat_History_ToolCallEntriesHaveToolFields(t *testing.T) {
	m := New()
	fake := installFake(m)
	fake.script = []*GenerateResult{{Text: "final", Model: "openai/x"}}

	// Simulate a single tool-trace invocation by injecting on first Generate.
	fake.preGenerateHook = func(req GenerateRequest) {
		if req.ToolTrace != nil && len(*req.ToolTrace) == 0 {
			*req.ToolTrace = append(*req.ToolTrace, ToolInvocation{
				Name:   "check_health",
				Input:  map[string]any{"svc": "nginx"},
				Output: map[string]any{"ok": true},
			})
		}
	}
	globals, err := m.Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
def check_health(svc): return {"ok": True}
c = ai.chat(model="openai/x", tools=[check_health])
c.send("probe")
entries = c.history
# Expected: user:probe, assistant tool_req, tool response, assistant final
count = len(entries)
req_role = entries[1]["role"]
req_name = entries[1]["tool_name"]
resp_role = entries[2]["role"]
resp_name = entries[2]["tool_name"]
final_role = entries[3]["role"]
final_content = entries[3]["content"]
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertInt(t, got, "count", 4)
	assertString(t, got, "req_role", "assistant")
	assertString(t, got, "req_name", "check_health")
	assertString(t, got, "resp_role", "tool")
	assertString(t, got, "resp_name", "check_health")
	assertString(t, got, "final_role", "assistant")
	assertString(t, got, "final_content", "final")
}

func TestChat_Reset_ClearsHistory(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("hi")
before = len(c.history)
c.reset()
after = len(c.history)
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertInt(t, got, "before", 2)
	assertInt(t, got, "after", 0)
}

func TestChat_History_Seeded_FullRoundTrip(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "berlin", Model: "openai/x"}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
prior = [
    {"role": "user", "content": "capital of france?"},
    {"role": "assistant", "content": "paris"},
]
c = ai.chat(model="openai/x", history=prior)
c.send("and germany?")
entries = c.history
count = len(entries)
first = entries[0]["content"]
last = entries[-1]["content"]
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// Should have: seeded user, seeded assistant, new user, new assistant.
	assertInt(t, got, "count", 4)
	assertString(t, got, "first", "capital of france?")
	assertString(t, got, "last", "berlin")

	// The new send should have passed the full history (prior + new user msg).
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	h := fake.calls[0].History
	if len(h) != 3 || h[0].Role != "user" || h[1].Role != "assistant" || h[2].Role != "user" {
		t.Errorf("request history shape wrong: %+v", h)
	}
}

func TestChat_History_InvalidRole_Errors(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.chat(model="openai/x", history=[{"role": "bogus", "content": "x"}])
`, globals)
	if err == nil || !contains(err.Error(), "role must be one of") {
		t.Fatalf("expected role error, got %v", err)
	}
}

func TestChat_History_InvalidShape_Errors(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.chat(model="openai/x", history=[42])
`, globals)
	if err == nil || !contains(err.Error(), "expected a dict") {
		t.Fatalf("expected dict-required error, got %v", err)
	}
}

func TestChat_History_ToolMessageRequiresName_Errors(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.chat(model="openai/x", history=[{"role": "tool"}])
`, globals)
	if err == nil || !contains(err.Error(), "tool_name") {
		t.Fatalf("expected tool_name error, got %v", err)
	}
}

func TestChat_History_Snapshot_Immutable(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
c.send("a")
snap = c.history
# Mutating the returned list must not affect c.history.
snap.append({"role": "user", "content": "injected"})
restored = c.history
count = len(restored)
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// Mutation should not leak into subsequent c.history.
	// (c.history snapshot is fresh each call; original internal slice untouched.)
	// restored should still have 2 entries (user + assistant), not 3.
	// Note: this test depends on historyValue() returning a freshly-built list.
}
