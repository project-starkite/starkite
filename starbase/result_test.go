package starbase

import (
	"fmt"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func TestResultOK(t *testing.T) {
	r := ResultOK(starlark.String("hello"))

	if r.Type() != "Result" {
		t.Fatalf("Type() = %q, want Result", r.Type())
	}
	if r.Truth() != starlark.True {
		t.Fatal("Truth() should be True for ok result")
	}

	ok, _ := r.Attr("ok")
	if ok != starlark.True {
		t.Fatal("ok attr should be True")
	}
	val, _ := r.Attr("value")
	if val.(starlark.String) != "hello" {
		t.Fatalf("value = %v, want hello", val)
	}
	errAttr, _ := r.Attr("error")
	if string(errAttr.(starlark.String)) != "" {
		t.Fatalf("error = %q, want empty", errAttr)
	}
}

func TestResultErr(t *testing.T) {
	r := ResultErr("something failed")

	if r.Truth() != starlark.False {
		t.Fatal("Truth() should be False for error result")
	}

	ok, _ := r.Attr("ok")
	if ok != starlark.False {
		t.Fatal("ok attr should be False")
	}
	val, _ := r.Attr("value")
	if val != starlark.None {
		t.Fatalf("value = %v, want None", val)
	}
	errAttr, _ := r.Attr("error")
	if string(errAttr.(starlark.String)) != "something failed" {
		t.Fatalf("error = %q, want 'something failed'", errAttr)
	}
}

func TestResultNilValue(t *testing.T) {
	r := ResultOK(nil)
	val, _ := r.Attr("value")
	if val != starlark.None {
		t.Fatalf("ResultOK(nil) value = %v, want None", val)
	}
}

func TestResultString(t *testing.T) {
	ok := ResultOK(starlark.MakeInt(42))
	if s := ok.String(); s != `Result(ok=True, value=42)` {
		t.Fatalf("String() = %q", s)
	}

	fail := ResultErr("boom")
	if s := fail.String(); s != `Result(ok=False, error="boom")` {
		t.Fatalf("String() = %q", s)
	}
}

func TestResultAttrNames(t *testing.T) {
	r := ResultOK(starlark.None)
	names := r.AttrNames()
	if len(names) != 3 {
		t.Fatalf("AttrNames() len = %d, want 3", len(names))
	}
	// Should be sorted: error, ok, value
	if names[0] != "error" || names[1] != "ok" || names[2] != "value" {
		t.Fatalf("AttrNames() = %v", names)
	}
}

func TestResultUnknownAttr(t *testing.T) {
	r := ResultOK(starlark.None)
	v, err := r.Attr("unknown")
	if v != nil || err != nil {
		t.Fatalf("unknown attr: v=%v, err=%v", v, err)
	}
}

func TestTryWrapSuccess(t *testing.T) {
	base := starlark.NewBuiltin("add", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.MakeInt(42), nil
	})

	wrapped := TryWrap("try_add", base)
	thread := &starlark.Thread{Name: "test"}
	val, err := starlark.Call(thread, wrapped, nil, nil)
	if err != nil {
		t.Fatalf("TryWrap call error: %v", err)
	}

	r, ok := val.(*Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}
	if !r.ok {
		t.Fatal("expected ok=true")
	}
	v, _ := r.Attr("value")
	if v.(starlark.Int) != starlark.MakeInt(42) {
		t.Fatalf("value = %v, want 42", v)
	}
}

func TestTryWrapError(t *testing.T) {
	base := starlark.NewBuiltin("fail", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return nil, fmt.Errorf("not found")
	})

	wrapped := TryWrap("try_fail", base)
	thread := &starlark.Thread{Name: "test"}
	val, err := starlark.Call(thread, wrapped, nil, nil)
	if err != nil {
		t.Fatalf("TryWrap should not return error, got: %v", err)
	}

	r, ok := val.(*Result)
	if !ok {
		t.Fatalf("expected *Result, got %T", val)
	}
	if r.ok {
		t.Fatal("expected ok=false")
	}
	e, _ := r.Attr("error")
	if !containsStr(string(e.(starlark.String)), "not found") {
		t.Fatalf("error = %q, want 'not found'", e)
	}
}

