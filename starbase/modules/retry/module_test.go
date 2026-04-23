package retry

import (
	"fmt"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

func testThread() *starlark.Thread {
	reg := starbase.NewRegistry(&starbase.ModuleConfig{})
	reg.Register(New())
	rt := starbase.NewTrusted(starbase.WithRegistry(reg))
	return rt.NewThread("test")
}

// makeFunc creates a Starlark builtin that returns the given value.
func makeFunc(val starlark.Value) starlark.Callable {
	return starlark.NewBuiltin("testfn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		return val, nil
	})
}

// makeCounterFunc creates a func that fails N times then succeeds.
func makeCounterFunc(failCount int, successVal starlark.Value) starlark.Callable {
	count := 0
	return starlark.NewBuiltin("testfn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		count++
		if count <= failCount {
			return starbase.ResultErr("attempt failed"), nil
		}
		return starbase.ResultOK(successVal), nil
	})
}

func asRetryResult(t *testing.T, val starlark.Value) *starbase.RetryResult {
	t.Helper()
	rr, ok := val.(*starbase.RetryResult)
	if !ok {
		t.Fatalf("expected *RetryResult, got %T (%v)", val, val)
	}
	return rr
}

func attrInt(t *testing.T, rr *starbase.RetryResult, name string) int {
	t.Helper()
	v, _ := rr.Attr(name)
	i, _ := v.(starlark.Int).Int64()
	return int(i)
}

func TestDoSuccessFirstAttempt(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starbase.ResultOK(starlark.String("ok")))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{{starlark.String("delay"), starlark.String("1ms")}})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	if attrInt(t, rr, "attempts") != 1 {
		t.Fatalf("attempts = %d, want 1", attrInt(t, rr, "attempts"))
	}
}

func TestDoSuccessAfterRetries(t *testing.T) {
	thread := testThread()
	fn := makeCounterFunc(2, starlark.String("done"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(5)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	if attrInt(t, rr, "attempts") != 3 {
		t.Fatalf("attempts = %d, want 3", attrInt(t, rr, "attempts"))
	}
	v, _ := rr.Attr("value")
	if v.(starlark.String) != "done" {
		t.Fatalf("value = %v, want done", v)
	}
}

func TestDoAllAttemptsFail(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starbase.ResultErr("always fails"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(3)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.False {
		t.Fatal("expected ok=False")
	}
	if attrInt(t, rr, "attempts") != 3 {
		t.Fatalf("attempts = %d, want 3", attrInt(t, rr, "attempts"))
	}
	errors, _ := rr.Attr("errors")
	errList := errors.(*starlark.List)
	if errList.Len() != 3 {
		t.Fatalf("errors len = %d, want 3", errList.Len())
	}
}

func TestDoWithDelay(t *testing.T) {
	thread := testThread()
	fn := makeCounterFunc(2, starlark.String("ok"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	start := time.Now()
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(3)},
			{starlark.String("delay"), starlark.String("20ms")},
		})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	// Should have slept at least 2 * 20ms = 40ms
	if elapsed < 35*time.Millisecond {
		t.Fatalf("elapsed = %v, expected >= 35ms", elapsed)
	}
}

func TestDoMaxAttempts(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starbase.ResultErr("fail"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(5)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if attrInt(t, rr, "attempts") != 5 {
		t.Fatalf("attempts = %d, want 5", attrInt(t, rr, "attempts"))
	}
}

func TestDoRetryOnPredicate(t *testing.T) {
	thread := testThread()
	callCount := 0
	fn := starlark.NewBuiltin("testfn", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		callCount++
		if callCount <= 2 {
			return starbase.ResultOK(starlark.String("not ready")), nil
		}
		return starbase.ResultOK(starlark.String("ready")), nil
	})

	// retry_on returns True if value is "not ready" (keep retrying)
	retryOn := starlark.NewBuiltin("retry_on", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		r := args[0].(*starbase.Result)
		v, _ := r.Attr("value")
		if v.(starlark.String) == "not ready" {
			return starlark.True, nil
		}
		return starlark.False, nil
	})

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(5)},
			{starlark.String("delay"), starlark.String("1ms")},
			{starlark.String("retry_on"), retryOn},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	if attrInt(t, rr, "attempts") != 3 {
		t.Fatalf("attempts = %d, want 3", attrInt(t, rr, "attempts"))
	}
	v, _ := rr.Attr("value")
	if v.(starlark.String) != "ready" {
		t.Fatalf("value = %v, want ready", v)
	}
}

func TestDoOnRetryCallback(t *testing.T) {
	thread := testThread()
	fn := makeCounterFunc(2, starlark.String("ok"))

	var retryArgs []string
	onRetry := starlark.NewBuiltin("on_retry", func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		attempt := args[0].(starlark.Int)
		errMsg := args[1].(starlark.String)
		retryArgs = append(retryArgs, fmt.Sprintf("%s:%s", attempt, errMsg))
		return starlark.None, nil
	})

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(5)},
			{starlark.String("delay"), starlark.String("1ms")},
			{starlark.String("on_retry"), onRetry},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	if len(retryArgs) != 2 {
		t.Fatalf("on_retry called %d times, want 2", len(retryArgs))
	}
}

