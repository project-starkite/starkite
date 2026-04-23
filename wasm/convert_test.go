package wasm

import (
	"encoding/json"
	"testing"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

func TestStarlarkArgsToJSON_Positional(t *testing.T) {
	params := []ParamManifest{
		{Name: "name", Type: "string"},
		{Name: "count", Type: "int"},
	}
	args := starlark.Tuple{starlark.String("hello"), starlark.MakeInt(42)}

	data, err := starlarkArgsToJSON(params, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["name"] != "hello" {
		t.Errorf("name = %v, want %q", result["name"], "hello")
	}
	if result["count"] != float64(42) {
		t.Errorf("count = %v, want %v", result["count"], 42)
	}
}

func TestStarlarkArgsToJSON_Kwargs(t *testing.T) {
	params := []ParamManifest{
		{Name: "name", Type: "string"},
		{Name: "count", Type: "int"},
	}
	kwargs := []starlark.Tuple{
		{starlark.String("count"), starlark.MakeInt(10)},
		{starlark.String("name"), starlark.String("world")},
	}

	data, err := starlarkArgsToJSON(params, nil, kwargs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["name"] != "world" {
		t.Errorf("name = %v, want %q", result["name"], "world")
	}
}

func TestStarlarkArgsToJSON_MissingRequired(t *testing.T) {
	params := []ParamManifest{
		{Name: "name", Type: "string"},
	}

	_, err := starlarkArgsToJSON(params, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing required arg")
	}
}

func TestStarlarkArgsToJSON_OptionalMissing(t *testing.T) {
	f := false
	params := []ParamManifest{
		{Name: "name", Type: "string"},
		{Name: "opt", Type: "string", Required: &f},
	}
	args := starlark.Tuple{starlark.String("hello")}

	data, err := starlarkArgsToJSON(params, args, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]interface{}
	json.Unmarshal(data, &result)

	if result["name"] != "hello" {
		t.Errorf("name = %v, want %q", result["name"], "hello")
	}
	if _, ok := result["opt"]; ok {
		t.Error("optional param should not be in output")
	}
}

func TestStarlarkArgsToJSON_TooManyArgs(t *testing.T) {
	params := []ParamManifest{
		{Name: "name", Type: "string"},
	}
	args := starlark.Tuple{starlark.String("a"), starlark.String("b")}

	_, err := starlarkArgsToJSON(params, args, nil)
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestStarlarkArgsToJSON_DuplicateArg(t *testing.T) {
	params := []ParamManifest{
		{Name: "name", Type: "string"},
	}
	args := starlark.Tuple{starlark.String("pos")}
	kwargs := []starlark.Tuple{
		{starlark.String("name"), starlark.String("kw")},
	}

	_, err := starlarkArgsToJSON(params, args, kwargs)
	if err == nil {
		t.Fatal("expected error for duplicate arg")
	}
}

func TestStarlarkArgsToJSON_UnexpectedKwarg(t *testing.T) {
	params := []ParamManifest{
		{Name: "name", Type: "string"},
	}
	kwargs := []starlark.Tuple{
		{starlark.String("unknown"), starlark.String("val")},
	}

	_, err := starlarkArgsToJSON(params, nil, kwargs)
	if err == nil {
		t.Fatal("expected error for unexpected kwarg")
	}
}

func TestJsonToStarlark_String(t *testing.T) {
	v, err := jsonToStarlark([]byte(`"hello"`), "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := v.(starlark.String); !ok || string(s) != "hello" {
		t.Errorf("got %v, want String(hello)", v)
	}
}

func TestJsonToStarlark_Int(t *testing.T) {
	v, err := jsonToStarlark([]byte(`42`), "int")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i, ok := v.(starlark.Int); !ok {
		t.Errorf("got %T, want Int", v)
	} else if val, ok := i.Int64(); !ok || val != 42 {
		t.Errorf("got %v, want 42", val)
	}
}

func TestJsonToStarlark_Float(t *testing.T) {
	v, err := jsonToStarlark([]byte(`3.14`), "float")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f, ok := v.(starlark.Float); !ok || float64(f) != 3.14 {
		t.Errorf("got %v, want Float(3.14)", v)
	}
}

func TestJsonToStarlark_Bool(t *testing.T) {
	v, err := jsonToStarlark([]byte(`true`), "bool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b, ok := v.(starlark.Bool); !ok || !bool(b) {
		t.Errorf("got %v, want Bool(true)", v)
	}
}

func TestJsonToStarlark_Dict(t *testing.T) {
	v, err := jsonToStarlark([]byte(`{"a": 1, "b": "two"}`), "dict")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	d, ok := v.(*starlark.Dict)
	if !ok {
		t.Fatalf("got %T, want *Dict", v)
	}
	if d.Len() != 2 {
		t.Errorf("dict len = %d, want 2", d.Len())
	}
}

func TestJsonToStarlark_List(t *testing.T) {
	v, err := jsonToStarlark([]byte(`[1, "two", true]`), "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	l, ok := v.(*starlark.List)
	if !ok {
		t.Fatalf("got %T, want *List", v)
	}
	if l.Len() != 3 {
		t.Errorf("list len = %d, want 3", l.Len())
	}
}

func TestJsonToStarlark_None(t *testing.T) {
	v, err := jsonToStarlark(nil, "none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != starlark.None {
		t.Errorf("got %v, want None", v)
	}
}

func TestJsonToStarlark_EmptyData(t *testing.T) {
	v, err := jsonToStarlark([]byte{}, "string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != starlark.None {
		t.Errorf("got %v, want None", v)
	}
}

func TestStarlarkToGo_Roundtrip(t *testing.T) {
	tests := []struct {
		name     string
		input    starlark.Value
		expected interface{}
	}{
		{"none", starlark.None, nil},
		{"bool_true", starlark.Bool(true), true},
		{"bool_false", starlark.Bool(false), false},
		{"int", starlark.MakeInt(42), int64(42)},
		{"float", starlark.Float(3.14), 3.14},
		{"string", starlark.String("hello"), "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := startype.Starlark(tt.input).ToGoValue()
			if err != nil {
				t.Fatalf("ToGoValue error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.expected, tt.expected)
			}
		})
	}
}

func TestStarlarkToGo_List(t *testing.T) {
	list := starlark.NewList([]starlark.Value{
		starlark.String("a"),
		starlark.MakeInt(1),
	})
	got, err := startype.Starlark(list).ToGoValue()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	slice, ok := got.([]interface{})
	if !ok {
		t.Fatalf("got %T, want []interface{}", got)
	}
	if len(slice) != 2 {
		t.Errorf("len = %d, want 2", len(slice))
	}
}

func TestStarlarkToGo_Dict(t *testing.T) {
	dict := starlark.NewDict(2)
	dict.SetKey(starlark.String("key"), starlark.String("value"))

	got, err := startype.Starlark(dict).ToGoValue()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("got %T, want map[string]interface{}", got)
	}
	if m["key"] != "value" {
		t.Errorf("key = %v, want %q", m["key"], "value")
	}
}

func TestGoToStarlark_NestedMap(t *testing.T) {
	input := map[string]interface{}{
		"nested": map[string]interface{}{
			"inner": "value",
		},
	}
	v, err := startype.Map(input).ToDict()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	nested, _, err := v.Get(starlark.String("nested"))
	if err != nil {
		t.Fatalf("get nested: %v", err)
	}
	innerDict, ok := nested.(*starlark.Dict)
	if !ok {
		t.Fatalf("nested: got %T, want *Dict", nested)
	}
	inner, _, err := innerDict.Get(starlark.String("inner"))
	if err != nil {
		t.Fatalf("get inner: %v", err)
	}
	if s, ok := inner.(starlark.String); !ok || string(s) != "value" {
		t.Errorf("inner = %v, want String(value)", inner)
	}
}
