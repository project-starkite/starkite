package libkite

import (
	"fmt"

	"go.starlark.net/starlark"
)

// builtinFail implements fail(msg) - exits with code 1.
func (rt *Runtime) builtinFail(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg?", &msg); err != nil {
		return nil, err
	}
	if msg == "" {
		msg = "script failed"
	}
	return nil, fmt.Errorf("%s", msg)
}

// builtinExit implements exit(code) - exits with custom code.
func (rt *Runtime) builtinExit(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	code := 0
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "code?", &code); err != nil {
		return nil, err
	}
	return nil, &exitError{code: code}
}

// builtinDefer implements defer(fn) - registers cleanup function.
func (rt *Runtime) builtinDefer(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var callable starlark.Callable
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "fn", &callable); err != nil {
		return nil, err
	}

	rt.deferMu.Lock()
	rt.deferredFuncs = append(rt.deferredFuncs, callable)
	rt.deferMu.Unlock()

	return starlark.None, nil
}

// builtinOnSignal implements on_signal(name, handler) - registers signal handler.
func (rt *Runtime) builtinOnSignal(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var signalName string
	var handler starlark.Callable
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "signal", &signalName, "handler", &handler); err != nil {
		return nil, err
	}

	rt.signalMu.Lock()
	rt.signalHandlers[signalName] = handler
	rt.signalMu.Unlock()

	return starlark.None, nil
}

// builtinResult implements Result(ok=, value=, error=) — constructs a Result value.
func (rt *Runtime) builtinResult(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ok bool
	var value starlark.Value = starlark.None
	var errMsg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"ok", &ok, "value?", &value, "error?", &errMsg); err != nil {
		return nil, err
	}
	if ok {
		return ResultOK(value), nil
	}
	return ResultErr(errMsg), nil
}

