package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
)

// FromFile loads a Fleet from a YAML or JSON file.
func FromFile(path string) (*Fleet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fleet.file: failed to read file %q: %w", path, err)
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return New(nil), nil
	}

	// If path or content looks like JSON, attempt JSON parse first
	if strings.HasSuffix(path, ".json") || strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		if fleet, err := FromJSON(data); err == nil {
			return fleet, nil
		}
	}

	// Default to YAML parse (which is a superset of JSON)
	return FromYAML(data)
}

// FromSource parses a Fleet from a Starlark runtime value (list, callable, string JSON, or dict).
func FromSource(thread *starlark.Thread, source starlark.Value) (*Fleet, error) {
	if source == nil || source == starlark.None {
		return New(nil), nil
	}

	switch v := source.(type) {
	case *Fleet:
		return v, nil

	case *starlark.List:
		return fromStarlarkSequence(v)

	case starlark.Tuple:
		return fromStarlarkSequence(v)

	case starlark.Callable:
		res, err := starlark.Call(thread, v, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("fleet.from: discovery function failed: %w", err)
		}
		return FromSource(thread, res)

	case starlark.String:
		data := []byte(string(v))
		trimmed := strings.TrimSpace(string(v))
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return FromJSON(data)
		}
		return FromYAML(data)

	case *starlark.Dict:
		var items []Resource
		for _, item := range v.Items() {
			groupKey, ok := starlark.AsString(item[0])
			if !ok {
				continue
			}
			switch val := item[1].(type) {
			case *starlark.List:
				for i := 0; i < val.Len(); i++ {
					r, err := NormalizeStarlarkValue(val.Index(i))
					if err == nil {
						if r.Labels == nil {
							r.Labels = make(map[string]string)
						}
						r.Labels["_group"] = groupKey
						if r.Data == nil {
							r.Data = make(map[string]any)
						}
						r.Data["_group"] = groupKey
						items = append(items, r)
					}
				}
			default:
				r, err := NormalizeStarlarkValue(item[1])
				if err == nil {
					items = append(items, r)
				}
			}
		}
		return New(items), nil

	default:
		return nil, fmt.Errorf("fleet.from: unsupported source type %s (expected list, callable, dict, or JSON string)", source.Type())
	}
}

// FromJSON parses a Fleet from JSON bytes.
func FromJSON(data []byte) (*Fleet, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("fleet.from: invalid JSON payload: %w", err)
	}
	return parseRawData(raw)
}

// FromYAML parses a Fleet from YAML bytes.
func FromYAML(data []byte) (*Fleet, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("fleet.file: invalid YAML payload: %w", err)
	}
	return parseRawData(raw)
}

func parseRawData(raw any) (*Fleet, error) {
	if raw == nil {
		return New(nil), nil
	}

	var resources []Resource

	switch v := raw.(type) {
	case []any:
		for i, item := range v {
			r, err := NormalizeResource(item)
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", i, err)
			}
			resources = append(resources, r)
		}
		return New(resources), nil

	case map[string]any:
		// Check for wrapper keys like "hosts", "nodes", "servers", "resources"
		for _, wrapperKey := range []string{"hosts", "nodes", "servers", "resources", "items"} {
			if subList, ok := v[wrapperKey].([]any); ok {
				for i, item := range subList {
					r, err := NormalizeResource(item)
					if err != nil {
						return nil, fmt.Errorf("item %d in %q: %w", i, wrapperKey, err)
					}
					resources = append(resources, r)
				}
				return New(resources), nil
			}
		}

		// Otherwise check for grouped map: groupName -> []items
		for groupName, groupVal := range v {
			if subList, ok := groupVal.([]any); ok {
				for _, item := range subList {
					r, err := NormalizeResource(item)
					if err == nil {
						if r.Labels == nil {
							r.Labels = make(map[string]string)
						}
						r.Labels["_group"] = groupName
						if r.Data == nil {
							r.Data = make(map[string]any)
						}
						r.Data["_group"] = groupName
						resources = append(resources, r)
					}
				}
			}
		}
		return New(resources), nil

	default:
		return nil, fmt.Errorf("expected list or map of compute resources, got %T", raw)
	}
}

func fromStarlarkSequence(seq starlark.Sequence) (*Fleet, error) {
	resources := make([]Resource, 0, seq.Len())
	iter := seq.Iterate()
	defer iter.Done()

	var val starlark.Value
	idx := 0
	for iter.Next(&val) {
		r, err := NormalizeStarlarkValue(val)
		if err != nil {
			return nil, fmt.Errorf("item at index %d: %w", idx, err)
		}
		resources = append(resources, r)
		idx++
	}
	return New(resources), nil
}