func TestTryModuleDirectLookup(t *testing.T) {
	fn := starlark.NewBuiltin("test.hello", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return starlark.String("world"), nil
	})

	m := NewTryModule("test", starlark.StringDict{
		"hello": fn,
	})

	v, err := m.Attr("hello")
	if err != nil {
		t.Fatal(err)
	}
	if v != fn {
		t.Fatal("direct lookup should return the original builtin")
	}
}

func TestTryModuleTryPrefix(t *testing.T) {
	fn := starlark.NewBuiltin("test.get", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return nil, fmt.Errorf("resource not found")
	})

	m := NewTryModule("test", starlark.StringDict{
		"get": fn,
	})

	v, err := m.Attr("try_get")
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		t.Fatal("try_get should resolve")
	}

	// Call the wrapped function — should return Result, not error
	thread := &starlark.Thread{Name: "test"}
	result, err := starlark.Call(thread, v, nil, nil)
	if err != nil {
		t.Fatalf("try_ call should not error: %v", err)
	}
	r := result.(*Result)
	if r.ok {
		t.Fatal("expected ok=false")
	}
}

func TestTryModuleUnknownAttr(t *testing.T) {
	m := NewTryModule("test", starlark.StringDict{
		"get": starlark.NewBuiltin("test.get", nil),
	})

	v, err := m.Attr("nonexistent")
	if err != nil || v != nil {
		t.Fatalf("unknown attr: v=%v, err=%v", v, err)
	}

	// try_ of nonexistent base
	v, err = m.Attr("try_nonexistent")
	if err != nil || v != nil {
		t.Fatalf("try_ unknown: v=%v, err=%v", v, err)
	}
}

func TestTryModuleAttrNames(t *testing.T) {
	m := NewTryModule("test", starlark.StringDict{
		"get":  starlark.NewBuiltin("test.get", nil),
		"list": starlark.NewBuiltin("test.list", nil),
		"obj":  starlark.String("not a builtin"),
	})

	names := m.AttrNames()
	// Expect: get, list, obj, try_get, try_list (obj is not a builtin, no try_ variant)
	expected := map[string]bool{
		"get": true, "list": true, "obj": true,
		"try_get": true, "try_list": true,
	}
	if len(names) != len(expected) {
		t.Fatalf("AttrNames() = %v (len %d), want %d entries", names, len(names), len(expected))
	}
	for _, n := range names {
		if !expected[n] {
			t.Fatalf("unexpected attr name: %q", n)
		}
	}
}

func TestTryModuleNonBuiltinNoTry(t *testing.T) {
	// Non-builtin members should not get try_ variants
	m := NewTryModule("test", starlark.StringDict{
		"version": starlark.String("1.0"),
	})

	v, err := m.Attr("try_version")
	if err != nil || v != nil {
		t.Fatalf("try_ of non-builtin: v=%v, err=%v", v, err)
	}
}

func TestTryModuleStringAndType(t *testing.T) {
	m := NewTryModule("k8s", starlark.StringDict{})
	if m.Type() != "module" {
		t.Fatalf("Type() = %q", m.Type())
	}
	if m.String() != `<module "k8s">` {
		t.Fatalf("String() = %q", m.String())
	}
	if m.Truth() != starlark.True {
		t.Fatal("Truth() should be True")
	}
}

