package genai

import (
	"fmt"
	"strings"

	openaiGo "github.com/openai/openai-go"
)

const (
	// defaultOllamaServerAddr is the native Ollama plugin's default server
	// address. The native plugin wants the bare server (no /v1 suffix);
	// it speaks Ollama's native protocol, not the OpenAI-compat one.
	defaultOllamaServerAddr = "http://localhost:11434"

	// Provider prefixes recognized by parseModelString. Each is wired to a
	// Genkit plugin in genkitClient.init.
	providerOpenAI    = "openai"
	providerAnthropic = "anthropic"
	providerGoogleAI  = "googleai"
	providerOllama    = "ollama"
)

// knownProviders is the set of recognized model-string prefixes.
var knownProviders = map[string]struct{}{
	providerOpenAI:    {},
	providerAnthropic: {},
	providerGoogleAI:  {},
	providerOllama:    {},
}

// parseModelString splits a model identifier like "openai/gpt-4o-mini" into
// (provider, model, error). Returns an error if the string has no "/" or the
// provider prefix is not recognized.
func parseModelString(s string) (provider, model string, err error) {
	slash := strings.Index(s, "/")
	if slash == -1 {
		return "", "", fmt.Errorf("model must be of the form 'provider/name' (e.g. 'openai/gpt-4o-mini'), got %q", s)
	}
	provider = s[:slash]
	model = s[slash+1:]
	if provider == "" || model == "" {
		return "", "", fmt.Errorf("model identifier has empty provider or name: %q", s)
	}
	if _, ok := knownProviders[provider]; !ok {
		return "", "", fmt.Errorf("unknown provider %q in model %q (known: %s)", provider, s, knownList())
	}
	return provider, model, nil
}

func knownList() string {
	parts := make([]string, 0, len(knownProviders))
	for p := range knownProviders {
		parts = append(parts, p)
	}
	return strings.Join(parts, ", ")
}

// buildProviderConfig dispatches to a provider-specific config builder. Each
// provider has its own config struct shape (OpenAI's ChatCompletionNewParams,
// Anthropic's MessageNewParams, etc.). When we can't meaningfully translate
// our request kwargs for a given provider, this returns nil (sampling-control
// kwargs like temperature are silently dropped — documented limitation; users
// who need precise control for Tier 1 non-OpenAI providers can pass raw
// provider configs via a future extension).
func buildProviderConfig(provider string, req GenerateRequest) any {
	switch provider {
	case providerOpenAI, providerOllama:
		// Ollama's native plugin also accepts the OpenAI-compat param shape
		// for common fields (temperature, max_tokens, etc.).
		if cfg := buildOpenAIConfig(req); cfg != nil {
			return cfg
		}
	}
	// Anthropic and GoogleAI: v1.5 ships without per-call config; providers
	// use their defaults. Follow-up slice can add provider-specific builders.
	return nil
}

// buildOpenAIConfig constructs the OpenAI-compat config struct from our
// GenerateRequest. Returns nil if no config kwargs were provided (letting
// Genkit use its defaults).
func buildOpenAIConfig(req GenerateRequest) *openaiGo.ChatCompletionNewParams {
	if req.Temperature == nil && req.MaxTokens == nil && req.TopP == nil && len(req.Stop) == 0 {
		return nil
	}
	cfg := &openaiGo.ChatCompletionNewParams{}
	if req.Temperature != nil {
		cfg.Temperature = openaiGo.Float(*req.Temperature)
	}
	if req.MaxTokens != nil {
		cfg.MaxTokens = openaiGo.Int(int64(*req.MaxTokens))
	}
	if req.TopP != nil {
		cfg.TopP = openaiGo.Float(*req.TopP)
	}
	if len(req.Stop) > 0 {
		// openai-go expects a single-value or multi-value stop parameter;
		// the API accepts up to 4 strings.
		cfg.Stop = openaiGo.ChatCompletionNewParamsStopUnion{
			OfStringArray: req.Stop,
		}
	}
	// Note: top_k is not part of the OpenAI Chat Completions schema.
	return cfg
}
