package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/aikite/modules/genai"
	"github.com/project-starkite/starkite/starbase"
)

// buildToolHandler returns the callback MCP invokes when a connected client
// calls a registered tool. The handler routes the request to the backing
// Starlark function on a fresh thread (preserving permissions via
// starbase.Runtime.NewThread), converts input/output using startype, and
// translates Starlark errors into MCP error responses so the server stays
// alive.
func buildToolHandler(t *genai.Tool, rt *starbase.Runtime) func(context.Context, *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		toolThread := rt.NewThread("mcp-tool-" + t.Name())

		if err := starbase.Check(toolThread, "mcp", "tool_invoke", t.Name()); err != nil {
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
