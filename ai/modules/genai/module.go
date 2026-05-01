// Package genai provides LLM generation for starkite via Firebase Genkit.
//
// Slice 1.1 surface:
//
//	ai.config(default_model=..., api_keys=..., base_urls=..., timeout=...)
//	ai.generate(prompt, model=..., system=..., temperature=..., ...)
//
// Only the OpenAI-compat plugin (pointed at Ollama) is wired. Slice 1.5 adds
// Anthropic, OpenAI proper, and Google AI plugins.
package genai

import (
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// ModuleName is the Starlark namespace for this module: `ai`.
const ModuleName libkite.ModuleName = "ai"

// Module implements the libkite Module interface.
//
// Lifetime: one instance per Registry. Thread-safety is required because
// libkite threads share the module value across script invocations.
type Module struct {
	loadOnce sync.Once
	module   starlark.Value
	mcfg     *libkite.ModuleConfig

	// config holds user-set defaults from ai.config(...).
	config *Config

	// clientMu guards cachedClient. It's regenerated whenever ai.config()
	// changes the provider view (API keys, base URLs).
	clientMu      sync.Mutex
	cachedClient  genaiClient
	newClientFunc func(cv configView) genaiClient // seam for tests
}

func New() *Module {
	return &Module{
		config:        newConfig(),
		newClientFunc: func(cv configView) genaiClient { return newGenkitClient(cv) },
	}
}

func (m *Module) Name() libkite.ModuleName    { return ModuleName }
func (m *Module) Description() string          { return "ai provides LLM generation via Genkit" }
func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

func (m *Module) Load(mcfg *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.loadOnce.Do(func() {
		m.mcfg = mcfg
		m.module = libkite.NewTryModule(string(ModuleName), starlark.StringDict{
			"config":    starlark.NewBuiltin("ai.config", m.configBuiltin),
			"generate":  starlark.NewBuiltin("ai.generate", m.generate),
			"tool":      starlark.NewBuiltin("ai.tool", m.toolBuiltin),
			"chat":      starlark.NewBuiltin("ai.chat", m.chatBuiltin),
			"run_until": starlark.NewBuiltin("ai.run_until", m.runUntilBuiltin),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

// clientFor returns the genaiClient to use for a request. If the request
// carries a per-call base_url or api_key override, a one-shot client is
// constructed (not cached). Otherwise the module-level cached client is
// returned, initialized on first use from the current Config snapshot.
func (m *Module) clientFor(req GenerateRequest) (genaiClient, error) {
	if req.BaseURL != "" || req.APIKey != "" {
		return m.newClientFunc(m.overlayView(req)), nil
	}

	m.clientMu.Lock()
	defer m.clientMu.Unlock()
	if m.cachedClient == nil {
		m.cachedClient = m.newClientFunc(m.config.snapshot())
	}
	return m.cachedClient, nil
}

// overlayView applies per-call api_key / base_url overrides on top of the
// module-level config. The override is keyed to the request's provider.
func (m *Module) overlayView(req GenerateRequest) configView {
	base := m.config.snapshot()
	provider, _, err := parseModelString(req.ModelName)
	if err != nil {
		// Should not happen — generate() already validated.
		return base
	}
	if req.APIKey != "" {
		base.apiKeys[provider] = req.APIKey
	}
	if req.BaseURL != "" {
		base.baseURLs[provider] = req.BaseURL
	}
	return base
}

// resetClient drops the cached client. Called after ai.config() mutates state
// so the next ai.generate() call rebuilds with the fresh view.
func (m *Module) resetClient() {
	m.clientMu.Lock()
	m.cachedClient = nil
	m.clientMu.Unlock()
}
