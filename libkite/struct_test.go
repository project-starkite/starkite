package libkite

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func execStructScript(t *testing.T, code string, extraPredeclared starlark.StringDict) (starlark.StringDict, error) {
	t.Helper()
	rt, err := NewSandboxed(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	predecl := rt.buildPredeclared()
	maps.Copy(predecl, extraPredeclared)

	thread := rt.NewThread("test-struct")
	return starlark.ExecFile(thread, "test.star", code, predecl)
}

func TestStructCreationAndAccess(t *testing.T) {
	code := `
empty = struct()
is_empty_true = bool(empty)
type_name = type(empty)
simple = struct(b = 2, a = 1)
text = str(simple)
nested = struct(db = struct(host = "db.internal", port = 5432), tls = True)
from_kwargs = struct(**{"region": "us-central1", "replicas": 3})
host = nested.db.host
port = nested.db.port
region = from_kwargs.region
`
	out, err := execStructScript(t, code, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["is_empty_true"] != starlark.True {
		t.Fatalf("bool(struct()) = %v, want True", out["is_empty_true"])
	}
	if got := string(out["type_name"].(starlark.String)); got != "struct" {
		t.Fatalf("type(struct()) = %q, want %q", got, "struct")
	}
	if got := string(out["text"].(starlark.String)); got != "struct(a = 1, b = 2)" {
		t.Fatalf("str(simple) = %q, want %q", got, "struct(a = 1, b = 2)")
	}
	if got := string(out["host"].(starlark.String)); got != "db.internal" {
		t.Fatalf("nested.db.host = %q", got)
	}
	if got := out["port"].(starlark.Int).String(); got != "5432" {
		t.Fatalf("nested.db.port = %q", got)
	}
	if got := string(out["region"].(starlark.String)); got != "us-central1" {
		t.Fatalf("from_kwargs.region = %q", got)
	}
}

func TestStructImmutabilityAndErrors(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr string
	}{
		{
			name:    "positional arg rejected",
			code:    `s = struct(1)`,
			wantErr: "unexpected positional arguments",
		},
		{
			name:    "immutable field write",
			code:    "s = struct(a = 1)\ns.a = 2\n",
			wantErr: "can't assign",
		},
		{
			name:    "missing attr error",
			code:    "s = struct(a = 1)\nx = s.missing\n",
			wantErr: "no .missing attribute",
		},
		{
			name:    "bad plus operand",
			code:    "s = struct(a = 1) + 1\n",
			wantErr: "unknown binary op",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := execStructScript(t, tt.code, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestStructEqualityAndCrossComparison(t *testing.T) {
	goDefault := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"a": starlark.MakeInt(1),
		"b": starlark.String("x"),
	})
	goBranded := starlarkstruct.FromStringDict(starlark.String("branded"), starlark.StringDict{
		"a": starlark.MakeInt(1),
		"b": starlark.String("x"),
	})

	code := `
s1 = struct(a = 1, b = "x")
s2 = struct(b = "x", a = 1)
s3 = struct(a = 2, b = "x")
eq_same = (s1 == s2)
neq_diff = (s1 != s3)
eq_go_default = (s1 == go_default)
eq_go_branded = (s1 == go_branded)
`
	out, err := execStructScript(t, code, starlark.StringDict{
		"go_default": goDefault,
		"go_branded": goBranded,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, field := range []string{"eq_same", "neq_diff", "eq_go_default"} {
		if out[field] != starlark.True {
			t.Fatalf("%s = %v, want True", field, out[field])
		}
	}
	if out["eq_go_branded"] != starlark.False {
		t.Fatalf("eq_go_branded = %v, want False", out["eq_go_branded"])
	}

	_, err = execStructScript(t, `bad = struct(c = 3) + go_branded`, starlark.StringDict{
		"go_branded": goBranded,
	})
	if err == nil || !strings.Contains(err.Error(), "different constructors") {
		t.Fatalf("expected different constructors error, got: %v", err)
	}
}

func TestStructMergeHashingAndIntrospection(t *testing.T) {
	code := `
s1 = struct(a = 1, b = 2)
s2 = struct(b = 20, c = 3)
merged = s1 + s2
d = {s1: "found"}
key_hit = d[struct(b = 2, a = 1)]
attrs = dir(merged)
has_c = hasattr(merged, "c")
has_z = hasattr(merged, "z")
got_a = getattr(merged, "a")
got_fallback = getattr(merged, "missing", "fallback")
`
	out, err := execStructScript(t, code, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	merged := out["merged"].(*starlarkstruct.Struct)
	if v, err := merged.Attr("b"); err != nil || v.(starlark.Int).String() != "20" {
		t.Fatalf("merged.b = %v, err=%v; want 20", v, err)
	}
	if v, _ := out["s1"].(*starlarkstruct.Struct).Attr("b"); v.(starlark.Int).String() != "2" {
		t.Fatalf("s1 mutated: b = %v", v)
	}
	if got := string(out["key_hit"].(starlark.String)); got != "found" {
		t.Fatalf("dict key lookup = %q, want %q", got, "found")
	}
	if out["has_c"] != starlark.True || out["has_z"] != starlark.False {
		t.Fatalf("hasattr unexpected: c=%v z=%v", out["has_c"], out["has_z"])
	}
	if got := out["got_a"].(starlark.Int).String(); got != "1" {
		t.Fatalf("getattr(a) = %q, want 1", got)
	}
	if got := string(out["got_fallback"].(starlark.String)); got != "fallback" {
		t.Fatalf("getattr fallback = %q, want fallback", got)
	}

	_, err = execStructScript(t, `d = {struct(items = [1, 2]): "bad"}`, nil)
	if err == nil || !strings.Contains(err.Error(), "unhashable") {
		t.Fatalf("expected unhashable error, got: %v", err)
	}
}

func TestStructClosuresAndModuleBoundaryFreezing(t *testing.T) {
	dir := t.TempDir()
	modPath := filepath.Join(dir, "helper.star")
	mainPath := filepath.Join(dir, "main.star")

	modSrc := `
def make_client(base):
    return struct(
        base = base,
        tags = ["edge"],
        url = lambda path: base + path,
    )

client = make_client("https://api.example.invalid")
`
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	var printed []string
	rt, err := New(&Config{
		ScriptPath: mainPath,
		Print: func(_ *starlark.Thread, msg string) {
			printed = append(printed, msg)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	// Valid read and closure call across module boundary succeed cleanly.
	okScript := `
load("./helper.star", "helper")
print(helper.client.url("/v1/clusters"))
`
	if err := rt.Execute(context.Background(), okScript); err != nil {
		t.Fatalf("Execute across module boundary failed: %v", err)
	}
	if len(printed) != 1 || printed[0] != "https://api.example.invalid/v1/clusters" {
		t.Fatalf("closure call output = %v, want [https://api.example.invalid/v1/clusters]", printed)
	}

	// Nested mutable list inside loaded struct must be frozen across load().
	rt2, err := New(&Config{ScriptPath: mainPath})
	if err != nil {
		t.Fatal(err)
	}
	defer rt2.Close()

	mutScript := `
load("./helper.star", "helper")
helper.client.tags.append("mutated")
`
	err = rt2.Execute(context.Background(), mutScript)
	if err == nil || !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("expected frozen value mutation error, got: %v", err)
	}
}

func TestExpandStructs(t *testing.T) {
	// 1. Scalar fast path returns exact same value without allocation.
	for _, s := range []starlark.Value{
		starlark.None,
		starlark.True,
		starlark.MakeInt(42),
		starlark.Float(3.14),
		starlark.String("hello"),
		starlark.Bytes("raw"),
	} {
		got, err := ExpandStructs(s)
		if err != nil || got != s {
			t.Fatalf("scalar %T changed: got=%v err=%v", s, got, err)
		}
	}

	// 2. Plain containers without structs return the same pointer.
	plainDict := starlark.NewDict(1)
	plainDict.SetKey(starlark.String("k"), starlark.String("v"))
	if got, err := ExpandStructs(plainDict); err != nil || got != plainDict {
		t.Fatalf("plainDict returned copy (%v, err=%v), want identity", got, err)
	}
	plainList := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	if got, err := ExpandStructs(plainList); err != nil || got != plainList {
		t.Fatalf("plainList returned copy (%v, err=%v), want identity", got, err)
	}

	// 3. Nested structs in dicts, lists, and tuples expand recursively.
	inner := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"name": starlark.String("edge-1"),
	})
	outer := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"items": starlark.NewList([]starlark.Value{inner}),
		"pair":  starlark.Tuple{inner, starlark.String("ok")},
	})
	got, err := ExpandStructs(outer)
	if err != nil {
		t.Fatalf("ExpandStructs(outer) failed: %v", err)
	}
	outerDict, ok := got.(*starlark.Dict)
	if !ok {
		t.Fatalf("expected *starlark.Dict, got %T", got)
	}
	itemsVal, _, _ := outerDict.Get(starlark.String("items"))
	expandedInner := itemsVal.(*starlark.List).Index(0).(*starlark.Dict)
	nameVal, found, _ := expandedInner.Get(starlark.String("name"))
	if !found || string(nameVal.(starlark.String)) != "edge-1" {
		t.Fatalf("expanded nested list struct name = %v (found=%v)", nameVal, found)
	}
	pairVal, _, _ := outerDict.Get(starlark.String("pair"))
	pairTup := pairVal.(starlark.Tuple)
	if _, ok := pairTup[0].(*starlark.Dict); !ok {
		t.Fatalf("tuple element expected *starlark.Dict, got %T", pairTup[0])
	}

	// 4. Function field stays a callable (so encoders report unsupported type cleanly).
	fnStruct := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"fn": starlark.NewBuiltin("noop", func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
			return starlark.None, nil
		}),
	})
	fnExpanded, err := ExpandStructs(fnStruct)
	if err != nil {
		t.Fatalf("ExpandStructs(fnStruct) unexpected error: %v", err)
	}
	fnVal, _, _ := fnExpanded.(*starlark.Dict).Get(starlark.String("fn"))
	if _, ok := fnVal.(*starlark.Builtin); !ok {
		t.Fatalf("expected *starlark.Builtin preserved, got %T", fnVal)
	}

	// 5. Cyclic list wrapped inside struct triggers depth guard cleanly.
	cyclic := starlark.NewList(nil)
	if err := cyclic.Append(cyclic); err != nil {
		t.Fatal(err)
	}
	cyclicStruct := starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"loop": cyclic,
	})
	if _, err := ExpandStructs(cyclicStruct); err == nil || !strings.Contains(err.Error(), "maximum nesting depth") {
		t.Fatalf("expected nesting depth error on cycle, got: %v", err)
	}
}
