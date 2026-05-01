package libkite

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.starlark.net/starlark"
)

func newTrustedRT(t *testing.T) *Runtime {
	t.Helper()
	rt, err := NewTrusted(nil)
	if err != nil {
		t.Fatalf("NewTrusted error: %v", err)
	}
	t.Cleanup(rt.Close)
	return rt
}

func TestRuntime_Call_InvokesDefinedFunction(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def add(a, b): return a + b`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	v, err := rt.Call(context.Background(), "add", nil, map[string]any{"a": 2, "b": 3})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	got, ok := v.(starlark.Int)
	if !ok {
		t.Fatalf("return type = %T, want starlark.Int", v)
	}
	if n, _ := got.Int64(); n != 5 {
		t.Errorf("got %d, want 5", n)
	}
}

func TestRuntime_Call_UndefinedName_Errors(t *testing.T) {
	rt := newTrustedRT(t)

	_, err := rt.Call(context.Background(), "missing_fn", nil, nil)
	if err == nil {
		t.Fatal("want error for undefined name, got nil")
	}
	if !strings.Contains(err.Error(), "missing_fn") {
		t.Errorf("error %q does not mention name", err.Error())
	}
}

func TestRuntime_Call_NotCallable_Errors(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `x = 42`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	_, err := rt.Call(context.Background(), "x", nil, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "not callable") {
		t.Errorf("error %q does not say 'not callable'", err.Error())
	}
}

func TestRuntime_Call_PropagatesStarlarkError(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `
def boom():
    fail("boom")
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	_, err := rt.Call(context.Background(), "boom", nil, nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q does not contain 'boom'", err.Error())
	}
}

func TestRuntime_Call_KwargConversion(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `
def echo(s, n, f, b, xs, obj):
    return {"s": s, "n": n, "f": f, "b": b, "xs": xs, "obj": obj}
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	v, err := rt.Call(context.Background(), "echo", nil, map[string]any{
		"s":   "hi",
		"n":   int64(7),
		"f":   3.5,
		"b":   true,
		"xs":  []any{int64(1), int64(2), "x"},
		"obj": map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	d, ok := v.(*starlark.Dict)
	if !ok {
		t.Fatalf("return type = %T, want *starlark.Dict", v)
	}
	if d.Len() != 6 {
		t.Errorf("dict len = %d, want 6", d.Len())
	}
}

func TestRuntime_Call_ConcurrentCalls(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def sq(n): return n * n`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	const workers = 4
	const iters = 100
	var wg sync.WaitGroup
	var fail atomic.Int32
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				v, err := rt.Call(context.Background(), "sq", nil, map[string]any{"n": int64(i)})
				if err != nil {
					fail.Add(1)
					return
				}
				n, _ := v.(starlark.Int).Int64()
				if n != int64(i*i) {
					fail.Add(1)
					return
				}
			}
		}()
	}
	wg.Wait()
	if fail.Load() != 0 {
		t.Fatalf("%d concurrent failures", fail.Load())
	}
}

func TestRuntime_Call_ContextCancel_StopsRunaway(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `
def spin():
    total = 0
    for i in range(10000000000):
        total += 1
    return total
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := rt.Call(ctx, "spin", nil, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want cancel error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %s to cancel, expected well under 1s", elapsed)
	}
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error %q does not mention deadline/canceled", err.Error())
	}
}

func TestRuntime_Call_ContextAlreadyCanceled(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def one(): return 1`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Call(ctx, "one", nil, nil)
	if err == nil {
		t.Fatal("want cancel error, got nil")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Errorf("error %q does not mention canceled", err.Error())
	}
}

func TestRuntime_Call_NilContext_Allowed(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def one(): return 1`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	// nil ctx: no cancel wiring; watcher goroutine must not start.
	before := runtime.NumGoroutine()
	for i := 0; i < 20; i++ {
		if _, err := rt.Call(nil, "one", nil, nil); err != nil { //nolint:staticcheck // intentional nil
			t.Fatalf("Call(nil): %v", err)
		}
	}
	// Allow a tiny grace period for any stragglers.
	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Errorf("goroutines grew: before=%d after=%d", before, after)
	}
}

func TestRuntime_Call_ContextGoroutineCleanup(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def nop(): return 0`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	before := runtime.NumGoroutine()
	for i := 0; i < 500; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		if _, err := rt.Call(ctx, "nop", nil, nil); err != nil {
			cancel()
			t.Fatalf("Call: %v", err)
		}
		cancel()
	}
	time.Sleep(20 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+5 {
		t.Errorf("goroutine leak: before=%d after=%d", before, after)
	}
}

