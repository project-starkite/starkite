package genai

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Chat is a Starlark-visible multi-turn conversation. Each .send() call
// advances the conversation, with history accumulated in memory. Defaults
// captured at ai.chat() time apply to every turn unless overridden per-call.
type Chat struct {
	module *Module

	mu      sync.Mutex
	history []Message

	defaults chatDefaults
}

// chatDefaults captures the resolved kwargs from ai.chat(). toolsSet
// distinguishes "no tools kwarg" from "tools=[] kwarg" (explicitly none).
type chatDefaults struct {
	model         string
	system        string
	tools         []*Tool
	toolsSet      bool
	temperature   *float64
	maxTokens     *int
	topP          *float64
	topK          *int
	stop          []string
	apiKey        string
	baseURL       string
	maxIterations int
	onToolError   string
}

var _ starlark.HasAttrs = (*Chat)(nil)

func (c *Chat) String() string        { return "<ai.Chat>" }
func (c *Chat) Type() string          { return "ai.Chat" }
func (c *Chat) Freeze()               {}
func (c *Chat) Truth() starlark.Bool  { return starlark.True }
func (c *Chat) Hash() (uint32, error) { return 0, fmt.Errorf("ai.Chat is unhashable") }

func (c *Chat) Attr(name string) (starlark.Value, error) {
	switch name {
	case "send":
		return starlark.NewBuiltin("ai.chat.send", c.sendBuiltin), nil
	case "reset":
		return starlark.NewBuiltin("ai.chat.reset", c.resetBuiltin), nil
	case "history":
		return c.historyValue(), nil
	}
	return nil, nil
}

func (c *Chat) AttrNames() []string { return []string{"send", "reset", "history"} }

// historyValue returns a Starlark list-of-dicts snapshot of the conversation
// history. Each dict mirrors the shape ai.chat(history=...) accepts, so users
// can round-trip via: new_chat = ai.chat(..., history=old_chat.history).
//
// Dict keys:
//
//	role        — "user" | "assistant" | "tool"
//	content     — string (may be empty for assistant tool-request messages)
//	tool_name   — present on tool-request (assistant) and tool-response (tool) entries
//	tool_input  — present on tool-request entries; Starlark value converted from Go
//	tool_output — present on tool-response entries
//	tool_error  — present on tool-response entries when the Starlark tool errored
//
// The snapshot is point-in-time; mutating the returned list has no effect on
// the chat's internal state.
func (c *Chat) historyValue() starlark.Value {
	c.mu.Lock()
	defer c.mu.Unlock()

	items := make([]starlark.Value, 0, len(c.history))
	for _, m := range c.history {
		items = append(items, messageToDict(m))
	}
	return starlark.NewList(items)
}

// messageToDict projects an internal Message into the Starlark dict shape
// documented on historyValue.
func messageToDict(m Message) starlark.Value {
	d := starlark.NewDict(6)
	_ = d.SetKey(starlark.String("role"), starlark.String(m.Role))
	if m.Content != "" {
		_ = d.SetKey(starlark.String("content"), starlark.String(m.Content))
	}
	if m.ToolName != "" {
		_ = d.SetKey(starlark.String("tool_name"), starlark.String(m.ToolName))
	}
	if m.ToolInput != nil {
		if v, err := goAnyToStarlark(m.ToolInput); err == nil {
			_ = d.SetKey(starlark.String("tool_input"), v)
		}
	}
	if m.ToolOutput != nil {
		if v, err := goAnyToStarlark(m.ToolOutput); err == nil {
			_ = d.SetKey(starlark.String("tool_output"), v)
		}
	}
	if m.ToolErr != "" {
		_ = d.SetKey(starlark.String("tool_error"), starlark.String(m.ToolErr))
	}
	return d
}

