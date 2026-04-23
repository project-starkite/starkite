// Package concur provides concurrent execution functions for starkite.
package concur

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "concur"

// Module implements concurrent execution functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "concur provides concurrent execution: map, each, exec"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			"map":  starlark.NewBuiltin("concur.map", m.concurMap),
			"each": starlark.NewBuiltin("concur.each", m.concurEach),
			"exec": starlark.NewBuiltin("concur.exec", m.concurExec),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// iterableToSlice converts a starlark.Iterable to a []starlark.Value.
func iterableToSlice(v starlark.Value) ([]starlark.Value, error) {
	switch x := v.(type) {
	case *starlark.List:
		n := x.Len()
		out := make([]starlark.Value, n)
		for i := 0; i < n; i++ {
			out[i] = x.Index(i)
		}
		return out, nil
	case starlark.Tuple:
		out := make([]starlark.Value, len(x))
		copy(out, x)
		return out, nil
	case starlark.Iterable:
		var out []starlark.Value
		iter := x.Iterate()
		defer iter.Done()
		var elem starlark.Value
		for iter.Next(&elem) {
			out = append(out, elem)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected iterable, got %s", v.Type())
	}
}

// newChildThread creates a child thread that inherits Load, Print, permissions,
// and runtime from the parent thread.
func newChildThread(parent *starlark.Thread, name string) *starlark.Thread {
	if rt := starbase.GetRuntime(parent); rt != nil {
		return rt.NewThread(name)
	}
	// Fallback for tests without full Runtime
	child := &starlark.Thread{Name: name}
	child.Print = parent.Print
	child.Load = parent.Load
	if perms := starbase.GetPermissions(parent); perms != nil {
		starbase.SetPermissions(child, perms)
	}
	return child
}

// parseTimeout parses a duration string. Returns 0 for empty string.
func parseTimeout(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout: %w", err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("timeout must be positive, got %s", s)
	}
	return d, nil
}

// validateOnError validates the on_error parameter value.
func validateOnError(v string) error {
	switch v {
	case "", "abort", "continue":
		return nil
	default:
		return fmt.Errorf("on_error must be \"abort\" or \"continue\", got %q", v)
	}
}

// concurMap applies a function to each element of an iterable concurrently
// and returns a list of results.
//
// Usage: concur.map(items, func, workers=0, timeout="", on_error="abort")
func (m *Module) concurMap(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Items   starlark.Value    `name:"items" position:"0" required:"true"`
		Func    starlark.Callable `name:"func" position:"1" required:"true"`
		Workers int               `name:"workers"`
		Timeout string            `name:"timeout"`
		OnError string            `name:"on_error"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := validateOnError(p.OnError); err != nil {
		return nil, fmt.Errorf("concur.map: %w", err)
	}

	if p.Workers < 0 {
		return nil, fmt.Errorf("concur.map: workers must be non-negative, got %d", p.Workers)
	}

	items, err := iterableToSlice(p.Items)
	if err != nil {
		return nil, fmt.Errorf("concur.map: %w", err)
	}

	timeout, err := parseTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("concur.map: %w", err)
	}

	n := len(items)
	results := make([]starlark.Value, n)
	errors := make([]error, n)

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var sem chan struct{}
	if p.Workers > 0 {
		sem = make(chan struct{}, p.Workers)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		// Check context before launching goroutine
		if ctx.Err() != nil {
			errors[i] = fmt.Errorf("timeout exceeded")
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Acquire semaphore if worker pool is set
			if sem != nil {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					errors[idx] = fmt.Errorf("timeout exceeded")
					return
				}
			}

			// Check context before calling function
			if ctx.Err() != nil {
				errors[idx] = fmt.Errorf("timeout exceeded")
				return
			}

			childThread := newChildThread(thread, fmt.Sprintf("concur.map-%d", idx))
			result, err := starlark.Call(childThread, p.Func, starlark.Tuple{items[idx]}, nil)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = result
		}(i)
	}

	wg.Wait()

	if p.OnError == "continue" {
		wrapped := make([]starlark.Value, n)
		for i := 0; i < n; i++ {
			if errors[i] != nil {
				wrapped[i] = starbase.ResultErr(errors[i].Error())
			} else {
				wrapped[i] = starbase.ResultOK(results[i])
			}
		}
		return starlark.NewList(wrapped), nil
	}

	// Default "abort" mode
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("concur.map: item %d: %w", i, err)
		}
	}
	return starlark.NewList(results), nil
}

// concurEach applies a function to each element of an iterable concurrently
// without returning results (for side effects).
//
// Usage: concur.each(items, func, workers=0, timeout="", on_error="abort")
func (m *Module) concurEach(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Items   starlark.Value    `name:"items" position:"0" required:"true"`
		Func    starlark.Callable `name:"func" position:"1" required:"true"`
		Workers int               `name:"workers"`
		Timeout string            `name:"timeout"`
		OnError string            `name:"on_error"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := validateOnError(p.OnError); err != nil {
		return nil, fmt.Errorf("concur.each: %w", err)
	}

	if p.Workers < 0 {
		return nil, fmt.Errorf("concur.each: workers must be non-negative, got %d", p.Workers)
	}

	items, err := iterableToSlice(p.Items)
	if err != nil {
		return nil, fmt.Errorf("concur.each: %w", err)
	}

	timeout, err := parseTimeout(p.Timeout)
	if err != nil {
		return nil, fmt.Errorf("concur.each: %w", err)
	}

	n := len(items)
	errors := make([]error, n)

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var sem chan struct{}
	if p.Workers > 0 {
		sem = make(chan struct{}, p.Workers)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			errors[i] = fmt.Errorf("timeout exceeded")
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			if sem != nil {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					errors[idx] = fmt.Errorf("timeout exceeded")
					return
				}
			}

			if ctx.Err() != nil {
				errors[idx] = fmt.Errorf("timeout exceeded")
				return
			}

			childThread := newChildThread(thread, fmt.Sprintf("concur.each-%d", idx))
			_, err := starlark.Call(childThread, p.Func, starlark.Tuple{items[idx]}, nil)
			if err != nil {
				errors[idx] = err
			}
		}(i)
	}

	wg.Wait()

	if p.OnError == "continue" {
		wrapped := make([]starlark.Value, n)
		for i := 0; i < n; i++ {
			if errors[i] != nil {
				wrapped[i] = starbase.ResultErr(errors[i].Error())
			} else {
				wrapped[i] = starbase.ResultOK(starlark.None)
			}
		}
		return starlark.NewList(wrapped), nil
	}

	// Default "abort" mode
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("concur.each: item %d: %w", i, err)
		}
	}
	return starlark.None, nil
}

