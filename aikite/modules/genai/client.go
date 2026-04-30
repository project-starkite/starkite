package genai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/anthropic"
	oai "github.com/firebase/genkit/go/plugins/compat_oai/openai"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/firebase/genkit/go/plugins/ollama"
	"github.com/openai/openai-go/option"
	"go.starlark.net/starlark"
)

// GenerateRequest is the provider-agnostic request produced by the Starlark
// layer after merging kwargs, Config defaults, and env vars.
//
// Optional float/int fields are *pointers so absence is distinguishable from
// a zero value (zero is a valid temperature).
type GenerateRequest struct {
	ModelName string // full identifier, e.g. "openai/llama3.2"
	Prompt    string
	System    string

	Temperature *float64
	MaxTokens   *int
	TopP        *float64
	TopK        *int
	Stop        []string

	APIKey  string // provider API key override (from kwarg or config)
	BaseURL string // provider endpoint override

	// Schema, when non-nil, is a raw JSON Schema. The client instructs the
	// provider to return JSON matching this schema and populates GenerateResult.Data.
	Schema map[string]any

	// Tools are the Starlark-provided tools available to the model. Empty
	// means no tool calling.
	Tools []*Tool

	// MaxTurns caps tool-call iterations. Zero means use the client default.
	MaxTurns int

	// OnToolError is "feedback" (errors flow back to the model) or "halt"
	// (errors surface as Go errors). Validated at the generate.go layer;
	// empty means "feedback".
	OnToolError string

	// Thread is the Starlark thread the ai.generate() call is running on.
	// Tool callbacks reuse it to invoke user-provided Starlark functions.
	// Safe because tool callbacks run synchronously during genkit.Generate.
	Thread *starlark.Thread

	// History is the conversation so far (including the current user turn as
	// its last entry). When non-empty the client passes ai.WithMessages(...)
	// instead of ai.WithPrompt. Used by ai.chat(); empty for one-shot calls.
	History []Message

	// ToolTrace, when non-nil, is appended to by tool callbacks. Chat uses
	// this to reconstruct tool-request/tool-response messages and add them
	// to history after Generate returns. Nil for one-shot calls.
	ToolTrace *[]ToolInvocation
}

// Message is the provider-neutral conversation message used by the chat layer.
// Role is "user" | "assistant" | "tool".
//
// For assistant messages carrying a tool request, Content is empty and
// ToolName/ToolInput are populated. For tool messages, Content is empty and
// ToolName/ToolOutput (or ToolErr) are populated.
type Message struct {
	Role    string
	Content string

	ToolName   string
	ToolInput  any
	ToolOutput any
	ToolErr    string
}

// ToolInvocation is a single tool call captured by the tool-callback. Chat
// uses the recorded trace to rebuild tool messages in history.
type ToolInvocation struct {
	Name   string
	Input  any
	Output any
	Err    string // non-empty when the Starlark function errored under "feedback"
}

// GenerateResult is the client-layer response shape. The Starlark Response
// wrapper is built from this.
type GenerateResult struct {
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int

	// Data holds the parsed structured output when Schema was set on the
	// request. Go-native shape (map[string]any, []any, primitives). The
	// Starlark layer converts to a Starlark value before exposing as .data.
	Data any
}

// StreamResult carries a live streaming response. Chunks are delivered over
// the Chunks channel. When the channel is closed, the producer has finished
// (either normally or with an error). Err() returns nil on clean completion;
// Usage() is populated after the channel closes.
//
// The channel is always closed by the producer; the consumer must not close it.
type StreamResult struct {
	Chunks <-chan string

	mu           sync.Mutex
	err          error
	model        string
	inputTokens  int
	outputTokens int

	// cancel signals the producer goroutine to stop. The Starlark iterator
	// calls this from Done() to handle early break-out of for loops.
	cancel context.CancelFunc
}

