package table

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func TestTableModuleLoad(t *testing.T) {
	mod := New()
	dict, err := mod.Load(&libkite.ModuleConfig{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	m, ok := dict[string(ModuleName)]
	if !ok {
		t.Fatalf("module %q not found in load dict", ModuleName)
	}

	hasAttrs, ok := m.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("expected HasAttrs, got %T", m)
	}

	for _, member := range []string{"new", "try_new", "print", "try_print"} {
		attr, err := hasAttrs.Attr(member)
		if err != nil || attr == nil {
			t.Fatalf("missing expected member %q: %v", member, err)
		}
	}
}

func TestTableAddRowAndRender(t *testing.T) {
	mod := New()
	dict, _ := mod.Load(&libkite.ModuleConfig{})
	m := dict[string(ModuleName)].(starlark.HasAttrs)
	newFn, _ := m.Attr("new")

	thread := &starlark.Thread{Name: "test"}
	headers := starlark.NewList([]starlark.Value{
		starlark.String("NAME"),
		starlark.String("STATUS"),
		starlark.String("AGE"),
	})

	val, err := starlark.Call(thread, newFn, starlark.Tuple{headers}, nil)
	if err != nil {
		t.Fatalf("new() failed: %v", err)
	}

	tbl, ok := val.(*TableValue)
	if !ok {
		t.Fatalf("expected *TableValue, got %T", val)
	}

	// Add exact row
	addRowFn, _ := tbl.Attr("add_row")
	_, err = starlark.Call(thread, addRowFn, starlark.Tuple{
		starlark.String("pod-1"),
		starlark.String("Running"),
		starlark.String("10m"),
	}, nil)
	if err != nil {
		t.Fatalf("add_row exact failed: %v", err)
	}

	// Add short row (should pad with "")
	_, err = starlark.Call(thread, addRowFn, starlark.Tuple{
		starlark.String("pod-2"),
		starlark.String("Pending"),
	}, nil)
	if err != nil {
		t.Fatalf("add_row short failed: %v", err)
	}

	// Add excess row (should error)
	_, err = starlark.Call(thread, addRowFn, starlark.Tuple{
		starlark.String("pod-3"),
		starlark.String("Failed"),
		starlark.String("1m"),
		starlark.String("extra"),
	}, nil)
	if err == nil {
		t.Fatal("expected error for excess columns in add_row, got nil")
	}

	// Test try_add_row with excess columns
	tryAddRowFn, err := tbl.Attr("try_add_row")
	if err != nil || tryAddRowFn == nil {
		t.Fatalf("try_add_row missing: %v", err)
	}
	resVal, err := starlark.Call(thread, tryAddRowFn, starlark.Tuple{
		starlark.String("pod-3"),
		starlark.String("Failed"),
		starlark.String("1m"),
		starlark.String("extra"),
	}, nil)
	if err != nil {
		t.Fatalf("try_add_row call returned error instead of Result: %v", err)
	}
	res, ok := resVal.(*libkite.Result)
	if !ok {
		t.Fatalf("expected *libkite.Result, got %T", resVal)
	}
	if res.Truth() != starlark.False {
		t.Fatal("expected try_add_row with excess columns to return ok=False")
	}

	// Check row count
	rowCountVal, _ := tbl.Attr("row_count")
	if rowCountVal.(starlark.Int) != starlark.MakeInt(2) {
		t.Fatalf("expected row_count 2, got %v", rowCountVal)
	}

	// Render
	renderFn, _ := tbl.Attr("render")
	renderedVal, err := starlark.Call(thread, renderFn, nil, nil)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	rendered := string(renderedVal.(starlark.String))
	if !strings.Contains(rendered, "pod-1") || !strings.Contains(rendered, "pod-2") {
		t.Fatalf("rendered table missing rows: %s", rendered)
	}

	// Test try_render
	tryRenderFn, _ := tbl.Attr("try_render")
	tryRenderVal, err := starlark.Call(thread, tryRenderFn, nil, nil)
	if err != nil {
		t.Fatalf("try_render failed: %v", err)
	}
	resRender := tryRenderVal.(*libkite.Result)
	if resRender.Truth() != starlark.True {
		t.Fatal("expected try_render ok=True")
	}
}
