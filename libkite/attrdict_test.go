package libkite

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"
)

func TestAttrDictBasic(t *testing.T) {
	data := map[string]any{
		"id":   "c1234567890abcdef",
		"name": "web-server",
		"port": 8080,
		"env": map[string]any{
			"MODE": "production",
		},
	}
	ad := NewAttrDict(data)

	// Truth
	if ad.Truth() != starlark.True {
		t.Fatal("expected Truth=True")
	}

	// Len
	if ad.Len() != 4 {
		t.Fatalf("expected Len=4, got %d", ad.Len())
	}

	// String formatting with id and name
	str := ad.String()
	if !strings.Contains(str, `id="c1234567890a"`) || !strings.Contains(str, `name="web-server"`) {
		t.Fatalf("unexpected String(): %s", str)
	}

	// Attr lookup (exact)
	val, err := ad.Attr("name")
	if err != nil || val.(starlark.String) != "web-server" {
		t.Fatalf("Attr('name') = %v, err: %v", val, err)
	}

	// Case-insensitive lookup (Name -> name)
	val, err = ad.Attr("Name")
	if err != nil || val.(starlark.String) != "web-server" {
		t.Fatalf("Attr('Name') = %v, err: %v", val, err)
	}

	// Nested map is also an AttrDict
	envVal, err := ad.Attr("env")
	if err != nil {
		t.Fatalf("Attr('env') err: %v", err)
	}
	envAD, ok := envVal.(*AttrDict)
	if !ok {
		t.Fatalf("expected *AttrDict for env, got %T", envVal)
	}
	modeVal, err := envAD.Attr("mode")
	if err != nil || modeVal.(starlark.String) != "production" {
		t.Fatalf("env.Attr('mode') = %v, err: %v", modeVal, err)
	}

	// Dict-style Get
	v, found, err := ad.Get(starlark.String("id"))
	if err != nil || !found || v.(starlark.String) != "c1234567890abcdef" {
		t.Fatalf("Get('id') = %v, found: %v, err: %v", v, found, err)
	}

	// Dict methods: keys, values, items, to_dict
	keysFn, err := ad.Attr("keys")
	if err != nil {
		t.Fatalf("Attr('keys') err: %v", err)
	}
	keysVal, err := starlark.Call(&starlark.Thread{}, keysFn, nil, nil)
	if err != nil || keysVal.(*starlark.List).Len() != 4 {
		t.Fatalf("keys() = %v, err: %v", keysVal, err)
	}

	toDictFn, err := ad.Attr("to_dict")
	if err != nil {
		t.Fatalf("Attr('to_dict') err: %v", err)
	}
	dictVal, err := starlark.Call(&starlark.Thread{}, toDictFn, nil, nil)
	if err != nil || dictVal.(*starlark.Dict).Len() != 4 {
		t.Fatalf("to_dict() = %v, err: %v", dictVal, err)
	}
}

func TestAttrDictSetKeyAndFreeze(t *testing.T) {
	ad := NewAttrDict(map[string]any{"initial": "yes"})

	// Mutation before freeze
	err := ad.SetKey(starlark.String("key1"), starlark.String("val1"))
	if err != nil {
		t.Fatalf("SetKey failed before freeze: %v", err)
	}
	v, ok := ad.ToMap()["key1"]
	if !ok || v != "val1" {
		t.Fatalf("expected key1=val1 in ToMap, got %v", v)
	}

	// Freeze
	ad.Freeze()

	// Mutation after freeze must fail
	err = ad.SetKey(starlark.String("key2"), starlark.String("val2"))
	if err == nil {
		t.Fatal("expected error modifying frozen AttrDict, got nil")
	}
	if !strings.Contains(err.Error(), "frozen") {
		t.Fatalf("expected error mentioning frozen, got %v", err)
	}
}

func TestAttrDict_ChildFreezeToMapIsolationAndToDict(t *testing.T) {
	orig := map[string]any{
		"nested": map[string]any{"k": "v"},
		"tags":   []any{"a", "b"},
		"get":    "collide",
	}
	ad := NewAttrDict(orig)

	// Grab child AttrDict view before freezing parent.
	subVal, err := ad.Attr("nested")
	if err != nil {
		t.Fatalf("Attr(nested): %v", err)
	}
	sub := subVal.(*AttrDict)

	// Mutating ToMap() clone must not leak into ad.
	cloned := ad.ToMap()
	cloned["nested"].(map[string]any)["k"] = "mutated"
	if got, _ := sub.GetField("k"); got != "v" {
		t.Fatalf("ToMap leaked internal mutable map: got %v, want v", got)
	}

	// Freezing parent must prevent mutating pre-existing child AttrDict view.
	ad.Freeze()
	if err := sub.SetKey(starlark.String("k"), starlark.String("hacked")); err == nil {
		t.Fatalf("expected child AttrDict view to reject mutation after parent Freeze")
	}

	// Returned list on frozen AttrDict must also be frozen.
	listVal, err := ad.Attr("tags")
	if err != nil {
		t.Fatalf("Attr(tags): %v", err)
	}
	if err := listVal.(*starlark.List).Append(starlark.String("c")); err == nil {
		t.Fatalf("expected list inside frozen AttrDict to be frozen")
	}

	// ToDict must always return a non-nil *starlark.Dict.
	dict := ad.ToDict()
	if dict == nil || dict.Len() != 3 {
		t.Fatalf("unexpected ToDict(): %v", dict)
	}

	// AttrNames must deduplicate method names colliding with data keys.
	var getCount int
	for _, n := range ad.AttrNames() {
		if n == "get" {
			getCount++
		}
	}
	if getCount != 1 {
		t.Fatalf("expected deduplicated AttrNames for 'get', got count=%d", getCount)
	}

	// Methods reject unexpected args.
	keysFn, _ := ad.Attr("keys")
	if _, callErr := starlark.Call(&starlark.Thread{}, keysFn, starlark.Tuple{starlark.MakeInt(1)}, nil); callErr == nil {
		t.Fatalf("expected keys(1) to error on unexpected positional arg")
	}
}
