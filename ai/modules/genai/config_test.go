package genai

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestConfig_DefaultModel(t *testing.T) {
	m, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/llama3.2"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.config(default_model = "openai/llama3.2")
resp = ai.generate("hello")
result = resp.text
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}

	if len(fake.calls) != 1 || fake.calls[0].ModelName != "openai/llama3.2" {
		t.Errorf("expected call with default model, got %+v", fake.calls)
	}
	_ = m
}

func TestConfig_PerCallKwargOverridesDefault(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/override"}}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.config(default_model = "openai/llama3.2")
ai.generate("hello", model = "openai/override")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if fake.calls[0].ModelName != "openai/override" {
		t.Errorf("expected kwarg override, got %q", fake.calls[0].ModelName)
	}
}

func TestConfig_APIKeysMustBeStrings(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.config(api_keys = {"openai": 42})`, globals)
	if err == nil || !strings.Contains(err.Error(), "must be a string") {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestConfig_RejectsPositionalArgs(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.config("x")`, globals)
	if err == nil || !strings.Contains(err.Error(), "only keyword arguments") {
		t.Fatalf("expected keyword-only error, got %v", err)
	}
}

func TestConfig_BadTimeout(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.config(timeout = "not-a-duration")`, globals)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestConfig_APIKey_FallsBackToEnvVar(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-value")
	m, _, _ := loadModule(t)
	if got := m.config.apiKeyFor("openai"); got != "env-value" {
		t.Errorf("apiKeyFor(openai) = %q, want %q", got, "env-value")
	}
}

func TestConfig_APIKey_ConfigOverridesEnvVar(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-value")
	m, _, _ := loadModule(t)
	m.config.APIKeys = map[string]string{"openai": "config-value"}
	if got := m.config.apiKeyFor("openai"); got != "config-value" {
		t.Errorf("apiKeyFor(openai) = %q, want %q", got, "config-value")
	}
}

func TestConfig_ResetsClientOnChange(t *testing.T) {
	m, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: "first", Model: "openai/a"},
		{Text: "second", Model: "openai/b"},
	}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
ai.generate("one", model = "openai/a")
ai.config(default_model = "openai/b")       # should reset cached client
ai.generate("two")                          # uses new default, fresh client
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fake.calls))
	}
	// Both calls should have gone through the fake — the production
	// client factory would have been invoked once initially, then again
	// after the config change. The fake doesn't care about that detail,
	// but we prove the second call succeeds and sees the new default.
	if fake.calls[1].ModelName != "openai/b" {
		t.Errorf("call[1].ModelName = %q, want %q", fake.calls[1].ModelName, "openai/b")
	}
	_ = m
}
