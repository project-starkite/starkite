package genai

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

// makeThread returns a Starlark thread set up for module tests.
func makeThread(t *testing.T) *starlark.Thread {
	t.Helper()
	return &starlark.Thread{Name: "test"}
}

// loadModule constructs a fresh Module, wires a fake client, and returns
// (module, fake, globals) where globals contains "ai" bound to the module.
func loadModule(t *testing.T) (*Module, *fakeClient, starlark.StringDict) {
	t.Helper()
	m := New()
	fake := installFake(m)
	globals, err := m.Load(&starbase.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return m, fake, globals
}

// execStarlark runs src in a thread with `ai` pre-bound. Returns the globals
// after execution so tests can inspect script-set variables.
func execStarlark(t *testing.T, m *Module, src string) starlark.StringDict {
	t.Helper()
	_, _, globals := loadModule(t)
	_ = m // caller-configured module is already in `globals`
	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "test.star", src, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	return got
}

func TestLoad_ExposesBuiltins(t *testing.T) {
	_, _, globals := loadModule(t)
	aiMod, ok := globals["ai"]
	if !ok {
		t.Fatalf("expected 'ai' in globals, got keys=%v", keys(globals))
	}
	for _, name := range []string{"config", "generate"} {
		val, err := aiMod.(starlark.HasAttrs).Attr(name)
		if err != nil || val == nil {
			t.Errorf("ai.%s missing: err=%v val=%v", name, err, val)
		}
	}
}

func TestGenerate_RequiresPositionalPrompt(t *testing.T) {
	m, _, globals := loadModule(t)
	_ = m
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `ai.generate(model="openai/x")`, globals)
	if err == nil || !strings.Contains(err.Error(), "expected 1 positional argument") {
		t.Fatalf("expected positional-arg error, got %v", err)
	}
}

func TestGenerate_PromptMustBeString(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `ai.generate(42, model="openai/x")`, globals)
	if err == nil || !strings.Contains(err.Error(), "prompt must be a string") {
		t.Fatalf("expected prompt-type error, got %v", err)
	}
}

func TestGenerate_RequiresModel(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `ai.generate("hi")`, globals)
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("expected model-required error, got %v", err)
	}
}

func TestGenerate_UnknownProvider(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `ai.generate("hi", model="mystery/foo")`, globals)
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown-provider error, got %v", err)
	}
}

func TestGenerate_MalformedModel(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `ai.generate("hi", model="nobackslash")`, globals)
	if err == nil || !strings.Contains(err.Error(), "provider/name") {
		t.Fatalf("expected malformed-model error, got %v", err)
	}
}

func TestGenerate_HappyPath_PassesKwargsThrough(t *testing.T) {
	m, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{
		Text:         "response text",
		Model:        "openai/llama3.2",
		InputTokens:  12,
		OutputTokens: 34,
	}}
	_ = m

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
resp = ai.generate("say hi",
    model = "openai/llama3.2",
    system = "be terse",
    temperature = 0.3,
    max_tokens = 100,
    top_p = 0.9,
    stop = ["END"],
)
text = resp.text
model = resp.model
tokens_in = resp.usage.input
tokens_out = resp.usage.output
tokens_total = resp.usage.total
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	// Check script-visible response values.
	assertString(t, got, "text", "response text")
	assertString(t, got, "model", "openai/llama3.2")
	assertInt(t, got, "tokens_in", 12)
	assertInt(t, got, "tokens_out", 34)
	assertInt(t, got, "tokens_total", 46)

	// Check that the kwargs flowed into GenerateRequest.
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	call := fake.calls[0]
	if call.ModelName != "openai/llama3.2" {
		t.Errorf("ModelName = %q", call.ModelName)
	}
	if call.Prompt != "say hi" {
		t.Errorf("Prompt = %q", call.Prompt)
	}
	if call.System != "be terse" {
		t.Errorf("System = %q", call.System)
	}
	if call.Temperature == nil || *call.Temperature != 0.3 {
		t.Errorf("Temperature = %v", call.Temperature)
	}
	if call.MaxTokens == nil || *call.MaxTokens != 100 {
		t.Errorf("MaxTokens = %v", call.MaxTokens)
	}
	if call.TopP == nil || *call.TopP != 0.9 {
		t.Errorf("TopP = %v", call.TopP)
	}
	if len(call.Stop) != 1 || call.Stop[0] != "END" {
		t.Errorf("Stop = %v", call.Stop)
	}
}

// Helpers

func assertString(t *testing.T, g starlark.StringDict, name, want string) {
	t.Helper()
	v, ok := g[name]
	if !ok {
		t.Fatalf("missing %q in globals", name)
	}
	got, ok := starlark.AsString(v)
	if !ok {
		t.Fatalf("%s not a string: %v", name, v)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func assertInt(t *testing.T, g starlark.StringDict, name string, want int) {
	t.Helper()
	v, ok := g[name]
	if !ok {
		t.Fatalf("missing %q in globals", name)
	}
	i, ok := v.(starlark.Int)
	if !ok {
		t.Fatalf("%s not an int: %v", name, v)
	}
	got, ok := i.Int64()
	if !ok {
		t.Fatalf("%s int overflow", name)
	}
	if int(got) != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func keys(d starlark.StringDict) []string {
	out := make([]string, 0, len(d))
	for k := range d {
		out = append(out, k)
	}
	return out
}
