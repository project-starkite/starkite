package genai

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"
)

func TestRunUntil_RequiresChat(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star",
		`ai.run_until("not a chat", "initial")`, globals)
	if err == nil || !contains(err.Error(), "must be an ai.Chat") {
		t.Fatalf("expected type error, got %v", err)
	}
}

func TestRunUntil_RequiresInitialString(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, 42)
`, globals)
	if err == nil || !contains(err.Error(), "initial must be a string") {
		t.Fatalf("expected string error, got %v", err)
	}
}

func TestRunUntil_RejectsEmptyInitial(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, "")
`, globals)
	if err == nil || !contains(err.Error(), "non-empty") {
		t.Fatalf("expected non-empty error, got %v", err)
	}
}

func TestRunUntil_StopWhenTrue_StopsEarly(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: "step1", Model: "openai/x"},
		{Text: "step2 DONE", Model: "openai/x"},
		{Text: "step3", Model: "openai/x"}, // should not be called
	}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
r = ai.run_until(c, "begin", stop_when=lambda resp: "DONE" in resp.text, max_steps=5)
final = r.text
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "final", "step2 DONE")
	if len(fake.calls) != 2 {
		t.Errorf("expected 2 calls (stop_when triggered), got %d", len(fake.calls))
	}
}

func TestRunUntil_MaxStepsCapped(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: "1", Model: "openai/x"},
		{Text: "2", Model: "openai/x"},
		{Text: "3", Model: "openai/x"},
	}

	thread := makeThread(t)
	got, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
r = ai.run_until(c, "begin", max_steps=3)
final = r.text
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	assertString(t, got, "final", "3")
	if len(fake.calls) != 3 {
		t.Errorf("expected 3 calls (max_steps cap), got %d", len(fake.calls))
	}
}

func TestRunUntil_DefaultMaxSteps10(t *testing.T) {
	_, fake, globals := loadModule(t)
	// Script 12 responses; default cap should stop at 10.
	for i := 0; i < 12; i++ {
		fake.script = append(fake.script, &GenerateResult{Text: "x", Model: "openai/x"})
	}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, "go")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	if len(fake.calls) != 10 {
		t.Errorf("expected default max_steps=10, got %d calls", len(fake.calls))
	}
}

func TestRunUntil_FollowUpKwarg_Used(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{
		{Text: "a", Model: "openai/x"},
		{Text: "b", Model: "openai/x"},
	}

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, "first", max_steps=2, follow_up="keep going")
`, globals)
	if err != nil {
		t.Fatalf("ExecFile: %v", err)
	}
	// First call's last user message should be "first"; second's last should be "keep going".
	if len(fake.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fake.calls))
	}
	call1Last := fake.calls[0].History[len(fake.calls[0].History)-1]
	if call1Last.Content != "first" {
		t.Errorf("call[0] last message = %q, want 'first'", call1Last.Content)
	}
	call2Last := fake.calls[1].History[len(fake.calls[1].History)-1]
	if call2Last.Content != "keep going" {
		t.Errorf("call[1] last message = %q, want 'keep going'", call2Last.Content)
	}
}

func TestRunUntil_ErrorDuringLoop_Propagates(t *testing.T) {
	_, fake, globals := loadModule(t)
	fake.script = []*GenerateResult{{Text: "ok", Model: "openai/x"}}
	// Second call will exhaust the script → fake returns an error.

	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, "start", max_steps=5)
`, globals)
	if err == nil || !contains(err.Error(), "step 1") {
		t.Fatalf("expected step-1 error, got %v", err)
	}
}

func TestRunUntil_NegativeMaxSteps_Errors(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, "x", max_steps=-1)
`, globals)
	if err == nil || !contains(err.Error(), "max_steps must be positive") {
		t.Fatalf("expected max_steps error, got %v", err)
	}
}

func TestRunUntil_StopWhen_MustBeCallable(t *testing.T) {
	_, _, globals := loadModule(t)
	thread := makeThread(t)
	_, err := starlark.ExecFile(thread, "t.star", `
c = ai.chat(model="openai/x")
ai.run_until(c, "x", stop_when="notafunc")
`, globals)
	if err == nil || !contains(err.Error(), "stop_when must be callable") {
		t.Fatalf("expected callable error, got %v", err)
	}
}

// silence unused-import warnings if tests move later
var _ = errors.New
