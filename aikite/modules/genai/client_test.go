package genai

import (
	"strings"
	"testing"
)

// --- resolveKey & providerAPIKey ------------------------------------------

func TestResolveKey_ConfigOverride(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	v := configView{apiKeys: map[string]string{"openai": "cfg-key"}}
	if got := resolveKey(v, "openai"); got != "cfg-key" {
		t.Errorf("resolveKey = %q, want cfg-key", got)
	}
}

func TestResolveKey_EnvFallback(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-key")
	v := configView{apiKeys: map[string]string{}}
	if got := resolveKey(v, "openai"); got != "env-key" {
		t.Errorf("resolveKey = %q, want env-key", got)
	}
}

func TestResolveKey_Missing(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	v := configView{apiKeys: map[string]string{}}
	if got := resolveKey(v, "openai"); got != "" {
		t.Errorf("resolveKey = %q, want empty", got)
	}
}

func TestResolveKey_GoogleAI_PrefersGemini(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("GOOGLE_API_KEY", "google-key")
	v := configView{apiKeys: map[string]string{}}
	if got := resolveKey(v, "googleai"); got != "gemini-key" {
		t.Errorf("resolveKey = %q, want gemini-key", got)
	}
}

func TestResolveKey_GoogleAI_FallsBackToGoogleAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "google-key")
	v := configView{apiKeys: map[string]string{}}
	if got := resolveKey(v, "googleai"); got != "google-key" {
		t.Errorf("resolveKey = %q, want google-key", got)
	}
}

func TestResolveKey_GoogleAI_ConfigBeatsBothEnvVars(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("GOOGLE_API_KEY", "google-key")
	v := configView{apiKeys: map[string]string{"googleai": "cfg-key"}}
	if got := resolveKey(v, "googleai"); got != "cfg-key" {
		t.Errorf("resolveKey = %q, want cfg-key", got)
	}
}

// --- checkAvailability ----------------------------------------------------

func TestCheckAvailability_UnknownProvider(t *testing.T) {
	active := map[string]bool{"ollama": true}
	err := checkAvailability(active, "wat/foo")
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown-provider error, got %v", err)
	}
}

func TestCheckAvailability_InactiveProvider(t *testing.T) {
	active := map[string]bool{"ollama": true}
	err := checkAvailability(active, "anthropic/claude-3")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "anthropic") {
		t.Errorf("error missing provider name: %q", msg)
	}
	if !strings.Contains(msg, "ANTHROPIC_API_KEY") {
		t.Errorf("error missing env-var hint: %q", msg)
	}
	if !strings.Contains(msg, "unavailable") {
		t.Errorf("error missing 'unavailable': %q", msg)
	}
}

func TestCheckAvailability_GoogleAI_MentionsBothKeys(t *testing.T) {
	active := map[string]bool{"ollama": true}
	err := checkAvailability(active, "googleai/gemini-2.5-flash")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "GEMINI_API_KEY") {
		t.Errorf("error missing GEMINI_API_KEY: %q", msg)
	}
	if !strings.Contains(msg, "GOOGLE_API_KEY") {
		t.Errorf("error missing GOOGLE_API_KEY: %q", msg)
	}
}

func TestCheckAvailability_ActiveProvider(t *testing.T) {
	active := map[string]bool{"openai": true, "ollama": true}
	if err := checkAvailability(active, "openai/gpt-4o"); err != nil {
		t.Errorf("expected no error for active provider, got %v", err)
	}
}

func TestCheckAvailability_OllamaAlwaysAvailable(t *testing.T) {
	// In production, Ollama is always registered. Test the contract: when
	// "ollama" is in active, ollama/ models pass.
	active := map[string]bool{"ollama": true}
	if err := checkAvailability(active, "ollama/llama3.2"); err != nil {
		t.Errorf("expected no error for ollama, got %v", err)
	}
}

func TestCheckAvailability_MalformedModelString(t *testing.T) {
	active := map[string]bool{"ollama": true}
	err := checkAvailability(active, "noSlash")
	if err == nil {
		t.Fatal("expected error for malformed model string")
	}
	if !strings.Contains(err.Error(), "provider/name") {
		t.Errorf("expected malformed-model error, got %q", err.Error())
	}
}
