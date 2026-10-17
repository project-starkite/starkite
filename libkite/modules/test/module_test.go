package test

import (
	"strings"
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
	_, _ = m.Load(&libkite.ModuleConfig{})
	aliases := m.Aliases()
	for _, fn := range []string{"assert", "assert_equal", "assert_not_equal", "assert_contains", "assert_true", "assert_false", "skip", "fail"} {
		if _, ok := aliases[fn]; !ok {
			t.Fatalf("expected global alias %q", fn)
		}
	}
}

func TestAssertionsPass(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// assert(True)
	val, err := m.assert_(thread, nil, starlark.Tuple{starlark.True}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert(True) failed: %v", err)
	}

	// assert_true(True)
	val, err = m.assertTrue(thread, nil, starlark.Tuple{starlark.True}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_true(True) failed: %v", err)
	}

	// assert_false(False)
	val, err = m.assertFalse(thread, nil, starlark.Tuple{starlark.False}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_false(False) failed: %v", err)
	}

	// assert_equal
	val, err = m.assertEqual(thread, nil, starlark.Tuple{starlark.MakeInt(42), starlark.MakeInt(42)}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_equal(42, 42) failed: %v", err)
	}

	// assert_not_equal
	val, err = m.assertNotEqual(thread, nil, starlark.Tuple{starlark.String("a"), starlark.String("b")}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_not_equal('a', 'b') failed: %v", err)
	}

	// assert_contains with list
	list := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)})
	val, err = m.assertContains(thread, nil, starlark.Tuple{list, starlark.MakeInt(2)}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_contains(list, 2) failed: %v", err)
	}

	// assert_contains with string
	val, err = m.assertContains(thread, nil, starlark.Tuple{starlark.String("hello world"), starlark.String("world")}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_contains(str, 'world') failed: %v", err)
	}

	// assert_contains with dict
	dict := starlark.NewDict(1)
	_ = dict.SetKey(starlark.String("k"), starlark.MakeInt(1))
	val, err = m.assertContains(thread, nil, starlark.Tuple{dict, starlark.String("k")}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_contains(dict, 'k') failed: %v", err)
	}

	// assert_contains with tuple
	tup := starlark.Tuple{starlark.MakeInt(10), starlark.MakeInt(20)}
	val, err = m.assertContains(thread, nil, starlark.Tuple{tup, starlark.MakeInt(10)}, nil)
	if err != nil || val != starlark.None {
		t.Fatalf("assert_contains(tuple, 10) failed: %v", err)
	}
}

func TestAssertionsFail(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// assert(False, "got %d", 5)
	_, err := m.assert_(thread, nil, starlark.Tuple{starlark.False, starlark.String("got %d"), starlark.MakeInt(5)}, nil)
	if err == nil || !strings.Contains(err.Error(), "got 5") {
		t.Fatalf("assert(False) expected formatted error, got: %v", err)
	}

	// assert_true(False)
	_, err = m.assertTrue(thread, nil, starlark.Tuple{starlark.False}, nil)
	if err == nil {
		t.Fatal("assert_true(False) should fail")
	}

	// assert_false(True)
	_, err = m.assertFalse(thread, nil, starlark.Tuple{starlark.True}, nil)
	if err == nil {
		t.Fatal("assert_false(True) should fail")
	}

	// assert_equal(1, 2)
	_, err = m.assertEqual(thread, nil, starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)}, nil)
	if err == nil {
		t.Fatal("assert_equal(1, 2) should fail")
	}

	// assert_not_equal(1, 1)
	_, err = m.assertNotEqual(thread, nil, starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(1)}, nil)
	if err == nil {
		t.Fatal("assert_not_equal(1, 1) should fail")
	}

	// assert_contains missing
	list := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)})
	_, err = m.assertContains(thread, nil, starlark.Tuple{list, starlark.MakeInt(99)}, nil)
	if err == nil {
		t.Fatal("assert_contains missing element should fail")
	}
}

func TestSkipAndFail(t *testing.T) {
	m := New()
	_, _ = m.Load(&libkite.ModuleConfig{})
	thread := &starlark.Thread{Name: "test"}

	// skip
	_, err := m.skip(thread, starlark.NewBuiltin("test.skip", m.skip), starlark.Tuple{starlark.String("not ready")}, nil)
	if err == nil || !strings.Contains(err.Error(), "test skipped: not ready") {
		t.Fatalf("skip error unexpected: %v", err)
	}

	// fail
	_, err = m.fail(thread, starlark.NewBuiltin("test.fail", m.fail), starlark.Tuple{starlark.String("boom")}, nil)
	if err == nil || !strings.Contains(err.Error(), "test failed: boom") {
		t.Fatalf("fail error unexpected: %v", err)
	}
}

func TestTryAssertions(t *testing.T) {
	m := New()
	dict, _ := m.Load(&libkite.ModuleConfig{})
	modVal := dict[string(ModuleName)].(starlark.HasAttrs)
	thread := &starlark.Thread{Name: "test"}

	tryAssertVal, err := modVal.Attr("try_assert")
	if err != nil || tryAssertVal == nil {
		t.Fatalf("try_assert not found on module: %v", err)
	}
	fn := tryAssertVal.(*starlark.Builtin)

	// try_assert(True) -> ok=True
	resVal, err := starlark.Call(thread, fn, starlark.Tuple{starlark.True}, nil)
	if err != nil {
		t.Fatalf("try_assert call error: %v", err)
	}
	res := resVal.(*libkite.Result)
	if !bool(res.Truth()) {
		t.Fatalf("expected Result(ok=True), got %v", res)
	}

	// try_assert(False) -> ok=False
	resVal, err = starlark.Call(thread, fn, starlark.Tuple{starlark.False}, nil)
	if err != nil {
		t.Fatalf("try_assert call error: %v", err)
	}
	res = resVal.(*libkite.Result)
	if bool(res.Truth()) {
		t.Fatalf("expected Result(ok=False), got %v", res)
	}
}
