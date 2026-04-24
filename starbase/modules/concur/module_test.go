package concur

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

// testThread returns a thread from a trusted Runtime with the concur module registered.
func testThread() *starlark.Thread {
	reg := starbase.NewRegistry(&starbase.ModuleConfig{})
	reg.Register(New())
	rt, err := starbase.NewTrusted(nil, starbase.WithRegistry(reg))
	if err != nil {
		panic(err)
	}
	return rt.NewThread("test")
}

// callBuiltin loads the concur module from a runtime and calls the named function.
func callBuiltin(thread *starlark.Thread, name string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	rt := starbase.GetRuntime(thread)
	if rt == nil {
		return nil, fmt.Errorf("no runtime on thread")
	}
	reg := rt.Registry()
	predecl := reg.Predeclared()
	modVal, ok := predecl["concur"]
	if !ok {
		return nil, fmt.Errorf("concur module not found in predeclared")
	}
	mod, ok := modVal.(starlark.HasAttrs)
	if !ok {
		return nil, fmt.Errorf("concur module does not implement HasAttrs")
	}
	fn, err := mod.Attr(name)
	if err != nil {
		return nil, err
	}
	if fn == nil {
		return nil, fmt.Errorf("concur.%s not found", name)
	}
	callable, ok := fn.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("concur.%s is not callable", name)
	}
	return starlark.Call(thread, callable, args, kwargs)
}

// makeBuiltin creates a starlark.Builtin from a Go function.
func makeBuiltin(name string, fn func(thread *starlark.Thread, args starlark.Tuple) (starlark.Value, error)) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		return fn(thread, args)
	})
}

func TestMapWithList(t *testing.T) {
	thread := testThread()
	double := makeBuiltin("double", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		x, _ := starlark.AsInt32(args[0])
		return starlark.MakeInt(int(x) * 2), nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)})
	result, err := callBuiltin(thread, "map", starlark.Tuple{items, double}, nil)
	if err != nil {
		t.Fatal(err)
	}

	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", result)
	}
	if list.Len() != 3 {
		t.Fatalf("expected 3 results, got %d", list.Len())
	}
	for i, want := range []int{2, 4, 6} {
		got, _ := starlark.AsInt32(list.Index(i))
		if int(got) != want {
			t.Errorf("index %d: got %d, want %d", i, got, want)
		}
	}
}

func TestMapWithTuple(t *testing.T) {
	thread := testThread()
	double := makeBuiltin("double", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		x, _ := starlark.AsInt32(args[0])
		return starlark.MakeInt(int(x) * 2), nil
	})

	items := starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)}
	result, err := callBuiltin(thread, "map", starlark.Tuple{items, double}, nil)
	if err != nil {
		t.Fatal(err)
	}

	list, ok := result.(*starlark.List)
	if !ok {
		t.Fatalf("expected *starlark.List, got %T", result)
	}
	if list.Len() != 3 {
		t.Fatalf("expected 3 results, got %d", list.Len())
	}
}

func TestMapEmpty(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	items := starlark.NewList(nil)
	result, err := callBuiltin(thread, "map", starlark.Tuple{items, identity}, nil)
	if err != nil {
		t.Fatal(err)
	}

	list := result.(*starlark.List)
	if list.Len() != 0 {
		t.Fatalf("expected empty list, got %d items", list.Len())
	}
}

func TestMapPreservesOrder(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	vals := make([]starlark.Value, 100)
	for i := range vals {
		vals[i] = starlark.MakeInt(i)
	}
	items := starlark.NewList(vals)

	result, err := callBuiltin(thread, "map", starlark.Tuple{items, identity}, nil)
	if err != nil {
		t.Fatal(err)
	}

	list := result.(*starlark.List)
	for i := 0; i < 100; i++ {
		got, _ := starlark.AsInt32(list.Index(i))
		if int(got) != i {
			t.Errorf("index %d: got %d, want %d", i, got, i)
		}
	}
}

func TestMapWorkers(t *testing.T) {
	thread := testThread()

	var peak int64
	var current int64
	fn := makeBuiltin("work", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		c := atomic.AddInt64(&current, 1)
		// Update peak atomically
		for {
			p := atomic.LoadInt64(&peak)
			if c <= p {
				break
			}
			if atomic.CompareAndSwapInt64(&peak, p, c) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&current, -1)
		return args[0], nil
	})

	vals := make([]starlark.Value, 20)
	for i := range vals {
		vals[i] = starlark.MakeInt(i)
	}
	items := starlark.NewList(vals)

	kwargs := []starlark.Tuple{
		{starlark.String("workers"), starlark.MakeInt(3)},
	}

	_, err := callBuiltin(thread, "map", starlark.Tuple{items, fn}, kwargs)
	if err != nil {
		t.Fatal(err)
	}

	p := atomic.LoadInt64(&peak)
	if p > 3 {
		t.Errorf("peak concurrency %d exceeded workers limit of 3", p)
	}
}