func TestDoTimeout(t *testing.T) {
	thread := testThread()
	// Function that always sleeps longer than timeout
	fn := starlark.NewBuiltin("slow", func(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
		time.Sleep(100 * time.Millisecond)
		return starbase.ResultErr("still failing"), nil
	})

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(10)},
			{starlark.String("delay"), starlark.String("50ms")},
			{starlark.String("timeout"), starlark.String("200ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.False {
		t.Fatal("expected ok=False due to timeout")
	}
	if attrInt(t, rr, "attempts") >= 10 {
		t.Fatalf("should have timed out before all attempts, got %d", attrInt(t, rr, "attempts"))
	}
}

func TestDoNoneReturnSuccess(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starlark.None)

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{{starlark.String("delay"), starlark.String("1ms")}})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("None should be treated as success")
	}
	if attrInt(t, rr, "attempts") != 1 {
		t.Fatalf("attempts = %d, want 1", attrInt(t, rr, "attempts"))
	}
}

func TestDoBoolReturnTrue(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starlark.True)

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{{starlark.String("delay"), starlark.String("1ms")}})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("True should be success")
	}
}

func TestDoBoolReturnFalse(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starlark.False)

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(2)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.False {
		t.Fatal("False should mean failure, all retries should fail")
	}
	if attrInt(t, rr, "attempts") != 2 {
		t.Fatalf("attempts = %d, want 2", attrInt(t, rr, "attempts"))
	}
}

func TestDoNonResultReturn(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starlark.String("hello"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(5)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("non-Result return should be treated as success (execute once)")
	}
	if attrInt(t, rr, "attempts") != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry for non-Result)", attrInt(t, rr, "attempts"))
	}
	v, _ := rr.Attr("value")
	if v.(starlark.String) != "hello" {
		t.Fatalf("value = %v, want hello", v)
	}
}

func TestDoResultReturn(t *testing.T) {
	thread := testThread()
	fn := makeFunc(starbase.ResultOK(starlark.MakeInt(42)))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{{starlark.String("delay"), starlark.String("1ms")}})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	v, _ := rr.Attr("value")
	if v.(starlark.Int) != starlark.MakeInt(42) {
		t.Fatalf("value = %v, want 42", v)
	}
}

func TestWithBackoffDelayDoubles(t *testing.T) {
	thread := testThread()
	fn := makeCounterFunc(3, starlark.String("ok"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	start := time.Now()
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.with_backoff", mod.retryWithBackoff),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(5)},
			{starlark.String("delay"), starlark.String("20ms")},
			{starlark.String("max_delay"), starlark.String("1s")},
			{starlark.String("jitter"), starlark.False},
		})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	// 3 failures: delays of 20ms, 40ms, 80ms = 140ms minimum
	if elapsed < 100*time.Millisecond {
		t.Fatalf("elapsed = %v, expected >= 100ms for backoff", elapsed)
	}
}

func TestWithBackoffMaxDelay(t *testing.T) {
	thread := testThread()
	fn := makeCounterFunc(4, starlark.String("ok"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	start := time.Now()
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.with_backoff", mod.retryWithBackoff),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(6)},
			{starlark.String("delay"), starlark.String("20ms")},
			{starlark.String("max_delay"), starlark.String("50ms")},
			{starlark.String("jitter"), starlark.False},
		})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
	// 4 failures: delays of 20ms, 40ms, 50ms (capped), 50ms (capped) = 160ms
	// Without cap it would be 20+40+80+160 = 300ms
	if elapsed > 300*time.Millisecond {
		t.Fatalf("elapsed = %v, max_delay cap should limit total time", elapsed)
	}
}

func TestWithBackoffJitterDefault(t *testing.T) {
	// Just verify with_backoff doesn't error with default jitter=true
	thread := testThread()
	fn := makeFunc(starbase.ResultOK(starlark.String("ok")))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.with_backoff", mod.retryWithBackoff),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("expected ok=True")
	}
}

func TestRetryResultAttributes(t *testing.T) {
	thread := testThread()
	fn := makeCounterFunc(1, starlark.String("val"))

	mod := New()
	mod.Load(&starbase.ModuleConfig{})
	val, err := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(3)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	if err != nil {
		t.Fatal(err)
	}

	rr := asRetryResult(t, val)

	// Check all attributes exist
	for _, name := range []string{"ok", "value", "error", "attempts", "elapsed", "errors"} {
		v, attrErr := rr.Attr(name)
		if attrErr != nil {
			t.Fatalf("Attr(%q) error: %v", name, attrErr)
		}
		if v == nil {
			t.Fatalf("Attr(%q) = nil", name)
		}
	}
}

func TestRetryResultTruth(t *testing.T) {
	thread := testThread()

	mod := New()
	mod.Load(&starbase.ModuleConfig{})

	// ok result
	fn := makeFunc(starbase.ResultOK(starlark.None))
	val, _ := starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{{starlark.String("delay"), starlark.String("1ms")}})
	rr := asRetryResult(t, val)
	if rr.Truth() != starlark.True {
		t.Fatal("ok RetryResult should be truthy")
	}

	// fail result
	fn = makeFunc(starbase.ResultErr("fail"))
	val, _ = starlark.Call(thread, starlark.NewBuiltin("retry.do", mod.retryDo),
		starlark.Tuple{fn}, []starlark.Tuple{
			{starlark.String("max_attempts"), starlark.MakeInt(1)},
			{starlark.String("delay"), starlark.String("1ms")},
		})
	rr = asRetryResult(t, val)
	if rr.Truth() != starlark.False {
		t.Fatal("failed RetryResult should be falsy")
	}
}

func TestRetryResultFreeze(t *testing.T) {
	rr := starbase.NewRetryResult(true, starlark.String("ok"), "", 1, time.Millisecond, nil)
	rr.Freeze() // should not panic
}
