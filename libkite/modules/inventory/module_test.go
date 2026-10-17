package inventory

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestInventoryModuleLoad(t *testing.T) {
	mod := New()
	dict, err := mod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	m, ok := dict[string(ModuleName)]
	if !ok {
		t.Fatalf("module %q not found", ModuleName)
	}

	hasAttrs, ok := m.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs, got %T", m)
	}

	expected := []string{
		"file", "try_file",
		"list", "try_list",
		"filter", "try_filter",
		"group_by", "try_group_by",
		"merge", "try_merge",
		"addresses", "try_addresses",
	}
	for _, name := range expected {
		val, err := hasAttrs.Attr(name)
		if err != nil || val == nil {
			t.Fatalf("missing expected member %q: %v", name, err)
		}
	}
}

func TestInventoryFilterCallSafe(t *testing.T) {
	mod := New()
	dict, _ := mod.Load(&libkite.ModuleConfig{})
	m := dict[string(ModuleName)].(starlark.HasAttrs)
	filterFn, _ := m.Attr("filter")

	inv := &InventoryValue{
		items: []map[string]any{
			{"host": "web1", "role": "web"},
			{"host": "db1", "role": "db"},
			{"host": "web2", "role": "web"},
		},
	}

	predicate := starlark.NewBuiltin("pred", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		d := args[0].(*starlark.Dict)
		v, found, _ := d.Get(starlark.String("role"))
		if found && v.(starlark.String) == "web" {
			return starlark.True, nil
		}
		return starlark.False, nil
	})

	thread := &starlark.Thread{Name: "test"}
	res, err := starlark.Call(thread, filterFn, starlark.Tuple{inv, predicate}, nil)
	if err != nil {
		t.Fatalf("filter failed: %v", err)
	}

	filteredInv := res.(*InventoryValue)
	if len(filteredInv.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(filteredInv.items))
	}
}
