package strings

import (
	"testing"

	"github.com/project-starkite/starkite/libkite"
	"go.starlark.net/starlark"
)

func TestModuleMetadata(t *testing.T) {
	m := New()
	if m.Name() != ModuleName {
		t.Fatalf("expected name %s, got %s", ModuleName, m.Name())
	}
	if m.Description() == "" {
		t.Fatal("expected non-empty description")
	}
	if m.FactoryMethod() != "" {
		t.Fatalf("expected empty factory method, got %q", m.FactoryMethod())
	}
	if m.Aliases() != nil {
		t.Fatal("expected nil aliases for strings module")
	}
}

func TestPadFunctions(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// ljust default fillchar
	val, err := m.ljust(thread, nil, starlark.Tuple{starlark.String("cat"), starlark.MakeInt(6)}, nil)
	if err != nil {
		t.Fatalf("ljust error: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "cat   " {
		t.Fatalf("ljust got %q, want 'cat   '", val)
	}

	// rjust with custom fillchar
	val, err = m.rjust(thread, nil, starlark.Tuple{starlark.String("cat"), starlark.MakeInt(6), starlark.String("-")}, nil)
	if err != nil {
		t.Fatalf("rjust error: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "---cat" {
		t.Fatalf("rjust got %q, want '---cat'", val)
	}

	// center with default fillchar
	val, err = m.center(thread, nil, starlark.Tuple{starlark.String("cat"), starlark.MakeInt(7)}, nil)
	if err != nil {
		t.Fatalf("center error: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "  cat  " {
		t.Fatalf("center got %q, want '  cat  '", val)
	}

	// width <= string length
	val, err = m.center(thread, nil, starlark.Tuple{starlark.String("elephant"), starlark.MakeInt(4)}, nil)
	if err != nil {
		t.Fatalf("center width<=len error: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "elephant" {
		t.Fatalf("center got %q, want 'elephant'", val)
	}

	// multi-byte UTF-8 string
	val, err = m.ljust(thread, nil, starlark.Tuple{starlark.String("世界"), starlark.MakeInt(4), starlark.String(".")}, nil)
	if err != nil {
		t.Fatalf("ljust utf8 error: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "世界.." {
		t.Fatalf("ljust utf8 got %q, want '世界..'", val)
	}

	// Error when fillchar is empty
	_, err = m.ljust(thread, nil, starlark.Tuple{starlark.String("x"), starlark.MakeInt(5), starlark.String("")}, nil)
	if err == nil {
		t.Fatal("expected error for empty fillchar in ljust")
	}

	// Error when fillchar is multiple characters
	_, err = m.rjust(thread, nil, starlark.Tuple{starlark.String("x"), starlark.MakeInt(5), starlark.String("--")}, nil)
	if err == nil {
		t.Fatal("expected error for multi-char fillchar in rjust")
	}

	_, err = m.center(thread, nil, starlark.Tuple{starlark.String("x"), starlark.MakeInt(5), starlark.String("ab")}, nil)
	if err == nil {
		t.Fatal("expected error for multi-char fillchar in center")
	}
}

func TestCutEqualHasAny(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// cut
	val, err := m.cut(thread, nil, starlark.Tuple{starlark.String("key=value=extra"), starlark.String("=")}, nil)
	if err != nil {
		t.Fatalf("cut error: %v", err)
	}
	cutRes, ok := val.(*CutResult)
	if !ok {
		t.Fatalf("expected *CutResult, got %T", val)
	}
	if cutRes.Before != "key" || cutRes.After != "value=extra" || !cutRes.Found {
		t.Fatalf("cut result mismatch: before=%q, after=%q, found=%t", cutRes.Before, cutRes.After, cutRes.Found)
	}
	// Test attribute access
	attrVal, err := cutRes.Attr("before")
	if err != nil || attrVal != starlark.String("key") {
		t.Fatalf("Attr('before') = %v, err=%v", attrVal, err)
	}
	// Test sequence/indexing
	if cutRes.Len() != 3 || cutRes.Index(0) != starlark.String("key") || cutRes.Index(1) != starlark.String("value=extra") {
		t.Fatalf("cut indexing mismatch")
	}

	// equal (case-insensitive)
	val, err = m.equal(thread, nil, starlark.Tuple{starlark.String("Kite"), starlark.String("kite")}, nil)
	if err != nil || val != starlark.True {
		t.Fatalf("equal('Kite', 'kite') = %v, err=%v", val, err)
	}
	val, err = m.equal(thread, nil, starlark.Tuple{starlark.String("abc"), starlark.String("xyz")}, nil)
	if err != nil || val != starlark.False {
		t.Fatalf("equal('abc', 'xyz') = %v, err=%v", val, err)
	}

	// has_any with char string
	val, err = m.hasAny(thread, nil, starlark.Tuple{starlark.String("hello"), starlark.String("xyz e")}, nil)
	if err != nil || val != starlark.True {
		t.Fatalf("has_any with chars string failed: %v", val)
	}

	// has_any with list
	listArg := starlark.NewList([]starlark.Value{starlark.String("foo"), starlark.String("ell")})
	val, err = m.hasAny(thread, nil, starlark.Tuple{starlark.String("hello"), listArg}, nil)
	if err != nil || val != starlark.True {
		t.Fatalf("has_any with list failed: %v", val)
	}

	// has_any with tuple
	tupleArg := starlark.Tuple{starlark.String("bar"), starlark.String("baz")}
	val, err = m.hasAny(thread, nil, starlark.Tuple{starlark.String("hello"), tupleArg}, nil)
	if err != nil || val != starlark.False {
		t.Fatalf("has_any with tuple failed: %v", val)
	}
}

func TestQuoteAndUnquote(t *testing.T) {
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)
	thread := &starlark.Thread{Name: "test"}

	// quote
	val, err := m.quote(thread, nil, starlark.Tuple{starlark.String("hello\n\"world\"")}, nil)
	if err != nil {
		t.Fatalf("quote error: %v", err)
	}
	quotedStr, ok := val.(starlark.String)
	if !ok {
		t.Fatalf("quote returned non-string: %v", val)
	}

	// unquote
	val, err = m.unquote(thread, nil, starlark.Tuple{quotedStr}, nil)
	if err != nil {
		t.Fatalf("unquote error: %v", err)
	}
	if str, ok := val.(starlark.String); !ok || string(str) != "hello\n\"world\"" {
		t.Fatalf("unquote got %q, want original", val)
	}

	// try_unquote with malformed quoted string
	tryUnquoteVal, err := modVal.Attr("try_unquote")
	if err != nil || tryUnquoteVal == nil {
		t.Fatalf("try_unquote not found on module")
	}
	fn := tryUnquoteVal.(*starlark.Builtin)
	resVal, err := starlark.Call(thread, fn, starlark.Tuple{starlark.String("unterminated \"quote")}, nil)
	if err != nil {
		t.Fatalf("try_unquote should not return Go error: %v", err)
	}
	res, ok := resVal.(*libkite.Result)
	if !ok || bool(res.Truth()) {
		t.Fatalf("expected Result(ok=False), got %v", resVal)
	}
}
