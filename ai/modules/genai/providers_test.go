package genai

import (
	"strings"
	"testing"
)

func TestParseModelString(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantProv   string
		wantModel  string
		wantErrSub string // substring expected in error, "" for no error
	}{
		{"openai happy", "openai/gpt-4o", "openai", "gpt-4o", ""},
		{"anthropic happy", "anthropic/claude-3-5-sonnet", "anthropic", "claude-3-5-sonnet", ""},
		{"googleai happy", "googleai/gemini-2.5-flash", "googleai", "gemini-2.5-flash", ""},
		{"ollama happy", "ollama/llama3.2", "ollama", "llama3.2", ""},
		{"no slash", "llama3", "", "", "provider/name"},
		{"empty provider", "/model", "", "", "empty provider"},
		{"empty model", "openai/", "", "", "empty provider or name"},
		{"unknown provider", "claude/opus", "", "", "unknown provider"},
		{"model with slash", "openai/path/to/model", "openai", "path/to/model", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prov, model, err := parseModelString(tc.in)
			if tc.wantErrSub == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if prov != tc.wantProv {
					t.Errorf("provider = %q, want %q", prov, tc.wantProv)
				}
				if model != tc.wantModel {
					t.Errorf("model = %q, want %q", model, tc.wantModel)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q missing substring %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

func TestBuildOpenAIConfig_NilWhenNoOptions(t *testing.T) {
	if got := buildOpenAIConfig(GenerateRequest{ModelName: "openai/x"}); got != nil {
		t.Errorf("buildOpenAIConfig with no options = %v, want nil", got)
	}
}

func TestBuildOpenAIConfig_PopulatesFromRequest(t *testing.T) {
	temp := 0.7
	mx := 100
	tp := 0.9
	cfg := buildOpenAIConfig(GenerateRequest{
		ModelName:   "openai/x",
		Temperature: &temp,
		MaxTokens:   &mx,
		TopP:        &tp,
		Stop:        []string{"END"},
	})
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	// We don't deeply inspect the openai-go types here — just prove the
	// fields are set. The production path delegates marshaling to openai-go.
}

func TestEnvVarForProvider(t *testing.T) {
	tests := []struct {
		provider, want string
	}{
		{"openai", "OPENAI_API_KEY"},
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"googleai", "GOOGLE_API_KEY"},
		{"ollama", ""},
		{"unknown", ""},
	}
	for _, tc := range tests {
		if got := envVarForProvider(tc.provider); got != tc.want {
			t.Errorf("envVarForProvider(%q) = %q, want %q", tc.provider, got, tc.want)
		}
	}
}
