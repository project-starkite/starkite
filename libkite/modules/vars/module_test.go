package vars

import (
	"sort"
	"testing"

	"go.starlark.net/starlark"
)

// mockVarStore implements libkite.VarStore for testing.
type mockVarStore struct {
	data map[string]interface{}
}

func (m *mockVarStore) Get(key string) (interface{}, bool) {
	v, ok := m.data[key]
	return v, ok
}

func (m *mockVarStore) GetWithDefault(key string, def interface{}) interface{} {
	if v, ok := m.data[key]; ok {
		return v
	}
	return def
}

func (m *mockVarStore) GetString(key string) string {
	if v, ok := m.data[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func (m *mockVarStore) Keys() []string {
	keys := make([]string, 0, len(m.data))
	for k := range m.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestToStarlarkList_Slice(t *testing.T) {
	val, err := toStarlarkList([]interface{}{1.0, 2.0, 3.0}, "ports")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := val.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", val)
	}
	if list.Len() != 3 {
		t.Fatalf("expected 3 elements, got %d", list.Len())
	}
}

func TestToStarlarkList_JSONString(t *testing.T) {
	val, err := toStarlarkList(`[1, 2, 3]`, "ports")
	if err != nil {
		t.Fatal(err)
	}
	list, ok := val.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", val)
	}
	if list.Len() != 3 {
		t.Fatalf("expected 3 elements, got %d", list.Len())
	}
}

func TestToStarlarkList_InvalidString(t *testing.T) {
	_, err := toStarlarkList("not-a-list", "ports")
	if err == nil {
		t.Fatal("expected error for invalid string")
	}
}

func TestToStarlarkList_StringNotList(t *testing.T) {
	_, err := toStarlarkList(`{"a": 1}`, "ports")
	if err == nil {
		t.Fatal("expected error for non-list JSON")
	}
}

func TestToStarlarkList_UnsupportedType(t *testing.T) {
	_, err := toStarlarkList(42, "ports")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestToStarlarkDict_Map(t *testing.T) {
	val, err := toStarlarkDict(map[string]interface{}{"app": "web", "env": "prod"}, "labels")
	if err != nil {
		t.Fatal(err)
	}
	dict, ok := val.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", val)
	}
	if dict.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", dict.Len())
	}
	v, found, err := dict.Get(starlark.String("app"))
	if err != nil || !found {
		t.Fatal("key 'app' not found")
	}
	if v.(starlark.String) != "web" {
		t.Fatalf("expected 'web', got %v", v)
	}
}

func TestToStarlarkDict_JSONString(t *testing.T) {
	val, err := toStarlarkDict(`{"cpu": "500m", "memory": "256Mi"}`, "limits")
	if err != nil {
		t.Fatal(err)
	}
	dict, ok := val.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", val)
	}
	if dict.Len() != 2 {
		t.Fatalf("expected 2 entries, got %d", dict.Len())
	}
}

func TestToStarlarkDict_InvalidString(t *testing.T) {
	_, err := toStarlarkDict("not-a-dict", "labels")
	if err == nil {
		t.Fatal("expected error for invalid string")
	}
}

func TestToStarlarkDict_StringNotDict(t *testing.T) {
	_, err := toStarlarkDict(`[1, 2]`, "labels")
	if err == nil {
		t.Fatal("expected error for non-dict JSON")
	}
}

func TestToStarlarkDict_UnsupportedType(t *testing.T) {
	_, err := toStarlarkDict(42, "labels")
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestToStarlarkDict_Nested(t *testing.T) {
	val, err := toStarlarkDict(map[string]interface{}{
		"resources": map[string]interface{}{
			"cpu":    "500m",
			"memory": "256Mi",
		},
	}, "config")
	if err != nil {
		t.Fatal(err)
	}
	dict := val.(*starlark.Dict)
	v, found, err := dict.Get(starlark.String("resources"))
	if err != nil || !found {
		t.Fatal("key 'resources' not found")
	}
	inner, ok := v.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected nested dict, got %T", v)
	}
	if inner.Len() != 2 {
		t.Fatalf("expected 2 nested entries, got %d", inner.Len())
	}
}

func TestGoSliceToStarlarkList_StringElements(t *testing.T) {
	list, err := goSliceToStarlarkList([]interface{}{"web", "prod", "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Len() != 3 {
		t.Fatalf("expected 3 elements, got %d", list.Len())
	}
	if list.Index(0).(starlark.String) != "web" {
		t.Fatalf("expected 'web', got %v", list.Index(0))
	}
}

func TestGoSliceToStarlarkList_Empty(t *testing.T) {
	list, err := goSliceToStarlarkList([]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if list.Len() != 0 {
		t.Fatalf("expected 0 elements, got %d", list.Len())
	}
}

func TestGoMapToStarlarkDict_Empty(t *testing.T) {
	dict, err := goMapToStarlarkDict(map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if dict.Len() != 0 {
		t.Fatalf("expected 0 entries, got %d", dict.Len())
	}
}