func (s *StreamResult) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Usage returns the final token counts. Only meaningful after the Chunks
// channel has closed.
func (s *StreamResult) Usage() (inputTokens, outputTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inputTokens, s.outputTokens
}

// Model returns the model identifier the stream was generated against.
func (s *StreamResult) Model() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

// Cancel stops the underlying producer goroutine. Safe to call multiple times.
func (s *StreamResult) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// genaiClient is the surface our module uses to talk to a generation backend.
// It exists so tests can inject a fake without standing up Genkit.
type genaiClient interface {
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error)
	GenerateStream(ctx context.Context, req GenerateRequest) (*StreamResult, error)
}

// genkitClient is the production implementation of genaiClient, backed by
// Firebase Genkit. It initializes Genkit (and its plugins) lazily on first
// use so that scripts that never invoke ai.* pay no setup cost.
type genkitClient struct {
	gkOnce sync.Once
	gk     *genkit.Genkit
	gkErr  error

	// activeProviders tracks which provider prefixes have a plugin registered
	// in the Genkit instance. Populated by init; read by checkAvailability.
	activeProviders map[string]bool

	// cfgView captures the subset of Config that affects plugin init
	// (API keys, base URLs). Read once at init time.
	cfgView configView
}

// configView is what the client layer sees from Config.
type configView struct {
	apiKeys  map[string]string
	baseURLs map[string]string
}

func newGenkitClient(cfg configView) *genkitClient {
	return &genkitClient{cfgView: cfg}
}

// resolveKey returns the API key for a provider, preferring an explicit
// ai.config(api_keys=) entry over the environment variable.
func resolveKey(v configView, provider string) string {
	if k := v.apiKeys[provider]; k != "" {
		return k
	}
	return providerAPIKey(provider)
}

// providerAPIKey reads the conventional env var(s) for a provider.
// GoogleAI checks GEMINI_API_KEY first, then GOOGLE_API_KEY.
func providerAPIKey(provider string) string {
	switch provider {
	case providerGoogleAI:
		if v := os.Getenv("GEMINI_API_KEY"); v != "" {
			return v
		}
		return os.Getenv("GOOGLE_API_KEY")
	default:
		name := envVarForProvider(provider)
		if name == "" {
			return ""
		}
		return os.Getenv(name)
	}
}

// init registers Genkit plugins for each Tier 1 provider whose credentials
// are available. Ollama is registered unconditionally (no credentials needed;
// if the server isn't running, per-call requests produce clean errors).
//
// If a provider's env var / config entry is missing, its plugin is skipped
// and requests for that prefix error out at checkAvailability.
func (c *genkitClient) init(ctx context.Context) (*genkit.Genkit, error) {
	c.gkOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				c.gkErr = fmt.Errorf("genkit init panicked: %v", r)
			}
		}()

		active := map[string]bool{}
		var plugins []api.Plugin

		// OpenAI — real OpenAI API. Gated by OPENAI_API_KEY.
		if key := resolveKey(c.cfgView, providerOpenAI); key != "" {
			p := &oai.OpenAI{APIKey: key}
			if url := c.cfgView.baseURLs[providerOpenAI]; url != "" {
				p.Opts = []option.RequestOption{option.WithBaseURL(url)}
			}
			plugins = append(plugins, p)
			active[providerOpenAI] = true
		}

		// Anthropic — native plugin. Gated by ANTHROPIC_API_KEY.
		if key := resolveKey(c.cfgView, providerAnthropic); key != "" {
			plugins = append(plugins, &anthropic.Anthropic{
				APIKey:  key,
				BaseURL: c.cfgView.baseURLs[providerAnthropic],
			})
			active[providerAnthropic] = true
		}

		// Google AI — native plugin. Gated by GEMINI_API_KEY or GOOGLE_API_KEY.
		if key := resolveKey(c.cfgView, providerGoogleAI); key != "" {
			plugins = append(plugins, &googlegenai.GoogleAI{APIKey: key})
			active[providerGoogleAI] = true
		}

		// Ollama — native plugin. Always registered; no credential needed.
		{
			addr := c.cfgView.baseURLs[providerOllama]
			if addr == "" {
				addr = defaultOllamaServerAddr
			}
			plugins = append(plugins, &ollama.Ollama{ServerAddress: addr})
			active[providerOllama] = true
		}

		if len(plugins) == 0 {
			c.gkErr = errors.New("no providers available")
			return
		}

		c.activeProviders = active
		c.gk = genkit.Init(ctx, genkit.WithPlugins(plugins...))
	})
	return c.gk, c.gkErr
}