func TestRuntime_GetGlobalVal_ReturnsDefinedValue(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `x = 7`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	v, ok := rt.GetGlobalVal("x")
	if !ok {
		t.Fatal("GetGlobalVal(x) not found")
	}
	got, ok := v.(starlark.Int)
	if !ok {
		t.Fatalf("type = %T", v)
	}
	if n, _ := got.Int64(); n != 7 {
		t.Errorf("got %d, want 7", n)
	}
}

func TestRuntime_GetGlobalVal_UndefinedReturnsFalse(t *testing.T) {
	rt := newTrustedRT(t)
	if _, ok := rt.GetGlobalVal("nope"); ok {
		t.Error("GetGlobalVal returned ok=true for undefined name")
	}
}

func TestRuntime_ExecuteRepl_ConcurrentWithCall_NoRace(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def seed(): return 1`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = rt.ExecuteRepl(context.Background(), fmt.Sprintf(`def f%d(): return %d`, i, i))
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = rt.Call(context.Background(), "seed", nil, nil)
			_, _ = rt.GetGlobalVal("seed")
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
	// Success = no panic / no race report under -race.
}

func TestRuntime_Eval_ExpressionReturnsValue(t *testing.T) {
	rt := newTrustedRT(t)
	v, err := rt.Eval(context.Background(), "1 + 2")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	got, ok := v.(starlark.Int)
	if !ok {
		t.Fatalf("type = %T", v)
	}
	if n, _ := got.Int64(); n != 3 {
		t.Errorf("got %d, want 3", n)
	}
}

func TestRuntime_Eval_SeesDefinedGlobal(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def add(a, b): return a + b`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}
	v, err := rt.Eval(context.Background(), "add(1, 2)")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if n, _ := v.(starlark.Int).Int64(); n != 3 {
		t.Errorf("got %d, want 3", n)
	}
}

func TestRuntime_Eval_StatementErrors(t *testing.T) {
	rt := newTrustedRT(t)
	_, err := rt.Eval(context.Background(), "x = 1")
	if err == nil {
		t.Fatal("want error for statement, got nil")
	}
}

func TestRuntime_Eval_SyntaxErrorPropagates(t *testing.T) {
	rt := newTrustedRT(t)
	_, err := rt.Eval(context.Background(), "1 +")
	if err == nil {
		t.Fatal("want parse error, got nil")
	}
}

func TestRuntime_Eval_ContextCancel_StopsRunaway(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `
def spin():
    total = 0
    for i in range(10000000000):
        total += 1
    return total
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := rt.Eval(ctx, "spin()")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want cancel error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %s, expected well under 1s", elapsed)
	}
}

// Ensure Unwrap chain includes a Starlark EvalError, not just a plain error.
// Sanity check for callers who want to distinguish types.
func TestRuntime_Call_PropagatesEvalErrorType(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `
def throw():
    fail("nope")
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	_, err := rt.Call(context.Background(), "throw", nil, nil)
	if err == nil {
		t.Fatal("want error")
	}
	var evalErr *starlark.EvalError
	if !errors.As(err, &evalErr) {
		t.Errorf("err not *starlark.EvalError chain: %T %v", err, err)
	}
}

func TestRuntime_Call_PositionalArgs(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def mul(a, b): return a * b`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	v, err := rt.Call(context.Background(), "mul", []any{int64(4), int64(6)}, nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if n, _ := v.(starlark.Int).Int64(); n != 24 {
		t.Errorf("got %d, want 24", n)
	}
}

func TestRuntime_Call_MixedArgsAndKwargs(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def mix(a, b, c=0): return a + b + c`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}

	v, err := rt.Call(context.Background(), "mix",
		[]any{int64(1), int64(2)},
		map[string]any{"c": int64(10)})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if n, _ := v.(starlark.Int).Int64(); n != 13 {
		t.Errorf("got %d, want 13", n)
	}
}

// ---------- Phase 5.1: ctx unification + CallFn + exit unwrap ----------

func TestRuntime_Execute_ContextCancel_StopsRunaway(t *testing.T) {
	rt := newTrustedRT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := rt.Execute(ctx, `
for i in range(10000000000):
    pass
`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want cancel error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %s to cancel, expected well under 1s", elapsed)
	}
}

