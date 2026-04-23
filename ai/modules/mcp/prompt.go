package mcp

import (
	"context"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

// promptEntry is the registration record built once at mcp.serve() time.
//
// paramNames is cached so that prompt invocation can map MCP's
// map[string]string arguments into Starlark kwargs in declaration order.
type promptEntry struct {
	name        string
	description string
	arguments   []*mcpsdk.PromptArgument
	fn          starlark.Callable
	paramNames  []string
}

// coercePrompts converts the user's Starlark dict into a slice of registration
// records. Validates shapes; rejects callables using *args/**kwargs so the
// advertised MCP argument schema matches the function signature.
func coercePrompts(v starlark.Value) ([]*promptEntry, error) {
	d, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("prompts must be a dict, got %s", v.Type())
	}
	out := make([]*promptEntry, 0, d.Len())
	for _, item := range d.Items() {
		keyStr, ok := item[0].(starlark.String)
		if !ok {
			return nil, fmt.Errorf("prompt keys must be strings, got %s", item[0].Type())
		}
		name := string(keyStr)
		if name == "" {
			return nil, fmt.Errorf("prompt key must not be empty")
		}
		callable, ok := item[1].(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("prompt %q: value must be callable, got %s", name, item[1].Type())
		}
		e, err := buildPromptEntry(name, callable)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// buildPromptEntry inspects the Starlark function to derive the MCP prompt
// argument schema (name + required). Non-def callables produce a zero-arg
// schema since they lack introspection.
func buildPromptEntry(name string, callable starlark.Callable) (*promptEntry, error) {
	e := &promptEntry{name: name, fn: callable}

	fn, isStarFn := callable.(*starlark.Function)
	if !isStarFn {
		return e, nil
	}
	if fn.HasVarargs() {
		return nil, fmt.Errorf("prompt %q: function must not use *args", name)
	}
	if fn.HasKwargs() {
		return nil, fmt.Errorf("prompt %q: function must not use **kwargs", name)
	}

	e.description = fn.Doc()
	n := fn.NumParams()
	e.arguments = make([]*mcpsdk.PromptArgument, 0, n)
	e.paramNames = make([]string, 0, n)
	for i := 0; i < n; i++ {
		pname, _ := fn.Param(i)
		e.paramNames = append(e.paramNames, pname)
		e.arguments = append(e.arguments, &mcpsdk.PromptArgument{
			Name:     pname,
			Required: fn.ParamDefault(i) == nil,
		})
	}
	return e, nil
}

// buildPromptHandler returns the MCP prompt handler closure.
//
// MCP prompt arguments come in as map[string]string. We forward only the
// declared parameters (by exact name match), letting Starlark's own default
// handling fill in omitted optionals. Undeclared MCP arguments are dropped
// silently. Missing required arguments surface when starlark.Call reports
// an error.
func buildPromptHandler(p *promptEntry, rt *starbase.Runtime) func(context.Context, *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
	return func(ctx context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		thread := rt.NewThread("mcp-prompt-" + p.name)

		if err := starbase.Check(thread, "mcp", "prompt_render", p.name); err != nil {
			return nil, err
		}

		kwargs := make([]starlark.Tuple, 0, len(p.paramNames))
		for _, name := range p.paramNames {
			if v, ok := req.Params.Arguments[name]; ok {
				kwargs = append(kwargs, starlark.Tuple{
					starlark.String(name),
					starlark.String(v),
				})
			}
		}

		result, err := starlark.Call(thread, p.fn, starlark.Tuple{}, kwargs)
		if err != nil {
			return nil, fmt.Errorf("prompt %q: %w", p.name, err)
		}

		text, ok := starlark.AsString(result)
		if !ok {
			return nil, fmt.Errorf("prompt %q: function must return a string, got %s", p.name, result.Type())
		}

		return &mcpsdk.GetPromptResult{
			Description: p.description,
			Messages: []*mcpsdk.PromptMessage{{
				Role:    "user",
				Content: &mcpsdk.TextContent{Text: text},
			}},
		}, nil
	}
}
