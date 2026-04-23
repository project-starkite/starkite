package wasm

import (
	"encoding/json"
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// starlarkArgsToJSON converts Starlark positional and keyword arguments to JSON
// according to the function's parameter manifest. Positional args are matched
// by index; kwargs are matched by name.
func starlarkArgsToJSON(params []ParamManifest, args starlark.Tuple, kwargs []starlark.Tuple) ([]byte, error) {
	result := make(map[string]interface{})

	// Track which params have been set
	set := make(map[string]bool)

	// Match positional args by index
	for i, arg := range args {
		if i >= len(params) {
			return nil, fmt.Errorf("too many positional arguments: got %d, expected at most %d", len(args), len(params))
		}
		val, err := startype.Starlark(arg).ToGoValue()
		if err != nil {
			return nil, fmt.Errorf("arg %d (%s): %w", i, params[i].Name, err)
		}
		result[params[i].Name] = val
		set[params[i].Name] = true
	}

	// Match kwargs by name
	for _, kv := range kwargs {
		name := string(kv[0].(starlark.String))
		// Verify kwarg matches a declared param
		found := false
		for _, p := range params {
			if p.Name == name {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unexpected keyword argument %q", name)
		}
		if set[name] {
			return nil, fmt.Errorf("duplicate argument %q", name)
		}
		val, err := startype.Starlark(kv[1]).ToGoValue()
		if err != nil {
			return nil, fmt.Errorf("kwarg %q: %w", name, err)
		}
		result[name] = val
		set[name] = true
	}

	// Check required params
	for _, p := range params {
		if p.IsRequired() && !set[p.Name] {
			return nil, fmt.Errorf("missing required argument %q", p.Name)
		}
	}

	return json.Marshal(result)
}

// jsonToStarlark converts JSON bytes to a Starlark value guided by expectedType.
func jsonToStarlark(data []byte, expectedType string) (starlark.Value, error) {
	if expectedType == "none" || len(data) == 0 {
		return starlark.None, nil
	}

	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		// If unmarshal fails and expected type is string, treat raw bytes as string
		if expectedType == "string" {
			return starlark.String(string(data)), nil
		}
		return nil, fmt.Errorf("json decode: %w", err)
	}

	if expectedType != "" {
		return coerceToType(raw, expectedType)
	}

	return startype.Go[any](raw).ToStarlarkValue()
}

// coerceToType converts a Go value to a Starlark value of the expected type.
func coerceToType(v interface{}, expectedType string) (starlark.Value, error) {
	switch expectedType {
	case "string":
		switch val := v.(type) {
		case string:
			return starlark.String(val), nil
		case nil:
			return starlark.String(""), nil
		default:
			// Marshal non-string values back to JSON string
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("cannot convert %T to string: %w", v, err)
			}
			return starlark.String(string(b)), nil
		}

	case "int":
		switch val := v.(type) {
		case float64:
			return starlark.MakeInt(int(val)), nil
		case json.Number:
			i, err := val.Int64()
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to int: %w", val, err)
			}
			return starlark.MakeInt64(i), nil
		case nil:
			return starlark.MakeInt(0), nil
		default:
			return nil, fmt.Errorf("expected int, got %T", v)
		}

	case "float":
		switch val := v.(type) {
		case float64:
			return starlark.Float(val), nil
		case json.Number:
			f, err := val.Float64()
			if err != nil {
				return nil, fmt.Errorf("cannot convert %q to float: %w", val, err)
			}
			return starlark.Float(f), nil
		case nil:
			return starlark.Float(0), nil
		default:
			return nil, fmt.Errorf("expected float, got %T", v)
		}

	case "bool":
		switch val := v.(type) {
		case bool:
			return starlark.Bool(val), nil
		case nil:
			return starlark.Bool(false), nil
		default:
			return nil, fmt.Errorf("expected bool, got %T", v)
		}

	case "dict":
		switch val := v.(type) {
		case map[string]interface{}:
			return startype.Map(val).ToDict()
		case nil:
			return starlark.NewDict(0), nil
		default:
			return nil, fmt.Errorf("expected dict, got %T", v)
		}

	case "list":
		switch val := v.(type) {
		case []interface{}:
			return startype.Slice(val).ToList()
		case nil:
			return starlark.NewList(nil), nil
		default:
			return nil, fmt.Errorf("expected list, got %T", v)
		}

	case "none":
		return starlark.None, nil

	default:
		return nil, fmt.Errorf("unknown expected type %q", expectedType)
	}
}
