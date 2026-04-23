package genai

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Config holds module-global defaults set via ai.config(...). Callers may
// still override any field via per-call kwargs on ai.generate() / ai.chat().
type Config struct {
	DefaultModel string
	APIKeys      map[string]string // provider (lowercase) -> API key
	BaseURLs     map[string]string // provider (lowercase) -> base URL
	Timeout      time.Duration

	mu sync.RWMutex
}

// newConfig returns a fresh Config with empty maps.
func newConfig() *Config {
	return &Config{
		APIKeys:  map[string]string{},
		BaseURLs: map[string]string{},
	}
}

// snapshot returns a read-only copy of the fields the client layer needs.
// Callers that mutate the returned maps have their own copy.
func (c *Config) snapshot() configView {
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make(map[string]string, len(c.APIKeys))
	for k, v := range c.APIKeys {
		keys[k] = v
	}
	urls := make(map[string]string, len(c.BaseURLs))
	for k, v := range c.BaseURLs {
		urls[k] = v
	}
	return configView{apiKeys: keys, baseURLs: urls}
}

// defaultModel returns the configured default model or "".
func (c *Config) defaultModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DefaultModel
}

// apiKeyFor returns the resolved API key for a provider, consulting (in order)
// ai.config() overrides, then the conventional env var for that provider.
// Returns "" if unset.
func (c *Config) apiKeyFor(provider string) string {
	c.mu.RLock()
	key := c.APIKeys[provider]
	c.mu.RUnlock()
	if key != "" {
		return key
	}
	return os.Getenv(envVarForProvider(provider))
}

// baseURLFor returns the configured base URL for a provider, or "".
func (c *Config) baseURLFor(provider string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BaseURLs[provider]
}

// envVarForProvider returns the conventional environment variable name for
// a provider's API key. Returns "" for providers without a key (e.g., Ollama).
func envVarForProvider(provider string) string {
	switch provider {
	case "openai":
		return "OPENAI_API_KEY"
	case "anthropic":
		return "ANTHROPIC_API_KEY"
	case "googleai":
		return "GOOGLE_API_KEY"
	case "ollama":
		return "" // local; no key
	}
	return ""
}

// configBuiltin is the `ai.config(...)` handler. Accepts kwargs:
//
//	default_model : string
//	api_keys      : dict[string, string]   (provider -> key)
//	base_urls     : dict[string, string]   (provider -> URL)
//	timeout       : string (duration, parsed with time.ParseDuration)
//
// Idempotent: calling ai.config() again replaces previously set values.
// Returns None.
func (m *Module) configBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("ai.config: takes only keyword arguments, got %d positional", len(args))
	}

	var params struct {
		DefaultModel string                 `name:"default_model"`
		APIKeys      map[string]interface{} `name:"api_keys"`
		BaseURLs     map[string]interface{} `name:"base_urls"`
		Timeout      string                 `name:"timeout"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, fmt.Errorf("ai.config: %w", err)
	}

	keys, err := stringDict(params.APIKeys, "api_keys")
	if err != nil {
		return nil, fmt.Errorf("ai.config: %w", err)
	}
	urls, err := stringDict(params.BaseURLs, "base_urls")
	if err != nil {
		return nil, fmt.Errorf("ai.config: %w", err)
	}

	var timeout time.Duration
	if params.Timeout != "" {
		d, err := time.ParseDuration(params.Timeout)
		if err != nil {
			return nil, fmt.Errorf("ai.config: timeout: %w", err)
		}
		timeout = d
	}

	cfg := m.config
	cfg.mu.Lock()
	cfg.DefaultModel = params.DefaultModel
	cfg.APIKeys = keys
	cfg.BaseURLs = urls
	cfg.Timeout = timeout
	cfg.mu.Unlock()

	// Config change can affect future Genkit plugin init. Reset the client
	// so the next ai.generate() call re-initializes with the new view.
	m.resetClient()

	return starlark.None, nil
}

// stringDict coerces a startype-decoded map[string]interface{} into
// map[string]string, erroring on non-string values.
func stringDict(in map[string]interface{}, name string) (map[string]string, error) {
	if in == nil {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%q] must be a string, got %T", name, k, v)
		}
		out[k] = s
	}
	return out, nil
}
