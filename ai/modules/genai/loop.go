package genai

import (
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// defaultRunUntilMaxSteps caps autonomous ai.run_until() runs by default.
// Users who want longer must set max_steps= explicitly — prevents a misconfigured
// stop_when predicate from turning into an infinite API-spend loop.
const defaultRunUntilMaxSteps = 10

// runUntilBuiltin is the `ai.run_until(chat, initial, ...)` Starlark builtin.
//
// Sends `initial` as the first user message, then repeatedly sends `follow_up`
// (default "continue") until `stop_when(resp)` returns truthy or `max_steps`
// is reached. Returns the final *Response.
//
// Designed for autonomous run-to-completion patterns. Interactive (user in the
// loop) patterns are meant to be written as a plain Starlark while loop using
// io.prompt() + chat.send() — this builtin covers the case where the agent
// self-drives.
func (m *Module) runUntilBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("ai.run_until: expected at least 2 positional arguments (chat, initial), got %d", len(args))
	}
	if len(args) > 2 {
		return nil, fmt.Errorf("ai.run_until: expected 2 positional arguments, got %d (use keyword arguments for stop_when/max_steps/follow_up)", len(args))
	}
	chat, ok := args[0].(*Chat)
	if !ok {
		return nil, fmt.Errorf("ai.run_until: first argument must be an ai.Chat, got %s", args[0].Type())
	}
	initial, ok := starlark.AsString(args[1])
	if !ok {
		return nil, fmt.Errorf("ai.run_until: initial must be a string, got %s", args[1].Type())
	}
	if initial == "" {
		return nil, fmt.Errorf("ai.run_until: initial must be a non-empty string")
	}

	var p struct {
		StopWhen starlark.Value `name:"stop_when"`
		MaxSteps int            `name:"max_steps"`
		FollowUp string         `name:"follow_up"`
	}
	if err := startype.Args(starlark.Tuple{}, kwargs).Go(&p); err != nil {
		return nil, fmt.Errorf("ai.run_until: %w", err)
	}

	maxSteps := p.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultRunUntilMaxSteps
	}
	if maxSteps < 0 {
		return nil, fmt.Errorf("ai.run_until: max_steps must be positive, got %d", maxSteps)
	}

	var stopFn starlark.Callable
	if p.StopWhen != nil && p.StopWhen != starlark.None {
		c, ok := p.StopWhen.(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("ai.run_until: stop_when must be callable, got %s", p.StopWhen.Type())
		}
		stopFn = c
	}

	followUp := p.FollowUp
	if followUp == "" {
		followUp = "continue"
	}

	// Loop.
	var lastResp starlark.Value
	for step := 0; step < maxSteps; step++ {
		msg := followUp
		if step == 0 {
			msg = initial
		}

		// Delegate to chat.send(msg) — we invoke the bound builtin so that
		// all the merging/validation/history-append logic runs unchanged.
		sendArg := starlark.Tuple{starlark.String(msg)}
		resp, err := starlark.Call(thread, starlark.NewBuiltin("ai.chat.send", chat.sendBuiltin), sendArg, nil)
		if err != nil {
			return nil, fmt.Errorf("ai.run_until: step %d: %w", step, err)
		}
		lastResp = resp

		if stopFn != nil {
			stop, err := starlark.Call(thread, stopFn, starlark.Tuple{resp}, nil)
			if err != nil {
				return nil, fmt.Errorf("ai.run_until: stop_when: %w", err)
			}
			if bool(stop.Truth()) {
				return resp, nil
			}
		}
	}

	if lastResp == nil {
		// Unreachable — maxSteps > 0 guaranteed.
		return starlark.None, nil
	}
	return lastResp, nil
}
