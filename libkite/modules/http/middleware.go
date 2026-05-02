package http

import (
	"go.starlark.net/starlark"
)

// callChain executes the middleware chain ending with the handler.
// Middleware receives (req, next) where next is a callable that invokes the
// next middleware or final handler. Middleware can short-circuit by returning
// a response without calling next.
func callChain(thread *starlark.Thread, middlewares []starlark.Callable,
	handler starlark.Callable, req *Request) (starlark.Value, error) {

	if len(middlewares) == 0 {
		return starlark.Call(thread, handler, starlark.Tuple{req}, nil)
	}

	// Build chain from inside out: handler is the innermost callable.
	var next starlark.Callable = &middlewareStep{
		name: "handler",
		call: func(thread *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
			return starlark.Call(thread, handler, args, nil)
		},
	}

	for i := len(middlewares) - 1; i >= 0; i-- {
		mw := middlewares[i]
		captured := next
		next = &middlewareStep{
			name: "middleware",
			call: func(thread *starlark.Thread, args starlark.Tuple) (starlark.Value, error) {
				// mw(req, next) — pass request and the next step
				return starlark.Call(thread, mw, starlark.Tuple{args[0], captured}, nil)
			},
		}
	}

	// Call the outermost middleware
	return starlark.Call(thread, next, starlark.Tuple{req}, nil)
}

// middlewareStep wraps a function as a starlark.Callable for use in the chain.
type middlewareStep struct {
	name string
	call func(thread *starlark.Thread, args starlark.Tuple) (starlark.Value, error)
}

func (m *middlewareStep) Name() string { return m.name }
func (m *middlewareStep) String() string {
	return "<" + m.name + ">"
}
func (m *middlewareStep) Type() string          { return "middleware_step" }
func (m *middlewareStep) Freeze()               {}
func (m *middlewareStep) Truth() starlark.Bool  { return starlark.True }
func (m *middlewareStep) Hash() (uint32, error) { return 0, nil }

func (m *middlewareStep) CallInternal(thread *starlark.Thread, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return m.call(thread, args)
}
