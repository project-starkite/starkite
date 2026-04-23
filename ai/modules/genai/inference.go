package genai

import (
	"fmt"

	"go.starlark.net/starlark"
)

// inferToolSchema derives a JSON Schema description of a Starlark function
// using only built-in introspection (no source-file parsing).
//
// The returned Tool has its fn, name, description (from fn.Doc()), and
// params (object schema with properties + required) populated from the
// function's parameter list and default values.
//
// Returns an error for functions with *args or **kwargs — their shape cannot
// be expressed by a flat JSON object schema. Callers who need such tools
// must provide an explicit params= override via ai.tool().
func inferToolSchema(fn *starlark.Function) (*Tool, error) {
	if fn.HasVarargs() {
		return nil, fmt.Errorf("ai.tool: function %q uses *args; provide an explicit params= schema", fn.Name())
	}
	if fn.HasKwargs() {
		return nil, fmt.Errorf("ai.tool: function %q uses **kwargs; provide an explicit params= schema", fn.Name())
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
// When the default is nil (parameter is required) or None, we fall back to
// {"type": "string"} — the safest assumption for a required param.
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
		// None as a default — type is ambiguous; fall back to string.
		schema["type"] = "string"
	}
	return schema
}