// goAnyToStarlark routes through startype — reuse not wrap (per project preference).
func goAnyToStarlark(v any) (starlark.Value, error) {
	var out starlark.Value
	if err := startype.Go(v).Starlark(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// resetBuiltin implements chat.reset(): clears history, preserving defaults.
func (c *Chat) resetBuiltin(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 || len(kwargs) != 0 {
		return nil, fmt.Errorf("ai.chat.reset: takes no arguments")
	}
	c.mu.Lock()
	c.history = nil
	c.mu.Unlock()
	return starlark.None, nil
}

// ---- ai.chat() builtin -----------------------------------------------------

// chatBuiltin is the `ai.chat(**kwargs) -> Chat` builtin. Kwargs-only.
func (m *Module) chatBuiltin(thread *starlark.Thread, fnBuiltin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("ai.chat: takes only keyword arguments, got %d positional", len(args))
	}

	var p struct {
		Model         string         `name:"model"`
		System        string         `name:"system"`
		Temperature   *float64       `name:"temperature"`
		MaxTokens     *int           `name:"max_tokens"`
		TopP          *float64       `name:"top_p"`
		TopK          *int           `name:"top_k"`
		Stop          []string       `name:"stop"`
		APIKey        string         `name:"api_key"`
		BaseURL       string         `name:"base_url"`
		Tools         starlark.Value `name:"tools"`
		MaxIterations int            `name:"max_iterations"`
		OnToolError   string         `name:"on_tool_error"`
		History       starlark.Value `name:"history"`
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("ai.chat: %w", err)
	}

	// Validate on_tool_error at chat-creation time.
	switch p.OnToolError {
	case "", "feedback":
		p.OnToolError = "feedback"
	case "halt":
		// ok
	default:
		return nil, fmt.Errorf("ai.chat: on_tool_error must be \"feedback\" or \"halt\", got %q", p.OnToolError)
	}

	if p.MaxIterations < 0 {
		return nil, fmt.Errorf("ai.chat: max_iterations must be non-negative, got %d", p.MaxIterations)
	}

	var tools []*Tool
	toolsSet := false
	if p.Tools != nil {
		t, err := CoerceTools(p.Tools)
		if err != nil {
			return nil, fmt.Errorf("ai.chat: %w", err)
		}
		tools = t
		toolsSet = true
	}

	// Optional: seed the chat with prior history. Users feed back what
	// `old_chat.history` returned (or hand-built dicts in the same shape).
	var seeded []Message
	if p.History != nil {
		msgs, err := coerceHistory(p.History)
		if err != nil {
			return nil, fmt.Errorf("ai.chat: history: %w", err)
		}
		seeded = msgs
	}

	return &Chat{
		module:  m,
		history: seeded,
		defaults: chatDefaults{
			model:         p.Model,
			system:        p.System,
			tools:         tools,
			toolsSet:      toolsSet,
			temperature:   p.Temperature,
			maxTokens:     p.MaxTokens,
			topP:          p.TopP,
			topK:          p.TopK,
			stop:          p.Stop,
			apiKey:        p.APIKey,
			baseURL:       p.BaseURL,
			maxIterations: p.MaxIterations,
			onToolError:   p.OnToolError,
		},
	}, nil
}

// coerceHistory converts a Starlark iterable of dicts into the internal
// []Message slice used by Chat. Validates shape — bad entries error at
// construction so users get immediate feedback instead of downstream
// protocol failures.
func coerceHistory(v starlark.Value) ([]Message, error) {
	iter, ok := v.(starlark.Iterable)
	if !ok {
		return nil, fmt.Errorf("expected a list of message dicts, got %s", v.Type())
	}
	it := iter.Iterate()
	defer it.Done()

	var out []Message
	var elem starlark.Value
	idx := 0
	for it.Next(&elem) {
		m, err := coerceHistoryEntry(elem)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", idx, err)
		}
		out = append(out, m)
		idx++
	}
	return out, nil
}