// checkAvailability validates that the modelName's provider has been wired.
// Called after init(); returns an actionable error otherwise.
func checkAvailability(active map[string]bool, modelName string) error {
	provider, _, err := parseModelString(modelName)
	if err != nil {
		return err
	}
	if active[provider] {
		return nil
	}
	hint := providerKeyHint(provider)
	if hint == "" {
		return fmt.Errorf("%s provider is unavailable", provider)
	}
	return fmt.Errorf("%s provider is unavailable — %s, or pass api_key= per-call", provider, hint)
}

// providerKeyHint returns a human-readable hint about how to configure the
// provider's credentials. Returns "" if no hint is known.
func providerKeyHint(provider string) string {
	switch provider {
	case providerGoogleAI:
		return "set GEMINI_API_KEY or GOOGLE_API_KEY"
	default:
		envVar := envVarForProvider(provider)
		if envVar == "" {
			return ""
		}
		return "set " + envVar
	}
}

// buildGenerateOpts translates a GenerateRequest into the Genkit option slice.
// Shared by Generate and GenerateStream.
func (c *genkitClient) buildGenerateOpts(req GenerateRequest) []ai.GenerateOption {
	opts := []ai.GenerateOption{
		ai.WithModelName(req.ModelName),
	}
	if len(req.History) > 0 {
		msgs := make([]*ai.Message, 0, len(req.History))
		for _, m := range req.History {
			if gm := toGenkitMessage(m); gm != nil {
				msgs = append(msgs, gm)
			}
		}
		opts = append(opts, ai.WithMessages(msgs...))
	} else {
		opts = append(opts, ai.WithPrompt(req.Prompt))
	}
	if req.System != "" {
		opts = append(opts, ai.WithSystem(req.System))
	}
	if provider, _, perr := parseModelString(req.ModelName); perr == nil {
		if cfg := buildProviderConfig(provider, req); cfg != nil {
			opts = append(opts, ai.WithConfig(cfg))
		}
	}
	if req.Schema != nil {
		opts = append(opts, ai.WithOutputSchema(req.Schema))
	}
	if len(req.Tools) > 0 {
		refs := make([]ai.ToolRef, 0, len(req.Tools))
		for _, t := range req.Tools {
			refs = append(refs, buildGenkitTool(t, req.Thread, req.OnToolError, req.ToolTrace))
		}
		opts = append(opts, ai.WithTools(refs...))
	}
	if req.MaxTurns > 0 {
		opts = append(opts, ai.WithMaxTurns(req.MaxTurns))
	}
	return opts
}

// buildGenkitTool wraps a *Tool as a Genkit ToolRef. The callback invokes the
// Starlark function on the captured thread, honors on_tool_error, and — when
// trace is non-nil — records each invocation for the chat layer to rebuild
// history messages.
func buildGenkitTool(t *Tool, thread *starlark.Thread, onToolError string, trace *[]ToolInvocation) ai.ToolRef {
	return ai.NewTool[any, any](
		t.name,
		t.description,
		func(ctx *ai.ToolContext, input any) (any, error) {
			result, err := invokeToolCallback(t, input, thread, onToolError)
			if trace != nil {
				inv := ToolInvocation{Name: t.name, Input: input}
				if err != nil {
					inv.Err = err.Error()
				} else {
					inv.Output = result
				}
				*trace = append(*trace, inv)
			}
			return result, err
		},
		ai.WithInputSchema(t.params),
	)
}