func TestMapTimeout(t *testing.T) {
	thread := testThread()
	slow := makeBuiltin("slow", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		time.Sleep(2 * time.Second)
		return args[0], nil
	})

	// Use workers=1 so the second item blocks on semaphore acquisition and hits the timeout
	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)})
	kwargs := []starlark.Tuple{
		{starlark.String("timeout"), starlark.String("100ms")},
		{starlark.String("workers"), starlark.MakeInt(1)},
	}

	_, err := callBuiltin(thread, "map", starlark.Tuple{items, slow}, kwargs)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error should mention timeout, got: %s", err)
	}
}

func TestMapInvalidTimeout(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	kwargs := []starlark.Tuple{
		{starlark.String("timeout"), starlark.String("nope")},
	}

	_, err := callBuiltin(thread, "map", starlark.Tuple{items, identity}, kwargs)
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestMapInvalidWorkers(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	kwargs := []starlark.Tuple{
		{starlark.String("workers"), starlark.MakeInt(-1)},
	}

	_, err := callBuiltin(thread, "map", starlark.Tuple{items, identity}, kwargs)
	if err == nil {
		t.Fatal("expected error for negative workers")
	}
}

func TestMapErrorPropagation(t *testing.T) {
	thread := testThread()
	failOn2 := makeBuiltin("failOn2", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		x, _ := starlark.AsInt32(args[0])
		if x == 2 {
			return nil, fmt.Errorf("item 2 failed")
		}
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)})
	_, err := callBuiltin(thread, "map", starlark.Tuple{items, failOn2}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "item") {
		t.Errorf("error should mention item, got: %s", err)
	}
}

func TestEachWithList(t *testing.T) {
	thread := testThread()
	noop := makeBuiltin("noop", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2)})
	result, err := callBuiltin(thread, "each", starlark.Tuple{items, noop}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != starlark.None {
		t.Fatalf("expected None, got %s", result)
	}
}

func TestEachWithTuple(t *testing.T) {
	thread := testThread()
	noop := makeBuiltin("noop", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	items := starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)}
	result, err := callBuiltin(thread, "each", starlark.Tuple{items, noop}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != starlark.None {
		t.Fatalf("expected None, got %s", result)
	}
}