// coerceHistoryEntry validates one dict and returns a Message.
func coerceHistoryEntry(v starlark.Value) (Message, error) {
	d, ok := v.(*starlark.Dict)
	if !ok {
		return Message{}, fmt.Errorf("expected a dict, got %s", v.Type())
	}

	role, err := historyDictString(d, "role", true)
	if err != nil {
		return Message{}, err
	}
	switch role {
	case "user", "assistant", "tool":
		// ok
	default:
		return Message{}, fmt.Errorf("role must be one of 'user', 'assistant', 'tool'; got %q", role)
	}

	content, err := historyDictString(d, "content", false)
	if err != nil {
		return Message{}, err
	}
	toolName, err := historyDictString(d, "tool_name", false)
	if err != nil {
		return Message{}, err
	}
	toolErr, err := historyDictString(d, "tool_error", false)
	if err != nil {
		return Message{}, err
	}

	if role == "tool" && toolName == "" {
		return Message{}, fmt.Errorf("tool messages must include tool_name")
	}

	m := Message{
		Role:     role,
		Content:  content,
		ToolName: toolName,
		ToolErr:  toolErr,
	}

	// tool_input / tool_output are pass-through to the client; we route them
	// through startype back to Go-native shape.
	if in, found, _ := d.Get(starlark.String("tool_input")); found && in != starlark.None {
		gv, err := starlarkValueToGoAny(in)
		if err != nil {
			return Message{}, fmt.Errorf("tool_input: %w", err)
		}
		m.ToolInput = gv
	}
	if out, found, _ := d.Get(starlark.String("tool_output")); found && out != starlark.None {
		gv, err := starlarkValueToGoAny(out)
		if err != nil {
			return Message{}, fmt.Errorf("tool_output: %w", err)
		}
		m.ToolOutput = gv
	}

	return m, nil
}

// historyDictString reads a string-valued key from a dict. Missing + required
// → error; missing + optional → empty. Non-string values always error.
func historyDictString(d *starlark.Dict, key string, required bool) (string, error) {
	v, found, err := d.Get(starlark.String(key))
	if err != nil {
		return "", err
	}
	if !found {
		if required {
			return "", fmt.Errorf("missing required key %q", key)
		}
		return "", nil
	}
	s, ok := starlark.AsString(v)
	if !ok {
		return "", fmt.Errorf("%q must be a string, got %s", key, v.Type())
	}
	return s, nil
}

// starlarkValueToGoAny converts a Starlark value to JSON-friendly Go via
// startype's ToGoValue (same projection used by tool dispatch).
func starlarkValueToGoAny(v starlark.Value) (any, error) {
	return startype.Starlark(v).ToGoValue()
}

// ---- chat.send(...) --------------------------------------------------------

// sendParams is the kwargs shape for chat.send(). Defined as a named type so
// mergeOverrides can accept it cleanly.
type sendParams struct {
	Model         string         `name:"model"`
	System        string         `name:"system"`
	Temperature   *float64       `name:"temperature"`
	MaxTokens     *int           `name:"max_tokens"`
	TopP          *float64       `name:"top_p"`
	TopK          *int           `name:"top_k"`
	Stop          []string       `name:"stop"`
	APIKey        string         `name:"api_key"`
	BaseURL       string         `name:"base_url"`
	Tools         starlark.Value `name:"tools"`
	MaxIterations int            `name:"max_iterations"`
	OnToolError   string         `name:"on_tool_error"`
	Stream        bool           `name:"stream"`
	Schema        starlark.Value `name:"schema"`
}

