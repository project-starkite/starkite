package genai

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"
)

// TestGenerate_PerCallOverridesCreateFreshClient verifies that passing
// api_key= or base_url= produces a one-shot client that isn't cached — each
// such call should construct a new client.
func TestGenerate_PerCallOverridesCreateFreshClient(t *testing.T) {
	m := New()
	constructed := 0
	m.newClientFunc = func(cv configView) genaiClient {
		constructed++
		return &fakeClient{script: []*GenerateResult{{Text: "ok", Model: "openai/x"}}}
	}
	globals, err := m.Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	thread := makeThread(t)
	// Two calls, both with base_url override. Each should construct a new client.
	_, err = starlark.ExecFile(thread, "t.star", `
ai.generate("a", model = "openai/x", base_url = "http://host-1/v1")
ai.generate("b", model = "openai/x", base_url = "http://host-2/v1")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if constructed != 2 {
		t.Errorf("constructed = %d, want 2 (one per call with override)", constructed)
	}

	// A call without overrides should reuse the cached client — separate
	// test to avoid coupling: run it on a fresh module so we don't count
	// earlier constructions.
}

// TestGenerate_NoOverrides_CachesClient verifies repeated calls without
// api_key/base_url overrides share one cached client.
func TestGenerate_NoOverrides_CachesClient(t *testing.T) {
	m := New()
	constructed := 0
	m.newClientFunc = func(cv configView) genaiClient {
		constructed++
		return &fakeClient{script: []*GenerateResult{
			{Text: "a", Model: "openai/x"},
			{Text: "b", Model: "openai/x"},
		}}
	}
	globals, err := m.Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	thread := makeThread(t)
	_, err = starlark.ExecFile(thread, "t.star", `
ai.generate("a", model = "openai/x")
ai.generate("b", model = "openai/x")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if constructed != 1 {
		t.Errorf("constructed = %d, want 1 (cached across calls)", constructed)
	}
}

// TestGenerate_APIKeyOverride_PlacedUnderProviderPrefix verifies the per-call
// api_key= lands in the right provider slot on the configView.
func TestGenerate_APIKeyOverride_PlacedUnderProviderPrefix(t *testing.T) {
	m := New()
	var captured configView
	m.newClientFunc = func(cv configView) genaiClient {
		captured = cv
		return &fakeClient{script: []*GenerateResult{{Text: "ok", Model: "openai/x"}}}
	}
	globals, err := m.Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	thread := makeThread(t)
	_, err = starlark.ExecFile(thread, "t.star",
		`ai.generate("hi", model = "openai/x", api_key = "sk-test")`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if got := captured.apiKeys["openai"]; got != "sk-test" {
		t.Errorf("apiKeys[openai] = %q, want %q", got, "sk-test")
	}
}

// TestGenerate_Stream_HappyPath drives ai.generate(..., stream=True) through
// the fake and checks that the yielded chunks have the expected .text and
// the stream's .error is "" after clean completion.
func TestGenerate_Stream_HappyPath(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.streamScript = []*fakeStream{{
		chunks:       []string{"chunk1 ", "chunk2 ", "chunk3"},
		model:        "openai/llama3.2",
		inputTokens:  8,
		outputTokens: 16,
	}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
stream = ai.generate("hi", model="openai/llama3.2", stream=True)
parts = [chunk.text for chunk in stream]
collected = "".join(parts)
count = len(parts)
err_str = stream.error
in_tokens = stream.usage.input
out_tokens = stream.usage.output
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "collected", "chunk1 chunk2 chunk3")
	assertInt(t, got, "count", 3)
	assertString(t, got, "err_str", "")
	assertInt(t, got, "in_tokens", 8)
	assertInt(t, got, "out_tokens", 16)
}

// TestGenerate_Stream_ErrorSetAfterInterrupted — fake delivers 2 chunks and
// then signals an error via StreamResult.err. Iteration yields the chunks
// cleanly; .error is populated after the loop.
func TestGenerate_Stream_ErrorSetAfterInterrupted(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.streamScript = []*fakeStream{{
		chunks: []string{"part1 ", "part2"},
		err:    errors.New("rate limited"),
		model:  "openai/x",
	}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
stream = ai.generate("hi", model="openai/x", stream=True)
parts = [chunk.text for chunk in stream]
collected = "".join(parts)
err_str = stream.error
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "collected", "part1 part2")
	assertString(t, got, "err_str", "rate limited")
}

// TestGenerate_Stream_NoMoreIterationAfterConsumed — iterating a StreamValue
// twice yields nothing the second time (channel already drained).
func TestGenerate_Stream_NoMoreIterationAfterConsumed(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.streamScript = []*fakeStream{{
		chunks: []string{"a", "b"},
		model:  "openai/x",
	}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
stream = ai.generate("hi", model="openai/x", stream=True)
first_parts = [chunk.text for chunk in stream]
second_parts = [chunk.text for chunk in stream]
first_count = len(first_parts)
second_count = len(second_parts)
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertInt(t, got, "first_count", 2)
	assertInt(t, got, "second_count", 0)
}

// TestGenerate_Schema_ParsedIntoData — when schema= is passed, the parsed
// Data from the fake is exposed via resp.data as a Starlark value.
func TestGenerate_Schema_ParsedIntoData(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{
		Text:  `{"a":1,"b":"x"}`,
		Model: "openai/x",
		Data:  map[string]any{"a": int64(1), "b": "x"},
	}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
resp = ai.generate("give me json",
    model = "openai/x",
    schema = {"type": "object", "properties": {"a": {"type": "integer"}, "b": {"type": "string"}}},
)
raw = resp.text
a_val = resp.data["a"]
b_val = resp.data["b"]
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "raw", `{"a":1,"b":"x"}`)
	assertInt(t, got, "a_val", 1)
	assertString(t, got, "b_val", "x")

	// Verify the schema reached the request layer.
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(fake.calls))
	}
	if fake.calls[0].Schema == nil {
		t.Errorf("expected Schema on request, got nil")
	}
	if fake.calls[0].Schema["type"] != "object" {
		t.Errorf("Schema[type] = %v, want %q", fake.calls[0].Schema["type"], "object")
	}
}

// TestGenerate_Schema_MustBeDict verifies that a non-dict schema produces
// a type error at the Starlark-facing layer.
func TestGenerate_Schema_MustBeDict(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.generate("hi", model="openai/x", schema=42)`, globals)
	if err == nil || !contains(err.Error(), "schema must be a dict") {
		t.Fatalf("expected schema-type error, got %v", err)
	}
}

// TestGenerate_StreamAndSchemaMutuallyExclusive rejects the combination.
func TestGenerate_StreamAndSchemaMutuallyExclusive(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.generate("hi", model="openai/x", stream=True, schema={"type":"object"})`, globals)
	if err == nil || !contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive error, got %v", err)
	}
}

// TestGenerate_Schema_DataNoneWithoutSchema — a normal call (no schema=)
// exposes resp.data as None.
func TestGenerate_Schema_DataNoneWithoutSchema(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "hi", Model: "openai/x"}}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
resp = ai.generate("hi", model="openai/x")
data_is_none = (resp.data == None)
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	v, ok := got["data_is_none"]
	if !ok {
		t.Fatal("missing data_is_none")
	}
	if v != starlark.True {
		t.Errorf("data_is_none = %v, want True", v)
	}
}

// contains is a small helper so test messages don't need strings import dup.
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