// concurExec runs multiple functions concurrently and returns a tuple of results.
//
// Usage: a, b, c = concur.exec(fn_a, fn_b, fn_c, timeout="", on_error="abort")
func (m *Module) concurExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Parse kwargs manually since we need variadic positional callables
	var timeout string
	var onError string
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := string(kv[1].(starlark.String))
		switch key {
		case "timeout":
			timeout = val
		case "on_error":
			onError = val
		default:
			return nil, fmt.Errorf("concur.exec: unexpected keyword argument %q", key)
		}
	}

	if err := validateOnError(onError); err != nil {
		return nil, fmt.Errorf("concur.exec: %w", err)
	}

	// All positional args must be callable
	fns := make([]starlark.Callable, len(args))
	for i, arg := range args {
		callable, ok := arg.(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("concur.exec: argument %d is not callable, got %s", i, arg.Type())
		}
		fns[i] = callable
	}

	dur, err := parseTimeout(timeout)
	if err != nil {
		return nil, fmt.Errorf("concur.exec: %w", err)
	}

	n := len(fns)
	if n == 0 {
		return starlark.Tuple{}, nil
	}

	results := make([]starlark.Value, n)
	errors := make([]error, n)

	ctx := context.Background()
	var cancel context.CancelFunc
	if dur > 0 {
		ctx, cancel = context.WithTimeout(ctx, dur)
		defer cancel()
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			errors[i] = fmt.Errorf("timeout exceeded")
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			if ctx.Err() != nil {
				errors[idx] = fmt.Errorf("timeout exceeded")
				return
			}

			childThread := newChildThread(thread, fmt.Sprintf("concur.exec-%d", idx))
			result, err := starlark.Call(childThread, fns[idx], nil, nil)
			if err != nil {
				errors[idx] = err
				return
			}
			results[idx] = result
		}(i)
	}

	wg.Wait()

	if onError == "continue" {
		wrapped := make([]starlark.Value, n)
		for i := 0; i < n; i++ {
			if errors[i] != nil {
				wrapped[i] = starbase.ResultErr(errors[i].Error())
			} else {
				wrapped[i] = starbase.ResultOK(results[i])
			}
		}
		return starlark.Tuple(wrapped), nil
	}

	// Default "abort" mode
	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("concur.exec: function %d: %w", i, err)
		}
	}

	tuple := make(starlark.Tuple, n)
	copy(tuple, results)
	return tuple, nil
}