func TestRuntime_Execute_ExitError(t *testing.T) {
	rt := newTrustedRT(t)
	err := rt.Execute(context.Background(), `exit(7)`)
	if err == nil {
		t.Fatal("want ExitError")
	}
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err is not *ExitError: %T %v", err, err)
	}
	if ee.Code != 7 {
		t.Errorf("code = %d, want 7", ee.Code)
	}
}

func TestRuntime_Execute_ExitZeroReturnsNil(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.Execute(context.Background(), `exit(0)`); err != nil {
		t.Fatalf("want nil for exit(0), got %v", err)
	}
}

func TestRuntime_ExecuteRepl_ContextCancel_StopsRunaway(t *testing.T) {
	rt := newTrustedRT(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := rt.ExecuteRepl(ctx, `
total = 0
for i in range(10000000000):
    total += 1
`)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want cancel error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %s to cancel, expected well under 1s", elapsed)
	}
}

func TestRuntime_ExecuteRepl_ExitError(t *testing.T) {
	rt := newTrustedRT(t)
	err := rt.ExecuteRepl(context.Background(), `exit(3)`)
	if err == nil {
		t.Fatal("want ExitError")
	}
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err is not *ExitError: %T %v", err, err)
	}
	if ee.Code != 3 {
		t.Errorf("code = %d, want 3", ee.Code)
	}
}

func TestRuntime_ExecuteTests_ScriptLevelExit(t *testing.T) {
	rt := newTrustedRT(t)
	results, err := rt.ExecuteTests(context.Background(), `
exit(4)
def test_a(): pass
`)
	if err == nil {
		t.Fatal("want ExitError at module level")
	}
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("err is not *ExitError: %T %v", err, err)
	}
	if ee.Code != 4 {
		t.Errorf("code = %d, want 4", ee.Code)
	}
	if len(results) != 0 {
		t.Errorf("want no results, got %d", len(results))
	}
}

func TestRuntime_ExecuteTests_TestLevelExit(t *testing.T) {
	rt := newTrustedRT(t)
	results, err := rt.ExecuteTests(context.Background(), `
def test_a(): pass
def test_b(): exit(5)
def test_c(): pass
`)
	if err != nil {
		t.Fatalf("ExecuteTests: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// test_a passes
	if !results[0].Passed {
		t.Errorf("test_a should pass: %v", results[0].Error)
	}
	// test_b fails with exit error
	if results[1].Passed {
		t.Errorf("test_b should fail")
	}
	if !strings.Contains(results[1].Error.Error(), "exit(5)") {
		t.Errorf("test_b error does not mention exit(5): %v", results[1].Error)
	}
	var ee *ExitError
	if !errors.As(results[1].Error, &ee) || ee.Code != 5 {
		t.Errorf("test_b error chain missing *ExitError{Code:5}: %v", results[1].Error)
	}
	// test_c still runs and passes
	if !results[2].Passed {
		t.Errorf("test_c should pass: %v", results[2].Error)
	}
}

func TestRuntime_CallFn_InvokesCallable(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `def add(a, b): return a + b`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}
	fnVal, ok := rt.GetGlobalVal("add")
	if !ok {
		t.Fatal("add not found")
	}
	fn, ok := fnVal.(starlark.Callable)
	if !ok {
		t.Fatalf("add is not Callable: %T", fnVal)
	}

	v, err := rt.CallFn(context.Background(), fn, []any{int64(2), int64(3)}, nil)
	if err != nil {
		t.Fatalf("CallFn: %v", err)
	}
	if n, _ := v.(starlark.Int).Int64(); n != 5 {
		t.Errorf("got %d, want 5", n)
	}
}

func TestRuntime_CallFn_NilFnErrors(t *testing.T) {
	rt := newTrustedRT(t)
	_, err := rt.CallFn(context.Background(), nil, nil, nil)
	if err == nil {
		t.Fatal("want error for nil fn")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error %q does not mention nil", err.Error())
	}
}

func TestRuntime_CallFn_ContextCancel_StopsRunaway(t *testing.T) {
	rt := newTrustedRT(t)
	if err := rt.ExecuteRepl(context.Background(), `
def spin():
    total = 0
    for i in range(10000000000):
        total += 1
    return total
`); err != nil {
		t.Fatalf("ExecuteRepl: %v", err)
	}
	fnVal, _ := rt.GetGlobalVal("spin")
	fn := fnVal.(starlark.Callable)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := rt.CallFn(ctx, fn, nil, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("want cancel error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %s to cancel", elapsed)
	}
}
