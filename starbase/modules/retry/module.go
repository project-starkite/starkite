// Package retry provides retry functionality for starkite.
package retry

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/starbase"
)

const ModuleName starbase.ModuleName = "retry"

// Module implements retry functionality.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "retry provides retry functionality: do, with_backoff"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"do":           starlark.NewBuiltin("retry.do", m.retryDo),
				"with_backoff": starlark.NewBuiltin("retry.with_backoff", m.retryWithBackoff),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// evaluateResult determines if a return value represents success and whether retrying makes sense.
// Returns (ok, errMsg, shouldRetry).
func evaluateResult(val starlark.Value) (bool, string, bool) {
	switch v := val.(type) {
	case *starbase.RetryResult:
		return bool(v.Truth()), v.ErrorString(), true
	case *starbase.Result:
		return bool(v.Truth()), v.ErrorString(), true
	case starlark.Bool:
		if v {
			return true, "", true
		}
		return false, "returned False", true
	default:
		if val == starlark.None {
			return true, "", true
		}
		// Non-evaluatable type: execute once, return as-is
		return true, "", false
	}
}

// resultValue extracts .value from Result/RetryResult, returns raw value otherwise.
func resultValue(val starlark.Value) starlark.Value {
	switch v := val.(type) {
	case *starbase.RetryResult:
		attr, _ := v.Attr("value")
		return attr
	case *starbase.Result:
		attr, _ := v.Attr("value")
		return attr
	default:
		return val
	}
}

// toCallable asserts a starlark.Value is callable (or nil/None).
func toCallable(v starlark.Value, name string) (starlark.Callable, error) {
	if v == nil || v == starlark.None {
		return nil, nil
	}
	c, ok := v.(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("%s: expected callable, got %s", name, v.Type())
	}
	return c, nil
}

// applyJitter adds ±25% randomization to a delay.
func applyJitter(d time.Duration) time.Duration {
	factor := 1.0 + (rand.Float64()*0.5 - 0.25) // 0.75 to 1.25
	return time.Duration(float64(d) * factor)
}

// retryDo retries a function with fixed delay until it succeeds or max attempts reached.
// Usage: retry.do(func, max_attempts=3, delay="1s", retry_on=callable, on_retry=callable, timeout="60s", jitter=False)
func (m *Module) retryDo(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Func        starlark.Value `name:"func" position:"0" required:"true"`
		MaxAttempts int            `name:"max_attempts"`
		Delay       string         `name:"delay"`
		RetryOn     starlark.Value `name:"retry_on"`
		OnRetry     starlark.Value `name:"on_retry"`
		Timeout     string         `name:"timeout"`
		Jitter      bool           `name:"jitter"`
	}
	p.MaxAttempts = 3
	p.Delay = "1s"
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	callFn, err := toCallable(p.Func, "func")
	if err != nil {
		return nil, err
	}
	retryOnFn, err := toCallable(p.RetryOn, "retry_on")
	if err != nil {
		return nil, err
	}
	onRetryFn, err := toCallable(p.OnRetry, "on_retry")
	if err != nil {
		return nil, err
	}

	delay, err := time.ParseDuration(p.Delay)
	if err != nil {
		return nil, fmt.Errorf("invalid delay: %w", err)
	}

	return m.executeRetry(thread, callFn, retryOnFn, onRetryFn, p.MaxAttempts, delay, 0, p.Timeout, p.Jitter)
}

// retryWithBackoff retries a function with exponential backoff.
// Usage: retry.with_backoff(func, max_attempts=5, delay="500ms", max_delay="30s", ...)
func (m *Module) retryWithBackoff(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Func        starlark.Value `name:"func" position:"0" required:"true"`
		MaxAttempts int            `name:"max_attempts"`
		Delay       string         `name:"delay"`
		MaxDelay    string         `name:"max_delay"`
		RetryOn     starlark.Value `name:"retry_on"`
		OnRetry     starlark.Value `name:"on_retry"`
		Timeout     string         `name:"timeout"`
		Jitter      bool           `name:"jitter"`
	}
	p.MaxAttempts = 5
	p.Delay = "500ms"
	p.MaxDelay = "30s"
	p.Jitter = true
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	callFn, err := toCallable(p.Func, "func")
	if err != nil {
		return nil, err
	}
	retryOnFn, err := toCallable(p.RetryOn, "retry_on")
	if err != nil {
		return nil, err
	}
	onRetryFn, err := toCallable(p.OnRetry, "on_retry")
	if err != nil {
		return nil, err
	}

	delay, err := time.ParseDuration(p.Delay)
	if err != nil {
		return nil, fmt.Errorf("invalid delay: %w", err)
	}

	maxDelay, err := time.ParseDuration(p.MaxDelay)
	if err != nil {
		return nil, fmt.Errorf("invalid max_delay: %w", err)
	}

	return m.executeRetry(thread, callFn, retryOnFn, onRetryFn, p.MaxAttempts, delay, maxDelay, p.Timeout, p.Jitter)
}

