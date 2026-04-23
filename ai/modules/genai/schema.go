package genai

import (
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// starlarkDictToSchema converts a Starlark dict representing a JSON Schema
// into a Go map[string]any suitable for ai.WithOutputSchema. Returns an error
// if the argument is not a dict or has non-string keys.
func starlarkDictToSchema(v starlark.Value) (map[string]any, error) {
	d, ok := v.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("schema must be a dict, got %s", v.Type())
	}
	m, err := startype.Dict(d).ToMap()
	if err != nil {
		return nil, fmt.Errorf("converting schema dict: %w", err)
	}
	return m, nil
}

// goValueToStarlark converts the Go-native result of a structured-output
// parse (as produced by ModelResponse.Output) into a Starlark value.
func goValueToStarlark(v any) (starlark.Value, error) {
	if v == nil {
		return starlark.None, nil
	}
	var out starlark.Value
	if err := startype.Go(v).Starlark(&out); err != nil {
		return nil, fmt.Errorf("converting parsed output: %w", err)
	}
	return out, nil
}