// TestTryWrapPassesArgs verifies that positional and keyword args are forwarded
// through TryWrap, and that wrong arg count is captured as ResultErr.
func TestTryWrapPassesArgs(t *testing.T) {
	base := starlark.NewBuiltin("add", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		var a, b int
		if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "a", &a, "b", &b); err != nil {
			return nil, err
		}
		return starlark.MakeInt(a + b), nil
	})

	wrapped := TryWrap("try_add", base)
	thread := &starlark.Thread{Name: "test"}

	// Positional args
	val, err := starlark.Call(thread, wrapped, starlark.Tuple{starlark.MakeInt(3), starlark.MakeInt(7)}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := val.(*Result)
	if !r.ok {
		t.Fatal("expected ok=true for valid args")
	}
	v, _ := r.Attr("value")
	if v.(starlark.Int) != starlark.MakeInt(10) {
		t.Fatalf("value = %v, want 10", v)
	}

	// Kwargs
	kwargs := []starlark.Tuple{
		{starlark.String("a"), starlark.MakeInt(5)},
		{starlark.String("b"), starlark.MakeInt(2)},
	}
	val, err = starlark.Call(thread, wrapped, nil, kwargs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r = val.(*Result)
	if !r.ok {
		t.Fatal("expected ok=true for valid kwargs")
	}
	v, _ = r.Attr("value")
	if v.(starlark.Int) != starlark.MakeInt(7) {
		t.Fatalf("value = %v, want 7", v)
	}

	// Wrong arg count → captured as ResultErr
	val, err = starlark.Call(thread, wrapped, starlark.Tuple{starlark.MakeInt(1)}, nil)
	if err != nil {
		t.Fatalf("TryWrap should not propagate error, got: %v", err)
	}
	r = val.(*Result)
	if r.ok {
		t.Fatal("expected ok=false for wrong arg count")
	}
	e, _ := r.Attr("error")
	if string(e.(starlark.String)) == "" {
		t.Fatal("expected non-empty error message")
	}
}

// TestTryWrapPreservesReturnTypes verifies that Dict, List, and Tuple survive
// wrapping in Result.value with their types intact.
func TestTryWrapPreservesReturnTypes(t *testing.T) {
	thread := &starlark.Thread{Name: "test"}

	tests := []struct {
		name     string
		retVal   starlark.Value
		checkTyp string
	}{
		{"Dict", starlark.NewDict(1), "*starlark.Dict"},
		{"List", starlark.NewList([]starlark.Value{starlark.MakeInt(1)}), "*starlark.List"},
		{"Tuple", starlark.Tuple{starlark.String("a"), starlark.String("b")}, "starlark.Tuple"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := starlark.NewBuiltin("ret", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				return tt.retVal, nil
			})

			wrapped := TryWrap("try_ret", base)
			val, err := starlark.Call(thread, wrapped, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			r := val.(*Result)
			if !r.ok {
				t.Fatal("expected ok=true")
			}

			v, _ := r.Attr("value")
			gotType := fmt.Sprintf("%T", v)
			if gotType != tt.checkTyp {
				t.Fatalf("value type = %s, want %s", gotType, tt.checkTyp)
			}
		})
	}
}