// sendBuiltin is `chat.send(msg, **overrides)`. Accepts one positional string,
// plus any ai.generate kwargs as per-turn overrides.
func (c *Chat) sendBuiltin(thread *starlark.Thread, fnBuiltin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ai.chat.send: expected 1 positional argument (message), got %d", len(args))
	}
	msg, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("ai.chat.send: message must be a string, got %s", args[0].Type())
	}
	if msg == "" {
		return nil, fmt.Errorf("ai.chat.send: message must be a non-empty string")
	}

	var p sendParams
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("ai.chat.send: %w", err)
	}

	// Detect whether tools= was explicitly passed (even as []).
	toolsOverridden := p.Tools != nil

	// Merge with chat defaults.
	merged, err := c.mergeOverrides(&p, toolsOverridden)
	if err != nil {
		return nil, fmt.Errorf("ai.chat.send: %w", err)
	}

	// Resolve model: merged > config default > error.
	modelName := merged.model
	if modelName == "" {
		modelName = c.module.config.defaultModel()
	}
	if modelName == "" {
		return nil, fmt.Errorf("ai.chat.send: model is required (pass model=..., set it on ai.chat, or call ai.config(default_model=...))")
	}

	if _, _, err := parseModelString(modelName); err != nil {
		return nil, fmt.Errorf("ai.chat.send: %w", err)
	}

	// stream+schema / stream+tools mutual exclusion.
	if merged.stream && merged.schema != nil {
		return nil, fmt.Errorf("ai.chat.send: stream=True and schema= are mutually exclusive in this version")
	}
	if merged.stream && len(merged.tools) > 0 {
		return nil, fmt.Errorf("ai.chat.send: stream=True with tools= is not yet supported (pass tools=[] to disable tools for this turn)")
	}

	if err := libkite.Check(thread, "ai", "generate", modelName); err != nil {
		return nil, err
	}

	// Convert schema dict if present.
	var schemaMap map[string]any
	if merged.schema != nil {
		sm, err := starlarkDictToSchema(merged.schema)
		if err != nil {
			return nil, fmt.Errorf("ai.chat.send: %w", err)
		}
		schemaMap = sm
	}

	// Append user message and snapshot history for this turn.
	c.mu.Lock()
	c.history = append(c.history, Message{Role: "user", Content: msg})
	historySnapshot := append([]Message(nil), c.history...)
	c.mu.Unlock()

	// Build request. ToolTrace is always a fresh slice so callbacks can record.
	trace := make([]ToolInvocation, 0)
	req := GenerateRequest{
		ModelName:   modelName,
		Prompt:      "", // unused when History is set
		System:      merged.system,
		Temperature: merged.temperature,
		MaxTokens:   merged.maxTokens,
		TopP:        merged.topP,
		TopK:        merged.topK,
		Stop:        merged.stop,
		APIKey:      merged.apiKey,
		BaseURL:     merged.baseURL,
		Schema:      schemaMap,
		Tools:       merged.tools,
		MaxTurns:    merged.maxIterations,
		OnToolError: merged.onToolError,
		Thread:      thread,
		History:     historySnapshot,
		ToolTrace:   &trace,
	}

	client, err := c.module.clientFor(req)
	if err != nil {
		c.appendError(err)
		return nil, fmt.Errorf("ai.chat.send: %w", err)
	}

	if merged.stream {
		sr, err := client.GenerateStream(context.Background(), req)
		if err != nil {
			c.appendError(err)
			return nil, fmt.Errorf("ai.chat.send: %w", err)
		}
		return newChatStream(newStreamValue(sr), c), nil
	}

	result, err := client.Generate(context.Background(), req)
	if err != nil {
		c.appendError(err)
		return nil, fmt.Errorf("ai.chat.send: %w", err)
	}

	// Append any captured tool invocations, then final assistant message.
	c.mu.Lock()
	for _, inv := range trace {
		c.history = append(c.history,
			Message{Role: "assistant", ToolName: inv.Name, ToolInput: inv.Input},
			Message{Role: "tool", ToolName: inv.Name, ToolOutput: inv.Output, ToolErr: inv.Err},
		)
	}
	c.history = append(c.history, Message{Role: "assistant", Content: result.Text})
	c.mu.Unlock()

	var dataValue starlark.Value
	if result.Data != nil {
		dataValue, err = goValueToStarlark(result.Data)
		if err != nil {
			return nil, fmt.Errorf("ai.chat.send: %w", err)
		}
	}

	return newResponse(result.Text, result.Model, result.InputTokens, result.OutputTokens, dataValue), nil
}

// appendError records a synthetic assistant message capturing an error, so
// history remains alternating and subsequent turns have context.
func (c *Chat) appendError(err error) {
	c.mu.Lock()
	c.history = append(c.history, Message{
		Role:    "assistant",
		Content: fmt.Sprintf("[error: %s]", err.Error()),
	})
	c.mu.Unlock()
}

// appendAfterStream is called by chatStreamValue after its iterator's Done()
// fires. Merges accumulated text with any stream error into one assistant
// message, preserving alternation.
func (c *Chat) appendAfterStream(accumulated, errMsg string) {
	var content string
	switch {
	case errMsg == "":
		content = accumulated
	case accumulated == "":
		content = fmt.Sprintf("[error: %s]", errMsg)
	default:
		content = fmt.Sprintf("%s [error: %s]", accumulated, errMsg)
	}
	c.mu.Lock()
	c.history = append(c.history, Message{Role: "assistant", Content: content})
	c.mu.Unlock()
}

// ---- merged-kwarg helper ---------------------------------------------------