// toGenkitMessage converts a package-internal Message to Genkit's ai.Message.
// Returns nil for messages that shouldn't be emitted (e.g., unrecognized role).
func toGenkitMessage(m Message) *ai.Message {
	switch m.Role {
	case "user":
		return ai.NewUserTextMessage(m.Content)
	case "assistant":
		if m.ToolName != "" {
			// Assistant message carrying a tool request.
			return ai.NewModelMessage(ai.NewToolRequestPart(&ai.ToolRequest{
				Name:  m.ToolName,
				Input: m.ToolInput,
			}))
		}
		return ai.NewModelTextMessage(m.Content)
	case "tool":
		output := m.ToolOutput
		if m.ToolErr != "" {
			output = map[string]any{"error": m.ToolErr}
		}
		return &ai.Message{
			Role: ai.RoleTool,
			Content: []*ai.Part{
				ai.NewToolResponsePart(&ai.ToolResponse{
					Name:   m.ToolName,
					Output: output,
				}),
			},
		}
	}
	return nil
}

func (c *genkitClient) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	g, err := c.init(ctx)
	if err != nil {
		return nil, fmt.Errorf("genkit init failed: %w", err)
	}
	if err := checkAvailability(c.activeProviders, req.ModelName); err != nil {
		return nil, err
	}

	resp, err := genkit.Generate(ctx, g, c.buildGenerateOpts(req)...)
	if err != nil {
		return nil, err
	}

	result := &GenerateResult{
		Text:  resp.Text(),
		Model: req.ModelName,
	}
	if resp.Usage != nil {
		result.InputTokens = resp.Usage.InputTokens
		result.OutputTokens = resp.Usage.OutputTokens
	}
	if req.Schema != nil {
		var parsed any
		if err := resp.Output(&parsed); err != nil {
			return nil, fmt.Errorf("parsing structured output: %w", err)
		}
		result.Data = parsed
	}
	return result, nil
}

// GenerateStream invokes genkit.GenerateStream and returns a StreamResult that
// delivers chunks over a channel. A goroutine consumes Genkit's iterator in
// the background; the returned StreamResult allows the caller to read chunks,
// observe errors, and cancel early.
func (c *genkitClient) GenerateStream(ctx context.Context, req GenerateRequest) (*StreamResult, error) {
	g, err := c.init(ctx)
	if err != nil {
		return nil, fmt.Errorf("genkit init failed: %w", err)
	}
	if err := checkAvailability(c.activeProviders, req.ModelName); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	chunks := make(chan string, 16)
	sr := &StreamResult{
		Chunks: chunks,
		model:  req.ModelName,
		cancel: cancel,
	}

	go func() {
		// Ensure channel closes and errors propagate even on panic paths.
		defer close(chunks)

		for val, err := range genkit.GenerateStream(ctx, g, c.buildGenerateOpts(req)...) {
			if err != nil {
				sr.mu.Lock()
				sr.err = err
				sr.mu.Unlock()
				return
			}
			if val == nil {
				continue
			}
			if val.Done {
				if val.Response != nil && val.Response.Usage != nil {
					sr.mu.Lock()
					sr.inputTokens = val.Response.Usage.InputTokens
					sr.outputTokens = val.Response.Usage.OutputTokens
					sr.mu.Unlock()
				}
				return
			}
			if val.Chunk == nil {
				continue
			}
			text := val.Chunk.Text()
			if text == "" {
				continue
			}
			// Respect context cancellation: if the consumer hung up,
			// bail out instead of blocking forever on a full channel.
			select {
			case chunks <- text:
			case <-ctx.Done():
				return
			}
		}
	}()

	return sr, nil
}
