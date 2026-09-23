package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Tool is a Starlark-visible tool definition pairing a callable function with
// a JSON Schema parameter specification.
type Tool struct {
	name        string
	description string
	params      map[string]any
	fn          starlark.Callable
}

var _ starlark.Value = (*Tool)(nil)

func (t *Tool) Name() string           { return t.name }
func (t *Tool) Description() string    { return t.description }
func (t *Tool) Params() map[string]any { return t.params }
func (t *Tool) Fn() starlark.Callable  { return t.fn }

func (t *Tool) String() string        { return fmt.Sprintf("<mcp.Tool name=%q>", t.name) }
func (t *Tool) Type() string          { return "mcp.Tool" }
func (t *Tool) Freeze()               {}
func (t *Tool) Truth() starlark.Bool  { return starlark.Bool(t.fn != nil) }
func (t *Tool) Hash() (uint32, error) { return 0, fmt.Errorf("mcp.Tool is unhashable") }

// CoerceTools accepts a Starlark value (list or iterable) and converts
// each element into a *Tool. Elements may be an existing *Tool or a Starlark
// callable (auto-inferred via inferTool).
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
			return nil, fmt.Errorf("tools[%d]: expected function or mcp.Tool, got %s", idx, elem.Type())
		}
		idx++
	}
	return tools, nil
}

// inferTool handles shorthand inference for plain Starlark functions.
func inferTool(callable starlark.Callable) (*Tool, error) {
	starFn, ok := callable.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("tools[...]: %s is not a def-function and cannot be auto-inferred", callable.Type())
	}
	return inferToolSchema(starFn)
}

// inferToolSchema derives a JSON Schema description of a Starlark function
// using built-in introspection (parameter list and default values).
func inferToolSchema(fn *starlark.Function) (*Tool, error) {
	if fn.HasVarargs() {
		return nil, fmt.Errorf("mcp tool: function %q uses *args; cannot auto-infer schema", fn.Name())
	}
	if fn.HasKwargs() {
		return nil, fmt.Errorf("mcp tool: function %q uses **kwargs; cannot auto-infer schema", fn.Name())
	}

	props := map[string]any{}
	var required []string
	for i := 0; i < fn.NumParams(); i++ {
		name, _ := fn.Param(i)
		def := fn.ParamDefault(i)
		props[name] = inferParamSchema(def)
		if def == nil {
			required = append(required, name)
		}
	}

	t := &Tool{
		name:        fn.Name(),
		description: fn.Doc(),
		fn:          fn,
		params: map[string]any{
			"type":       "object",
			"properties": props,
		},
	}
	if len(required) > 0 {
		t.params["required"] = required
	}
	return t, nil
}

// inferParamSchema maps a Starlark default value to a JSON Schema fragment.
func inferParamSchema(def starlark.Value) map[string]any {
	schema := map[string]any{"type": "string"}
	if def == nil {
		return schema
	}
	switch v := def.(type) {
	case starlark.String:
		schema["type"] = "string"
		if s := string(v); s != "" {
			schema["default"] = s
		}
	case starlark.Bool:
		schema["type"] = "boolean"
		schema["default"] = bool(v)
	case starlark.Int:
		schema["type"] = "integer"
		if i, ok := v.Int64(); ok {
			schema["default"] = i
		}
	case starlark.Float:
		schema["type"] = "number"
		schema["default"] = float64(v)
	case *starlark.List:
		schema["type"] = "array"
	case *starlark.Dict:
		schema["type"] = "object"
	case starlark.NoneType:
		schema["type"] = "string"
	}
	return schema
}

// buildToolHandler returns the callback MCP invokes when a connected client
// calls a registered tool. The handler routes the request to the backing
// Starlark function on a fresh thread (preserving permissions via
// libkite.Runtime.NewThread), converts input/output using startype, and
// translates Starlark errors into MCP error responses so the server stays
// alive.
func buildToolHandler(t *Tool, rt *libkite.Runtime) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		toolThread := rt.NewThread("mcp-tool-" + t.Name())

		if err := libkite.Check(toolThread, "mcp", "client", "tool_invoke", t.Name()); err != nil {
			return errorResult(err), nil
		}

		// req.Params.Arguments is the raw JSON payload received over the wire.
		// Unmarshal into map[string]any before converting each value to Starlark.
		args, err := unmarshalArgs(req.Params.Arguments)
		if err != nil {
			return errorResult(err), nil
		}
		kwargs := make([]starlark.Tuple, 0, len(args))
		for k, v := range args {
			var sv starlark.Value
			if err := startype.Go(v).Starlark(&sv); err != nil {
				return errorResult(fmt.Errorf("arg %q: %w", k, err)), nil
			}
			kwargs = append(kwargs, starlark.Tuple{starlark.String(k), sv})
		}

		result, err := starlark.Call(toolThread, t.Fn(), starlark.Tuple{}, kwargs)
		if err != nil {
			return errorResult(err), nil
		}

		// Project the return value into a JSON-friendly Go shape.
		out, convErr := startype.Starlark(result).ToGoValue()
		if convErr != nil {
			out = result.String()
		}
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: serializeText(out)}},
		}, nil
	}
}

// unmarshalArgs decodes MCP's raw-JSON tool arguments into a map[string]any.
// An empty/nil payload is treated as "no arguments" (empty map).
func unmarshalArgs(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("tool arguments: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// serializeText returns v as a text representation suitable for a TextContent.
// Strings pass through unchanged. Other values are JSON-encoded (falling back
// to fmt.Sprintf if marshaling fails — extremely rare for ToGoValue output).
func serializeText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// errorResult wraps an error as an MCP tool-error result. The MCP client (and
// any LLM on the other side) sees IsError=true and the error text; the server
// continues serving.
func errorResult(err error) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
	}
}