type mergedCall struct {
	model         string
	system        string
	tools         []*Tool
	temperature   *float64
	maxTokens     *int
	topP          *float64
	topK          *int
	stop          []string
	apiKey        string
	baseURL       string
	maxIterations int
	onToolError   string
	stream        bool
	schema        starlark.Value
}

func (c *Chat) mergeOverrides(p *sendParams, toolsOverridden bool) (*mergedCall, error) {
	d := c.defaults
	m := &mergedCall{
		model:         firstString(p.Model, d.model),
		system:        firstString(p.System, d.system),
		temperature:   firstFloatPtr(p.Temperature, d.temperature),
		maxTokens:     firstIntPtr(p.MaxTokens, d.maxTokens),
		topP:          firstFloatPtr(p.TopP, d.topP),
		topK:          firstIntPtr(p.TopK, d.topK),
		stop:          firstStringSlice(p.Stop, d.stop),
		apiKey:        firstString(p.APIKey, d.apiKey),
		baseURL:       firstString(p.BaseURL, d.baseURL),
		maxIterations: firstInt(p.MaxIterations, d.maxIterations, 10),
		stream:        p.Stream,
		schema:        p.Schema,
	}

	// on_tool_error: override > default (already validated at chat time) > "feedback".
	switch p.OnToolError {
	case "":
		m.onToolError = d.onToolError
		if m.onToolError == "" {
			m.onToolError = "feedback"
		}
	case "feedback", "halt":
		m.onToolError = p.OnToolError
	default:
		return nil, fmt.Errorf("on_tool_error must be \"feedback\" or \"halt\", got %q", p.OnToolError)
	}

	// tools: override wins if explicitly passed (even []); else defaults.
	if toolsOverridden {
		tools, err := CoerceTools(p.Tools)
		if err != nil {
			return nil, err
		}
		m.tools = tools
	} else if d.toolsSet {
		m.tools = d.tools
	}

	return m, nil
}

// firstString returns the first non-empty string.
func firstString(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// firstInt returns the first positive int; final value is a fallback.
func firstInt(vals ...int) int {
	for _, v := range vals[:len(vals)-1] {
		if v > 0 {
			return v
		}
	}
	return vals[len(vals)-1]
}

func firstFloatPtr(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func firstIntPtr(vals ...*int) *int {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func firstStringSlice(vals ...[]string) []string {
	for _, v := range vals {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

// ---- streaming wrapper -----------------------------------------------------

// chatStreamValue wraps a StreamValue to intercept iteration and commit the
// accumulated text to chat history when the iterator completes.
//
// It embeds StreamValue so attribute access (.error, .model, .usage) still
// works uniformly for scripts.
type chatStreamValue struct {
	*StreamValue
	chat *Chat
}

var (
	_ starlark.Iterable = (*chatStreamValue)(nil)
	_ starlark.HasAttrs = (*chatStreamValue)(nil)
)

func newChatStream(inner *StreamValue, chat *Chat) *chatStreamValue {
	return &chatStreamValue{StreamValue: inner, chat: chat}
}

func (csv *chatStreamValue) String() string { return "<ai.ChatStream>" }
func (csv *chatStreamValue) Type() string   { return "ai.ChatStream" }

// Iterate wraps the inner iterator so we can accumulate text and, on Done(),
// commit to history.
func (csv *chatStreamValue) Iterate() starlark.Iterator {
	return &chatStreamIterator{
		inner: csv.StreamValue.Iterate(),
		csv:   csv,
	}
}

type chatStreamIterator struct {
	inner       starlark.Iterator
	csv         *chatStreamValue
	accumulated strings.Builder
	committed   bool
}

func (it *chatStreamIterator) Next(p *starlark.Value) bool {
	if !it.inner.Next(p) {
		return false
	}
	if chunk, ok := (*p).(*StreamChunk); ok {
		it.accumulated.WriteString(chunk.text)
	}
	return true
}

func (it *chatStreamIterator) Done() {
	it.inner.Done()
	if it.committed {
		return
	}
	it.committed = true

	errStr := ""
	if it.csv.StreamValue.result != nil {
		if err := it.csv.StreamValue.result.Err(); err != nil {
			errStr = err.Error()
		}
	}
	it.csv.chat.appendAfterStream(it.accumulated.String(), errStr)
}