// executeRetry is the core retry loop shared by retryDo and retryWithBackoff.
// When maxDelay > 0, exponential backoff is used.
func (m *Module) executeRetry(
	thread *starlark.Thread,
	callFn, retryOnFn, onRetryFn starlark.Callable,
	maxAttempts int,
	delay, maxDelay time.Duration,
	timeoutStr string,
	jitter bool,
) (starlark.Value, error) {
	start := time.Now()

	var ctx context.Context
	var cancel context.CancelFunc
	if timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	var errors []string
	currentDelay := delay
	backoff := maxDelay > 0

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check timeout
		select {
		case <-ctx.Done():
			if len(errors) == 0 {
				errors = append(errors, "timeout exceeded")
			}
			return starbase.NewRetryResult(false, starlark.None, "timeout exceeded", attempt-1, time.Since(start), errors), nil
		default:
		}

		val, callErr := starlark.Call(thread, callFn, nil, nil)
		if callErr != nil {
			// Go-level error (Starlark evaluation error)
			errors = append(errors, callErr.Error())
			if attempt < maxAttempts {
				if onRetryFn != nil {
					starlark.Call(thread, onRetryFn, starlark.Tuple{starlark.MakeInt(attempt), starlark.String(callErr.Error())}, nil)
				}
				m.sleepWithJitter(ctx, currentDelay, jitter)
				if backoff {
					currentDelay *= 2
					if currentDelay > maxDelay {
						currentDelay = maxDelay
					}
				}
			}
			continue
		}

		ok, errMsg, shouldRetry := evaluateResult(val)
		if !shouldRetry {
			// Non-retryable type — execute once, wrap and return
			return starbase.NewRetryResult(true, val, "", 1, time.Since(start), nil), nil
		}

		if ok {
			// Check retry_on predicate if set
			if retryOnFn != nil {
				predResult, predErr := starlark.Call(thread, retryOnFn, starlark.Tuple{val}, nil)
				if predErr == nil && predResult == starlark.True {
					errors = append(errors, "retry_on predicate triggered")
					if attempt < maxAttempts {
						if onRetryFn != nil {
							starlark.Call(thread, onRetryFn, starlark.Tuple{starlark.MakeInt(attempt), starlark.String("retry_on predicate triggered")}, nil)
						}
						m.sleepWithJitter(ctx, currentDelay, jitter)
						if backoff {
							currentDelay *= 2
							if currentDelay > maxDelay {
								currentDelay = maxDelay
							}
						}
					}
					continue
				}
			}
			// Success
			return starbase.NewRetryResult(true, resultValue(val), "", attempt, time.Since(start), errors), nil
		}

		// Failure
		errors = append(errors, errMsg)
		if attempt < maxAttempts {
			if onRetryFn != nil {
				starlark.Call(thread, onRetryFn, starlark.Tuple{starlark.MakeInt(attempt), starlark.String(errMsg)}, nil)
			}
			m.sleepWithJitter(ctx, currentDelay, jitter)
			if backoff {
				currentDelay *= 2
				if currentDelay > maxDelay {
					currentDelay = maxDelay
				}
			}
		}
	}

	// All attempts exhausted
	lastErr := "all attempts failed"
	if len(errors) > 0 {
		lastErr = errors[len(errors)-1]
	}
	return starbase.NewRetryResult(false, starlark.None, lastErr, maxAttempts, time.Since(start), errors), nil
}

// sleepWithJitter sleeps for the given duration, optionally applying ±25% jitter.
// Respects context cancellation.
func (m *Module) sleepWithJitter(ctx context.Context, d time.Duration, jitter bool) {
	if jitter {
		d = applyJitter(d)
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
