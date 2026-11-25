package yaml

import (
	"fmt"
	"os"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	goyaml "gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

// DeepCopy returns a fresh deep copy of a Go value produced from YAML or Starlark.
func DeepCopy(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = DeepCopy(item)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[fmt.Sprint(k)] = DeepCopy(item)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = DeepCopy(item)
		}
		return out
	default:
		return val
	}
}

// MergeMaps recursively merges patch into a deep copy of base using RFC 7396
// semantics (maps merge recursively, nil deletes keys). listMode controls list
// merging: "replace" (default), "append", or "by_key" (using listKey).
func MergeMaps(base, patch map[string]any, listMode, listKey string) map[string]any {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = DeepCopy(v)
	}
	for k, v := range patch {
		if v == nil {
			delete(out, k)
			continue
		}
		if patchMap, ok := toStringKeyMap(v); ok {
			if existingMap, ok := toStringKeyMap(out[k]); ok {
				out[k] = MergeMaps(existingMap, patchMap, listMode, listKey)
				continue
			}
			out[k] = DeepCopy(patchMap)
			continue
		}
		if patchList, ok := v.([]any); ok {
			if existingList, ok := out[k].([]any); ok {
				out[k] = mergeLists(existingList, patchList, listMode, listKey)
				continue
			}
			out[k] = DeepCopy(patchList)
			continue
		}
		out[k] = DeepCopy(v)
	}
	return out
}

func mergeLists(base, patch []any, listMode, listKey string) []any {
	switch listMode {
	case "append":
		out := make([]any, 0, len(base)+len(patch))
		for _, item := range base {
			out = append(out, DeepCopy(item))
		}
		for _, item := range patch {
			out = append(out, DeepCopy(item))
		}
		return out
	case "by_key":
		if listKey == "" {
			return DeepCopy(patch).([]any)
		}
		return mergeListsByKey(base, patch, listKey)
	default:
		return DeepCopy(patch).([]any)
	}
}

func mergeListsByKey(base, patch []any, listKey string) []any {
	patchMaps := make([]map[string]any, 0, len(patch))
	for _, item := range patch {
		m, ok := toStringKeyMap(item)
		if !ok {
			return DeepCopy(patch).([]any)
		}
		if _, hasKey := m[listKey]; !hasKey {
			return DeepCopy(patch).([]any)
		}
		patchMaps = append(patchMaps, m)
	}

	out := make([]any, 0, len(base)+len(patch))
	indexByKey := make(map[string]int, len(base))
	for _, item := range base {
		copied := DeepCopy(item)
		if m, ok := copied.(map[string]any); ok {
			if keyVal, hasKey := m[listKey]; hasKey && keyVal != nil {
				indexByKey[fmt.Sprint(keyVal)] = len(out)
			}
		}
		out = append(out, copied)
	}

	for _, pm := range patchMaps {
		keyStr := fmt.Sprint(pm[listKey])
		if idx, exists := indexByKey[keyStr]; exists {
			if existingMap, ok := out[idx].(map[string]any); ok {
				out[idx] = MergeMaps(existingMap, pm, "by_key", listKey)
				continue
			}
		}
		indexByKey[keyStr] = len(out)
		out = append(out, DeepCopy(pm))
	}
	return out
}

func toStringKeyMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, item := range m {
			out[fmt.Sprint(k)] = item
		}
		return out, true
	default:
		return nil, false
	}
}

// merge deeply merges a base mapping with zero or more patch mappings.
// Usage: yaml.merge(base, *patches, list_mode="replace", list_key="") -> dict
func (m *Module) merge(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("yaml.merge: expected at least 1 positional argument (base), got 0")
	}

	listMode := "replace"
	listKey := ""
	for _, kv := range kwargs {
		k := string(kv[0].(starlark.String))
		switch k {
		case "list_mode":
			s, ok := starlark.AsString(kv[1])
			if !ok {
				return nil, fmt.Errorf("yaml.merge: list_mode must be a string, got %s", kv[1].Type())
			}
			listMode = s
		case "list_key":
			s, ok := starlark.AsString(kv[1])
			if !ok {
				return nil, fmt.Errorf("yaml.merge: list_key must be a string, got %s", kv[1].Type())
			}
			listKey = s
		default:
			return nil, fmt.Errorf("yaml.merge: unexpected keyword argument %q", k)
		}
	}

	switch listMode {
	case "replace", "append":
	case "by_key":
		if listKey == "" {
			return nil, fmt.Errorf("yaml.merge: list_key is required when list_mode=%q", listMode)
		}
	default:
		return nil, fmt.Errorf("yaml.merge: invalid list_mode %q (choose replace, append, or by_key)", listMode)
	}

	acc, err := m.valueToMap(thread, args[0], 0)
	if err != nil {
		return nil, err
	}
	acc = DeepCopy(acc).(map[string]any)

	for i := 1; i < len(args); i++ {
		patch, err := m.valueToMap(thread, args[i], i)
		if err != nil {
			return nil, err
		}
		acc = MergeMaps(acc, patch, listMode, listKey)
	}

	var result starlark.Value
	if err := startype.Go(acc).Starlark(&result); err != nil {
		return nil, fmt.Errorf("yaml.merge: %w", err)
	}
	return result, nil
}