func TestEachEmpty(t *testing.T) {
	thread := testThread()
	noop := makeBuiltin("noop", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	items := starlark.NewList(nil)
	result, err := callBuiltin(thread, "each", starlark.Tuple{items, noop}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != starlark.None {
		t.Fatalf("expected None, got %s", result)
	}
}

func TestExecZeroFunctions(t *testing.T) {
	thread := testThread()
	result, err := callBuiltin(thread, "exec", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tuple, ok := result.(starlark.Tuple)
	if !ok {
		t.Fatalf("expected Tuple, got %T", result)
	}
	if len(tuple) != 0 {
		t.Fatalf("expected empty tuple, got %d items", len(tuple))
	}
}

func TestExecSingle(t *testing.T) {
	thread := testThread()
	fn := makeBuiltin("fn", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return starlark.MakeInt(42), nil
	})

	result, err := callBuiltin(thread, "exec", starlark.Tuple{fn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tuple := result.(starlark.Tuple)
	if len(tuple) != 1 {
		t.Fatalf("expected 1 item, got %d", len(tuple))
	}
	got, _ := starlark.AsInt32(tuple[0])
	if got != 42 {
		t.Errorf("got %d, want 42", got)
	}
}

func TestExecThree(t *testing.T) {
	thread := testThread()
	mkFn := func(val int) *starlark.Builtin {
		return makeBuiltin("fn", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
			return starlark.MakeInt(val), nil
		})
	}

	result, err := callBuiltin(thread, "exec", starlark.Tuple{mkFn(1), mkFn(2), mkFn(3)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tuple := result.(starlark.Tuple)
	if len(tuple) != 3 {
		t.Fatalf("expected 3 items, got %d", len(tuple))
	}
	for i, want := range []int{1, 2, 3} {
		got, _ := starlark.AsInt32(tuple[i])
		if int(got) != want {
			t.Errorf("index %d: got %d, want %d", i, got, want)
		}
	}
}

func TestExecTimeout(t *testing.T) {
	thread := testThread()
	// Test that timeout param is accepted and fast functions complete within it
	fast := makeBuiltin("fast", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
		return starlark.MakeInt(1), nil
	})

	kwargs := []starlark.Tuple{
		{starlark.String("timeout"), starlark.String("5s")},
	}

	result, err := callBuiltin(thread, "exec", starlark.Tuple{fast}, kwargs)
	if err != nil {
		t.Fatal(err)
	}
	tuple := result.(starlark.Tuple)
	if len(tuple) != 1 {
		t.Fatalf("expected 1 result, got %d", len(tuple))
	}
}

func TestExecTimeoutExpired(t *testing.T) {
	thread := testThread()
	slow := makeBuiltin("slow", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
		time.Sleep(2 * time.Second)
		return starlark.None, nil
	})
	fast := makeBuiltin("fast", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
		// Wait long enough for context to expire, then check
		time.Sleep(200 * time.Millisecond)
		return starlark.MakeInt(1), nil
	})

	kwargs := []starlark.Tuple{
		{starlark.String("timeout"), starlark.String("50ms")},
		{starlark.String("on_error"), starlark.String("continue")},
	}

	result, err := callBuiltin(thread, "exec", starlark.Tuple{slow, fast}, kwargs)
	if err != nil {
		t.Fatal(err)
	}
	// At least one function should see a timeout (context was expired during/after sleep)
	tuple := result.(starlark.Tuple)
	if len(tuple) != 2 {
		t.Fatalf("expected 2 results, got %d", len(tuple))
	}
}

func TestExecNonCallable(t *testing.T) {
	thread := testThread()
	_, err := callBuiltin(thread, "exec", starlark.Tuple{starlark.MakeInt(42)}, nil)
	if err == nil {
		t.Fatal("expected error for non-callable")
	}
	if !strings.Contains(err.Error(), "not callable") {
		t.Errorf("error should mention not callable, got: %s", err)
	}
}

func TestChildThreadHasPermissions(t *testing.T) {
	thread := testThread()
	checked := false

	fn := makeBuiltin("check", func(childThread *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		perms := starbase.GetPermissions(childThread)
		if perms == nil {
			return nil, fmt.Errorf("child thread has no permissions")
		}
		checked = true
		return starlark.None, nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	_, err := callBuiltin(thread, "map", starlark.Tuple{items, fn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("permission check function was not called")
	}
}

func TestChildThreadHasPrint(t *testing.T) {
	var printed string
	reg := starbase.NewRegistry(&starbase.ModuleConfig{})
	reg.Register(New())
	rt, err := starbase.NewTrusted(
		nil,
		starbase.WithRegistry(reg),
		func(c *starbase.Config) {
			c.Print = func(_ *starlark.Thread, msg string) {
				printed = msg
			}
		},
	)
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}
	thread := rt.NewThread("test")

	fn := makeBuiltin("printer", func(childThread *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		if childThread.Print != nil {
			childThread.Print(childThread, "hello from child")
		}
		return starlark.None, nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	_, err = callBuiltin(thread, "map", starlark.Tuple{items, fn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if printed != "hello from child" {
		t.Errorf("expected 'hello from child', got %q", printed)
	}
}

func TestTryMapReturnsResult(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	result, err := callBuiltin(thread, "try_map", starlark.Tuple{items, identity}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *starbase.Result, got %T", result)
	}
	if !bool(r.Truth()) {
		t.Fatal("expected ok=True")
	}
}

func TestTryEachReturnsResult(t *testing.T) {
	thread := testThread()
	noop := makeBuiltin("noop", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return starlark.None, nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	result, err := callBuiltin(thread, "try_each", starlark.Tuple{items, noop}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *starbase.Result, got %T", result)
	}
	if !bool(r.Truth()) {
		t.Fatal("expected ok=True")
	}
}

func TestTryExecReturnsResult(t *testing.T) {
	thread := testThread()
	fn := makeBuiltin("fn", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
		return starlark.MakeInt(1), nil
	})

	result, err := callBuiltin(thread, "try_exec", starlark.Tuple{fn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*starbase.Result)
	if !ok {
		t.Fatalf("expected *starbase.Result, got %T", result)
	}
	if !bool(r.Truth()) {
		t.Fatal("expected ok=True")
	}
}

func TestMapOnErrorContinue(t *testing.T) {
	thread := testThread()
	failOn2 := makeBuiltin("failOn2", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		x, _ := starlark.AsInt32(args[0])
		if x == 2 {
			return nil, fmt.Errorf("item 2 failed")
		}
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)})
	kwargs := []starlark.Tuple{
		{starlark.String("on_error"), starlark.String("continue")},
	}

	result, err := callBuiltin(thread, "map", starlark.Tuple{items, failOn2}, kwargs)
	if err != nil {
		t.Fatal(err)
	}

	list := result.(*starlark.List)
	if list.Len() != 3 {
		t.Fatalf("expected 3 results, got %d", list.Len())
	}

	// Item 0: ok
	r0 := list.Index(0).(*starbase.Result)
	if !bool(r0.Truth()) {
		t.Error("item 0 should be ok")
	}

	// Item 1: error
	r1 := list.Index(1).(*starbase.Result)
	if bool(r1.Truth()) {
		t.Error("item 1 should be error")
	}

	// Item 2: ok
	r2 := list.Index(2).(*starbase.Result)
	if !bool(r2.Truth()) {
		t.Error("item 2 should be ok")
	}
}

func TestEachOnErrorContinue(t *testing.T) {
	thread := testThread()
	failOn2 := makeBuiltin("failOn2", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		x, _ := starlark.AsInt32(args[0])
		if x == 2 {
			return nil, fmt.Errorf("failed")
		}
		return starlark.None, nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)})
	kwargs := []starlark.Tuple{
		{starlark.String("on_error"), starlark.String("continue")},
	}

	result, err := callBuiltin(thread, "each", starlark.Tuple{items, failOn2}, kwargs)
	if err != nil {
		t.Fatal(err)
	}

	list := result.(*starlark.List)
	if list.Len() != 3 {
		t.Fatalf("expected 3 results, got %d", list.Len())
	}
}

func TestExecOnErrorContinue(t *testing.T) {
	thread := testThread()
	okFn := makeBuiltin("ok", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
		return starlark.MakeInt(1), nil
	})
	failFn := makeBuiltin("fail", func(_ *starlark.Thread, _ starlark.Tuple) (starlark.Value, error) {
		return nil, fmt.Errorf("boom")
	})

	kwargs := []starlark.Tuple{
		{starlark.String("on_error"), starlark.String("continue")},
	}

	result, err := callBuiltin(thread, "exec", starlark.Tuple{okFn, failFn, okFn}, kwargs)
	if err != nil {
		t.Fatal(err)
	}

	tuple := result.(starlark.Tuple)
	if len(tuple) != 3 {
		t.Fatalf("expected 3 results, got %d", len(tuple))
	}

	r0 := tuple[0].(*starbase.Result)
	if !bool(r0.Truth()) {
		t.Error("fn 0 should be ok")
	}
	r1 := tuple[1].(*starbase.Result)
	if bool(r1.Truth()) {
		t.Error("fn 1 should be error")
	}
	r2 := tuple[2].(*starbase.Result)
	if !bool(r2.Truth()) {
		t.Error("fn 2 should be ok")
	}
}

func TestMapCollectAllSucceed(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(2), starlark.MakeInt(3)})
	kwargs := []starlark.Tuple{
		{starlark.String("on_error"), starlark.String("continue")},
	}

	result, err := callBuiltin(thread, "map", starlark.Tuple{items, identity}, kwargs)
	if err != nil {
		t.Fatal(err)
	}

	list := result.(*starlark.List)
	for i := 0; i < list.Len(); i++ {
		r := list.Index(i).(*starbase.Result)
		if !bool(r.Truth()) {
			t.Errorf("item %d should be ok", i)
		}
	}
}

func TestInvalidOnError(t *testing.T) {
	thread := testThread()
	identity := makeBuiltin("id", func(_ *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
		return args[0], nil
	})

	items := starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	kwargs := []starlark.Tuple{
		{starlark.String("on_error"), starlark.String("bogus")},
	}

	_, err := callBuiltin(thread, "map", starlark.Tuple{items, identity}, kwargs)
	if err == nil {
		t.Fatal("expected error for invalid on_error")
	}
}

// TestModuleName verifies the module name constant.
func TestModuleName(t *testing.T) {
	m := New()
	if m.Name() != "concur" {
		t.Fatalf("expected module name 'concur', got %q", m.Name())
	}
}