// TestResultFreeze verifies that Freeze() propagates to the inner value.
func TestResultFreeze(t *testing.T) {
	list := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)})
	r := ResultOK(list)

	// Before freeze, append should work
	if err := list.Append(starlark.MakeInt(3)); err != nil {
		t.Fatalf("append before freeze: %v", err)
	}

	// Freeze the result
	r.Freeze()

	// After freeze, append should fail
	if err := list.Append(starlark.MakeInt(4)); err == nil {
		t.Fatal("expected error appending to frozen list")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRetryResultOK(t *testing.T) {
	rr := NewRetryResult(true, starlark.String("done"), "", 3, 1500*time.Millisecond, []string{"err1", "err2"})

	if rr.Type() != "RetryResult" {
		t.Fatalf("Type() = %q, want RetryResult", rr.Type())
	}
	if rr.Truth() != starlark.True {
		t.Fatal("Truth() should be True for ok result")
	}

	ok, _ := rr.Attr("ok")
	if ok != starlark.True {
		t.Fatal("ok should be True")
	}
	val, _ := rr.Attr("value")
	if val.(starlark.String) != "done" {
		t.Fatalf("value = %v, want done", val)
	}
	errAttr, _ := rr.Attr("error")
	if string(errAttr.(starlark.String)) != "" {
		t.Fatalf("error = %q, want empty", errAttr)
	}
	attempts, _ := rr.Attr("attempts")
	if attempts.(starlark.Int) != starlark.MakeInt(3) {
		t.Fatalf("attempts = %v, want 3", attempts)
	}
	elapsed, _ := rr.Attr("elapsed")
	if string(elapsed.(starlark.String)) != "1.5s" {
		t.Fatalf("elapsed = %q, want 1.5s", elapsed)
	}
	errors, _ := rr.Attr("errors")
	errList := errors.(*starlark.List)
	if errList.Len() != 2 {
		t.Fatalf("errors len = %d, want 2", errList.Len())
	}
}

func TestRetryResultFail(t *testing.T) {
	rr := NewRetryResult(false, starlark.None, "final error", 5, 3*time.Second, []string{"err1", "err2", "err3", "err4", "final error"})

	if rr.Truth() != starlark.False {
		t.Fatal("Truth() should be False for failed result")
	}
	ok, _ := rr.Attr("ok")
	if ok != starlark.False {
		t.Fatal("ok should be False")
	}
	errAttr, _ := rr.Attr("error")
	if string(errAttr.(starlark.String)) != "final error" {
		t.Fatalf("error = %q, want 'final error'", errAttr)
	}
	errors, _ := rr.Attr("errors")
	errList := errors.(*starlark.List)
	if errList.Len() != 5 {
		t.Fatalf("errors len = %d, want 5", errList.Len())
	}
}

func TestRetryResultTruth(t *testing.T) {
	okResult := NewRetryResult(true, starlark.None, "", 1, time.Millisecond, nil)
	if okResult.Truth() != starlark.True {
		t.Fatal("ok RetryResult should be truthy")
	}

	failResult := NewRetryResult(false, starlark.None, "err", 3, time.Second, nil)
	if failResult.Truth() != starlark.False {
		t.Fatal("failed RetryResult should be falsy")
	}
}

func TestRetryResultType(t *testing.T) {
	rr := NewRetryResult(true, starlark.None, "", 1, 0, nil)
	if rr.Type() != "RetryResult" {
		t.Fatalf("Type() = %q, want RetryResult", rr.Type())
	}
}

func TestRetryResultString(t *testing.T) {
	ok := NewRetryResult(true, starlark.None, "", 2, 500*time.Millisecond, nil)
	s := ok.String()
	if !containsStr(s, "ok=True") || !containsStr(s, "attempts=2") {
		t.Fatalf("String() = %q", s)
	}

	fail := NewRetryResult(false, starlark.None, "boom", 3, time.Second, nil)
	s = fail.String()
	if !containsStr(s, "ok=False") || !containsStr(s, "boom") {
		t.Fatalf("String() = %q", s)
	}
}

func TestRetryResultAttrNames(t *testing.T) {
	rr := NewRetryResult(true, starlark.None, "", 1, 0, nil)
	names := rr.AttrNames()
	if len(names) != 6 {
		t.Fatalf("AttrNames() len = %d, want 6", len(names))
	}
	expected := []string{"attempts", "elapsed", "error", "errors", "ok", "value"}
	for i, name := range expected {
		if names[i] != name {
			t.Fatalf("AttrNames()[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestRetryResultFreeze(t *testing.T) {
	list := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	rr := NewRetryResult(true, list, "", 1, 0, nil)
	rr.Freeze()
	// After freeze, appending to the inner list should fail
	if err := list.Append(starlark.MakeInt(2)); err == nil {
		t.Fatal("expected error appending to frozen list")
	}
}