func (m *Module) valueToMap(thread *starlark.Thread, v starlark.Value, idx int) (map[string]any, error) {
	switch val := v.(type) {
	case *starlark.Dict:
		return starlarkDictToMap(val, idx)
	case *starlarkstruct.Struct:
		expanded, err := libkite.ExpandStructs(val)
		if err != nil {
			return nil, fmt.Errorf("yaml.merge: argument %d: %w", idx, err)
		}
		return starlarkDictToMap(expanded.(*starlark.Dict), idx)
	case startype.DictConvertible:
		return starlarkDictToMap(val.ToDict(), idx)
	case starlark.String:
		return decodeYAMLBytesToMap([]byte(string(val)), idx)
	case starlark.Bytes:
		return decodeYAMLBytesToMap([]byte(string(val)), idx)
	case *YamlFile:
		if err := libkite.Check(thread, "fs", "read", "read_file", val.path); err != nil {
			return nil, err
		}
		if val.isDryRun() {
			return map[string]any{}, nil
		}
		raw, err := os.ReadFile(val.path)
		if err != nil {
			return nil, fmt.Errorf("yaml.merge: arg %d (%s): %w", idx, val.path, err)
		}
		return decodeYAMLBytesToMap(raw, idx)
	default:
		return nil, fmt.Errorf("yaml.merge: argument %d must be dict, YAML string, or yaml.file, got %s", idx, v.Type())
	}
}

func starlarkDictToMap(d *starlark.Dict, idx int) (map[string]any, error) {
	out := make(map[string]any, d.Len())
	for _, kv := range d.Items() {
		key, ok := starlark.AsString(kv[0])
		if !ok {
			return nil, fmt.Errorf("yaml.merge: argument %d contains non-string key of type %s", idx, kv[0].Type())
		}
		goVal, err := starlarkValToGo(kv[1])
		if err != nil {
			return nil, fmt.Errorf("yaml.merge: argument %d key %q: %w", idx, key, err)
		}
		out[key] = goVal
	}
	return out, nil
}

func starlarkValToGo(v starlark.Value) (any, error) {
	if v == starlark.None {
		return nil, nil
	}
	switch val := v.(type) {
	case *starlark.Dict:
		out := make(map[string]any, val.Len())
		for _, kv := range val.Items() {
			k, ok := starlark.AsString(kv[0])
			if !ok {
				return nil, fmt.Errorf("non-string dict key of type %s", kv[0].Type())
			}
			item, err := starlarkValToGo(kv[1])
			if err != nil {
				return nil, err
			}
			out[k] = item
		}
		return out, nil
	case startype.DictConvertible:
		return starlarkValToGo(val.ToDict())
	case *starlarkstruct.Struct:
		expanded, err := libkite.ExpandStructs(val)
		if err != nil {
			return nil, err
		}
		return starlarkValToGo(expanded)
	case *starlark.List:
		out := make([]any, val.Len())
		for i := 0; i < val.Len(); i++ {
			item, err := starlarkValToGo(val.Index(i))
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	default:
		var goVal any
		if err := startype.Starlark(v).Go(&goVal); err != nil {
			return nil, err
		}
		return goVal, nil
	}
}

func decodeYAMLBytesToMap(raw []byte, idx int) (map[string]any, error) {
	var decoded any
	if err := goyaml.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("yaml.merge: argument %d invalid YAML: %w", idx, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	res, ok := toStringKeyMap(decoded)
	if !ok {
		return nil, fmt.Errorf("yaml.merge: argument %d YAML must decode to a mapping, got %T", idx, decoded)
	}
	return DeepCopy(res).(map[string]any), nil
}
