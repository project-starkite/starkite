package genai

import (
	"context"
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// generate is the `ai.generate(prompt, **kwargs)` builtin.
//
// Kwargs recognized in Slice 1.2:
//
//	model        : string         (required unless ai.config(default_model=...) is set)
//	system       : string         (system prompt)
//	temperature  : float
//	max_tokens   : int
//	top_p        : float
//	top_k        : int            (currently ignored for OpenAI-compat path)
//	stop         : list[string]
//	api_key      : string         (override env/config for this call)
//	base_url     : string         (override endpoint for this call)
//	stream       : bool           (when True, returns a *StreamValue)
//	schema       : dict           (JSON Schema; when set, response.data is parsed)
//
// Rules:
//   - stream=True and schema= are mutually exclusive in this slice.
//
// Return:
//   - stream=True → *StreamValue (iterable of *StreamChunk)
//   - otherwise   → *Response (.text, .model, .usage, .data)
func (m *Module) generate(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ai.generate: expected 1 positional argument (prompt), got %d", len(args))
	}
	prompt, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("ai.generate: prompt must be a string, got %s", args[0].Type())
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
		Stream        bool           `name:"stream"`
		Schema        starlark.Value `name:"schema"`
		Tools         starlark.Value `name:"tools"`
		MaxIterations int            `name:"max_iterations"`
		OnToolError   string         `name:"on_tool_error"`
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("ai.generate: %w", err)
	}

	// stream + schema are mutually exclusive for now (streaming structured
	// output is called out in the proposal's Future Production Concerns).
	if p.Stream && p.Schema != nil {
		return nil, fmt.Errorf("ai.generate: stream=True and schema= are mutually exclusive in this version")
	}

	// Tools with streaming: also deferred. The LLM and tool call loop
	// doesn't play cleanly with our current streaming iterator.
	if p.Stream && p.Tools != nil {
		return nil, fmt.Errorf("ai.generate: stream=True with tools= is not yet supported")
	}

	// Convert schema dict if present.
	var schemaMap map[string]any
	if p.Schema != nil {
		m, err := starlarkDictToSchema(p.Schema)
		if err != nil {
			return nil, fmt.Errorf("ai.generate: %w", err)
		}
		schemaMap = m
	}

	// Convert tools list (if any) to []*Tool.
	var tools []*Tool
	if p.Tools != nil {
		t, err := CoerceTools(p.Tools)
		if err != nil {
			return nil, fmt.Errorf("ai.generate: %w", err)
		}
		tools = t
	}

	// Validate on_tool_error (applies only when tools are present but
	// we validate eagerly so users see errors even without a network call).
	switch p.OnToolError {
	case "", "feedback":
		p.OnToolError = "feedback"
	case "halt":
		// ok
	default:
		return nil, fmt.Errorf("ai.generate: on_tool_error must be \"feedback\" or \"halt\", got %q", p.OnToolError)
	}

	// Default max iterations.
	maxIterations := p.MaxIterations
	if maxIterations == 0 {
		maxIterations = 10
	} else if maxIterations < 0 {
		return nil, fmt.Errorf("ai.generate: max_iterations must be positive, got %d", maxIterations)
	}

	// Resolve model: per-call kwarg > ai.config() default.
	modelName := p.Model
	if modelName == "" {
		modelName = m.config.defaultModel()
	}
	if modelName == "" {
		return nil, fmt.Errorf("ai.generate: model is required (pass model=..., or set ai.config(default_model=...))")
	}

	// Validate provider prefix.
	if _, _, err := parseModelString(modelName); err != nil {
		return nil, fmt.Errorf("ai.generate: %w", err)
	}

	if err := libkite.Check(thread, "ai", "generate", "generate", modelName); err != nil {
		return nil, err
	}

	req := GenerateRequest{
		ModelName:   modelName,
		Prompt:      prompt,
		System:      p.System,
		Temperature: p.Temperature,
		MaxTokens:   p.MaxTokens,
		TopP:        p.TopP,
		TopK:        p.TopK,
		Stop:        p.Stop,
		APIKey:      p.APIKey,
		BaseURL:     p.BaseURL,
		Schema:      schemaMap,
		Tools:       tools,
		MaxTurns:    maxIterations,
		OnToolError: p.OnToolError,
		Thread:      thread,
	}

	client, err := m.clientFor(req)
	if err != nil {
		return nil, fmt.Errorf("ai.generate: %w", err)
	}

	if p.Stream {
		sr, err := client.GenerateStream(context.Background(), req)
		if err != nil {
			return nil, fmt.Errorf("ai.generate: %w", err)
		}
		return newStreamValue(sr), nil
	}

	result, err := client.Generate(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("ai.generate: %w", err)
	}

	var dataValue starlark.Value
	if result.Data != nil {
		dataValue, err = goValueToStarlark(result.Data)
		if err != nil {
			return nil, fmt.Errorf("ai.generate: %w", err)
		}
	}

	return newResponse(result.Text, result.Model, result.InputTokens, result.OutputTokens, dataValue), nil
}

// CoerceTools accepts a Starlark value that should be a list and converts
// each element into a *Tool. Elements may be either an ai.Tool (use as-is)
// or a Starlark callable (auto-infer via ai.tool(fn)). Other element types
// produce a clear error.
func CoerceTools(v starlark.Value) ([]*Tool, error) {
	iter, ok := v.(starlark.Iterable)
	if !ok {
		return nil, fmt.Errorf("tools must be a list, got %s", v.Type())
	}
	it := iter.Iterate()
	defer it.Done()

	var tools []*Tool
	var elem starlark.Value
	idx := 0
	for it.Next(&elem) {
		switch e := elem.(type) {
		case *Tool:
			tools = append(tools, e)
		case starlark.Callable:
			t, err := inferTool(e)
			if err != nil {
				return nil, fmt.Errorf("tools[%d]: %w", idx, err)
			}
			tools = append(tools, t)
		default:
			return nil, fmt.Errorf("tools[%d]: expected function or ai.Tool, got %s", idx, elem.Type())
		}
		idx++
	}
	return tools, nil
}
