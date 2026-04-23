package genai

import (
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Tool is a Starlark-visible tool definition: a wrapped callable paired with
// a JSON Schema describing its inputs. Construct via ai.tool() or by passing
// a plain Starlark function to ai.generate(tools=[...]), in which case
// inference runs automatically.
//
// Attributes exposed to Starlark:
//
//	.name         — string
//	.description  — string (may be empty)
type Tool struct {
	name        string
	description string
	params      map[string]any
	fn          starlark.Callable
}

var _ starlark.HasAttrs = (*Tool)(nil)

// Exported accessors so other ai/* modules (e.g. mcp) can adapt a Tool to
// their provider's tool representation without taking a dependency on
// genai's internals.
func (t *Tool) Name() string           { return t.name }
func (t *Tool) Description() string    { return t.description }
func (t *Tool) Params() map[string]any { return t.params }
func (t *Tool) Fn() starlark.Callable  { return t.fn }

func (t *Tool) String() string        { return fmt.Sprintf("<ai.Tool name=%q>", t.name) }
func (t *Tool) Type() string          { return "ai.Tool" }
func (t *Tool) Freeze()               {}
func (t *Tool) Truth() starlark.Bool  { return starlark.Bool(t.fn != nil) }
func (t *Tool) Hash() (uint32, error) { return 0, fmt.Errorf("ai.Tool is unhashable") }

func (t *Tool) Attr(name string) (starlark.Value, error) {
	switch name {
	case "name":
		return starlark.String(t.name), nil
	case "description":
		return starlark.String(t.description), nil
	}
	return nil, nil
}

func (t *Tool) AttrNames() []string { return []string{"name", "description"} }

// toolBuiltin is the `ai.tool(fn, description=, params=)` builtin.
//
// Inference resolves independently for each piece:
//   - name: always fn.Name() (no override in v1)
//   - description: explicit kwarg wins, else fn.Doc()
//   - params: explicit kwarg wins, else inferred from signature + defaults
func (m *Module) toolBuiltin(thread *starlark.Thread, fnBuiltin *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("ai.tool: expected 1 positional argument (function), got %d", len(args))
	}
	callable, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("ai.tool: argument must be a function, got %s", args[0].Type())
	}

	var p struct {
		Description string         `name:"description"`
		DescPresent bool           // set manually; distinguishes "" from not-set
		Params      starlark.Value `name:"params"`
	}
	// startype doesn't report presence for string kwargs (empty string is
	// indistinguishable from unset), so we first detect the presence of
	// description= in kwargs, then unpack normally.
	for _, kv := range kwargs {
		if key, ok := kv[0].(starlark.String); ok && string(key) == "description" {
			p.DescPresent = true
		}
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("ai.tool: %w", err)
	}

	// Convert explicit params dict (if any) to a Go map upfront.
	var explicitParams map[string]any
	if p.Params != nil {
		sm, err := starlarkDictToSchema(p.Params)
		if err != nil {
			return nil, fmt.Errorf("ai.tool: params: %w", err)
		}
		explicitParams = sm
	}

	// For non-Starlark-function callables (builtins), introspection is
	// unavailable; the user must supply both description and params.
	starFn, isStarFn := callable.(*starlark.Function)
	if !isStarFn {
		if !p.DescPresent || explicitParams == nil {
			return nil, fmt.Errorf("ai.tool: %s is not a def-function and lacks introspection; "+
				"pass description= and params= explicitly", callable.Type())
		}
		return &Tool{
			name:        callable.Name(),
			description: p.Description,
			params:      explicitParams,
			fn:          callable,
		}, nil
	}

	// Starlark function — run inference and apply overrides on top.
	t, err := inferToolSchema(starFn)
	if err != nil {
		return nil, err
	}
	if p.DescPresent {
		t.description = p.Description
	}
	if explicitParams != nil {
		t.params = explicitParams
	}
	return t, nil
}

// inferTool is the shorthand path used when a plain Starlark function appears
// in ai.generate(tools=[fn]). It calls inferToolSchema directly for functions
// and errors for other callables (which would require explicit ai.tool()).
func inferTool(callable starlark.Callable) (*Tool, error) {
	starFn, ok := callable.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("ai.generate: tools[...]: %s is not a def-function and "+
			"cannot be auto-inferred; wrap it with ai.tool(fn, description=, params=)", callable.Type())
	}
	return inferToolSchema(starFn)
}

// invokeToolCallback is what Genkit calls when the model requests a tool.
// Input arrives as the JSON-deserialized argument map (typically
// map[string]any for object-shaped schemas). The Starlark function is
// invoked on the original ai.generate() thread; its return value is
// converted to a Go-native value for Genkit to re-serialize.
//
// onToolError governs what happens when the Starlark function errors:
//   - "halt":     return (nil, err)  → Genkit surfaces as overall Generate error
//   - "feedback": return ({"error": msg}, nil) → Genkit feeds to model, loop continues
//
// Extracted as a top-level function (not a closure inside buildGenkitTool)
// so tests can exercise the dispatch path directly without going through
// Genkit's iteration protocol.
func invokeToolCallback(t *Tool, input any, thread *starlark.Thread, onToolError string) (any, error) {
	args, kwargs, err := toolInputToStarlark(input)
	if err != nil {
		return toolErrResult(err, onToolError)
	}

	result, err := starlark.Call(thread, t.fn, args, kwargs)
	if err != nil {
		return toolErrResult(err, onToolError)
	}

	return starlarkToGoResult(result)
}

// toolInputToStarlark converts the Genkit-delivered tool input into Starlark
// call arguments. For object-shaped schemas the input is map[string]any, and
// we pass each key/value as a kwarg. Non-object inputs error out.
func toolInputToStarlark(input any) (starlark.Tuple, []starlark.Tuple, error) {
	if input == nil {
		return starlark.Tuple{}, nil, nil
	}
	m, ok := input.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("tool input expected to be an object, got %T", input)
	}
	kwargs := make([]starlark.Tuple, 0, len(m))
	for k, v := range m {
		sv, err := goValueToStarlark(v)
		if err != nil {
			return nil, nil, fmt.Errorf("tool input field %q: %w", k, err)
		}
		kwargs = append(kwargs, starlark.Tuple{starlark.String(k), sv})
	}
	return starlark.Tuple{}, kwargs, nil
}

// starlarkToGoResult converts a Starlark return value to a Go value suitable
// for JSON serialization back to the model. Uses startype's ToGoValue which
// emits JSON-friendly shapes (map[string]any for dicts, []any for lists, etc.).
// Falls back to stringifying for any type it can't convert cleanly.
func starlarkToGoResult(v starlark.Value) (any, error) {
	out, err := startype.Starlark(v).ToGoValue()
	if err != nil {
		return v.String(), nil
	}
	return out, nil
}

// toolErrResult encodes a tool-invocation error per the on_tool_error policy.
func toolErrResult(err error, onToolError string) (any, error) {
	if onToolError == "halt" {
		return nil, err
	}
	return map[string]any{"error": err.Error()}, nil
}
