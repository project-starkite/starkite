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

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "retry"

// Module implements retry functionality.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "retry provides retry functionality: do, with_backoff"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = libkite.NewTryModule(
			string(ModuleName),
			starlark.StringDict{
				"do":           starlark.NewBuiltin("retry.do", m.retryDo),
				"with_backoff": starlark.NewBuiltin("retry.with_backoff", m.retryWithBackoff),
			},
		)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// evaluateResult determines if a return value represents success and whether retrying makes sense.
// Returns (ok, errMsg, shouldRetry).
func evaluateResult(val starlark.Value) (bool, string, bool) {
	switch v := val.(type) {
	case *libkite.RetryResult:
		return bool(v.Truth()), v.ErrorString(), true
	case *libkite.Result:
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
	case *libkite.RetryResult:
		attr, _ := v.Attr("value")
		return attr
	case *libkite.Result:
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

	return m.executeRetry(thread, callFn, retryOnFn, onRetryFn, p.MaxAttempts, delay, 0, 1.0, p.Timeout, p.Jitter)
}

// retryWithBackoff retries a function with exponential backoff.
// Usage: retry.with_backoff(func, max_attempts=5, delay="500ms", max_delay="30s", backoff_factor=2.0, ...)
func (m *Module) retryWithBackoff(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Func          starlark.Value `name:"func" position:"0" required:"true"`
		MaxAttempts   int            `name:"max_attempts"`
		Delay         string         `name:"delay"`
		InitialDelay  string         `name:"initial_delay"`
		MaxDelay      string         `name:"max_delay"`
		BackoffFactor float64        `name:"backoff_factor"`
		RetryOn       starlark.Value `name:"retry_on"`
		OnRetry       starlark.Value `name:"on_retry"`
		Timeout       string         `name:"timeout"`
		Jitter        bool           `name:"jitter"`
	}
	p.MaxAttempts = 5
	p.Delay = "500ms"
	p.MaxDelay = "30s"
	p.BackoffFactor = 2.0
	p.Jitter = true
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if p.BackoffFactor < 1.0 {
		return nil, fmt.Errorf("retry.with_backoff: backoff_factor must be >= 1.0, got %v", p.BackoffFactor)
	}
	if p.InitialDelay != "" {
		p.Delay = p.InitialDelay
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

	return m.executeRetry(thread, callFn, retryOnFn, onRetryFn, p.MaxAttempts, delay, maxDelay, p.BackoffFactor, p.Timeout, p.Jitter)
}

// executeRetry is the core retry loop shared by retryDo and retryWithBackoff.
// When maxDelay > 0, exponential backoff with backoffFactor is used.
func (m *Module) executeRetry(
	thread *starlark.Thread,
	callFn, retryOnFn, onRetryFn starlark.Callable,
	maxAttempts int,
	delay, maxDelay time.Duration,
	backoffFactor float64,
	timeoutStr string,
	jitter bool,
) (starlark.Value, error) {
	start := time.Now()

	parentCtx := libkite.ThreadContext(thread)
	ctx := parentCtx
	var cancel context.CancelFunc
	if timeoutStr != "" {
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		ctx, cancel = context.WithTimeout(parentCtx, timeout)
		defer cancel()
		libkite.SetThreadContext(thread, ctx)
		defer libkite.SetThreadContext(thread, parentCtx)
	}

	var errors []string
	currentDelay := delay

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check timeout
		select {
		case <-ctx.Done():
			if len(errors) == 0 {
				errors = append(errors, "timeout exceeded")
			}
			return libkite.NewRetryResult(false, starlark.None, "timeout exceeded", attempt-1, time.Since(start), errors), nil
		default:
		}

		val, callErr := starlark.Call(thread, callFn, nil, nil)
		if callErr != nil {
			// Go-level error (Starlark evaluation error)
			errors = append(errors, callErr.Error())
			if attempt < maxAttempts {
				currentDelay = m.waitNextAttempt(ctx, thread, onRetryFn, attempt, callErr.Error(), currentDelay, maxDelay, backoffFactor, jitter)
			}
			continue
		}

		ok, errMsg, shouldRetry := evaluateResult(val)
		if !shouldRetry {
			// Non-retryable type — execute once, wrap and return
			return libkite.NewRetryResult(true, val, "", 1, time.Since(start), nil), nil
		}

		if ok {
			// Check retry_on predicate if set
			if retryOnFn != nil {
				predResult, predErr := starlark.Call(thread, retryOnFn, starlark.Tuple{val}, nil)
				if predErr == nil && predResult == starlark.True {
					errors = append(errors, "retry_on predicate triggered")
					if attempt < maxAttempts {
						currentDelay = m.waitNextAttempt(ctx, thread, onRetryFn, attempt, "retry_on predicate triggered", currentDelay, maxDelay, backoffFactor, jitter)
					}
					continue
				}
			}
			// Success
			return libkite.NewRetryResult(true, resultValue(val), "", attempt, time.Since(start), errors), nil
		}

		// Failure
		errors = append(errors, errMsg)
		if attempt < maxAttempts {
			currentDelay = m.waitNextAttempt(ctx, thread, onRetryFn, attempt, errMsg, currentDelay, maxDelay, backoffFactor, jitter)
		}
	}

	// All attempts exhausted
	lastErr := "all attempts failed"
	if len(errors) > 0 {
		lastErr = errors[len(errors)-1]
	}
	return libkite.NewRetryResult(false, starlark.None, lastErr, maxAttempts, time.Since(start), errors), nil
}

// waitNextAttempt invokes onRetry if non-nil, sleeps with optional jitter, and computes the next delay.
func (m *Module) waitNextAttempt(
	ctx context.Context,
	thread *starlark.Thread,
	onRetryFn starlark.Callable,
	attempt int,
	errMsg string,
	currentDelay, maxDelay time.Duration,
	backoffFactor float64,
	jitter bool,
) time.Duration {
	if onRetryFn != nil {
		starlark.Call(thread, onRetryFn, starlark.Tuple{starlark.MakeInt(attempt), starlark.String(errMsg)}, nil)
	}
	m.sleepWithJitter(ctx, currentDelay, jitter)
	if maxDelay > 0 {
		currentDelay = min(time.Duration(float64(currentDelay)*backoffFactor), maxDelay)
	}
	return currentDelay
}

// sleepWithJitter sleeps for the given duration, optionally applying ±25% jitter.
// Respects context cancellation.
func (m *Module) sleepWithJitter(ctx context.Context, d time.Duration, jitter bool) {
	if jitter {
		d = applyJitter(d)
	}
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
