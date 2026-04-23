// Package test provides test utilities for starkite.
package test

import (
	"fmt"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "test"

// Module implements test utility functions.
type Module struct {
	once    sync.Once
	module  starlark.Value
	aliases starlark.StringDict
	config  *starbase.ModuleConfig
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "test provides test utilities: skip, fail, assert helpers"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			"skip":             starlark.NewBuiltin("test.skip", m.skip),
			"fail":             starlark.NewBuiltin("test.fail", m.fail),
			"assert":           starlark.NewBuiltin("test.assert", m.assert_),
			"assert_equal":     starlark.NewBuiltin("test.assert_equal", m.assertEqual),
			"assert_not_equal": starlark.NewBuiltin("test.assert_not_equal", m.assertNotEqual),
			"assert_contains":  starlark.NewBuiltin("test.assert_contains", m.assertContains),
			"assert_true":      starlark.NewBuiltin("test.assert_true", m.assertTrue),
			"assert_false":     starlark.NewBuiltin("test.assert_false", m.assertFalse),
		}

		m.module = &starlarkstruct.Module{
			Name:    string(ModuleName),
			Members: members,
		}

		// Global aliases for common test functions
		m.aliases = starlark.StringDict{
			"skip":             starlark.NewBuiltin("skip", m.skip),
			"fail":             starlark.NewBuiltin("fail", m.fail),
			"assert":           starlark.NewBuiltin("assert", m.assert_),
			"assert_equal":     starlark.NewBuiltin("assert_equal", m.assertEqual),
			"assert_not_equal": starlark.NewBuiltin("assert_not_equal", m.assertNotEqual),
			"assert_contains":  starlark.NewBuiltin("assert_contains", m.assertContains),
			"assert_true":      starlark.NewBuiltin("assert_true", m.assertTrue),
			"assert_false":     starlark.NewBuiltin("assert_false", m.assertFalse),
		}
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return m.aliases
}

func (m *Module) FactoryMethod() string { return "" }

// skip skips the current test with an optional reason.
// Usage: skip() or skip("reason")
func (m *Module) skip(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var reason string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "reason?", &reason); err != nil {
		return nil, err
	}

	if reason != "" {
		return nil, fmt.Errorf("test skipped: %s", reason)
	}
	return nil, fmt.Errorf("test skipped")
}

// fail explicitly fails the current test with a message.
// Usage: fail("reason")
func (m *Module) fail(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("test failed: %s", msg)
}

// assert_ implements assert(condition, msg?, *args).
//
// Condition is evaluated for truthiness (not limited to Bool).
// Message is optional — defaults to "assertion failed".
// Extra args are interpolated into msg via fmt.Sprintf verbs (%d, %s, etc.).
// Starlark str.format() ("{}".format(x)) is evaluated at script-time before
// the function is called, so only printf-style is needed here.
func (m *Module) assert_(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("assert: requires at least 1 argument (condition)")
	}

	if args[0].Truth() == starlark.True {
		return starlark.None, nil
	}

	// Build error message
	if len(args) < 2 {
		return nil, fmt.Errorf("assertion failed")
	}

	msgStr, ok := starlark.AsString(args[1])
	if !ok {
		return nil, fmt.Errorf("assert: message must be a string")
	}

	if len(args) > 2 {
		goArgs := make([]any, len(args)-2)
		for i, arg := range args[2:] {
			var goVal any
			if err := startype.Starlark(arg).Go(&goVal); err != nil {
				goArgs[i] = arg.String()
				continue
			}
			goArgs[i] = goVal
		}
		msgStr = fmt.Sprintf(msgStr, goArgs...)
	}

	return nil, fmt.Errorf("assertion failed: %s", msgStr)
}

// formatMsg builds an error message from optional msg + printf args starting at the given
// index in the args tuple. Returns empty string if no message argument is present.
func formatMsg(args starlark.Tuple, msgIndex int) (string, error) {
	if len(args) <= msgIndex {
		return "", nil
	}
	msgStr, ok := starlark.AsString(args[msgIndex])
	if !ok {
		return "", fmt.Errorf("message must be a string")
	}
	if len(args) > msgIndex+1 {
		goArgs := make([]any, len(args)-msgIndex-1)
		for i, arg := range args[msgIndex+1:] {
			var goVal any
			if err := startype.Starlark(arg).Go(&goVal); err != nil {
				goArgs[i] = arg.String()
				continue
			}
			goArgs[i] = goVal
		}
		msgStr = fmt.Sprintf(msgStr, goArgs...)
	}
	return msgStr, nil
}

// assertEqual implements assert_equal(actual, expected, msg?, *args).
func (m *Module) assertEqual(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assert_equal: requires at least 2 arguments (actual, expected)")
	}
	eq, err := starlark.Equal(args[0], args[1])
	if err != nil {
		return nil, fmt.Errorf("assert_equal: comparison error: %w", err)
	}
	if eq {
		return starlark.None, nil
	}
	msg, err := formatMsg(args, 2)
	if err != nil {
		return nil, fmt.Errorf("assert_equal: %w", err)
	}
	if msg != "" {
		return nil, fmt.Errorf("assert_equal: %s", msg)
	}
	return nil, fmt.Errorf("assert_equal: got %s, want %s", args[0].String(), args[1].String())
}

// assertNotEqual implements assert_not_equal(actual, unexpected, msg?, *args).
func (m *Module) assertNotEqual(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assert_not_equal: requires at least 2 arguments (actual, unexpected)")
	}
	eq, err := starlark.Equal(args[0], args[1])
	if err != nil {
		return nil, fmt.Errorf("assert_not_equal: comparison error: %w", err)
	}
	if !eq {
		return starlark.None, nil
	}
	msg, err := formatMsg(args, 2)
	if err != nil {
		return nil, fmt.Errorf("assert_not_equal: %w", err)
	}
	if msg != "" {
		return nil, fmt.Errorf("assert_not_equal: %s", msg)
	}
	return nil, fmt.Errorf("assert_not_equal: values are equal: %s", args[0].String())
}

// assertContains implements assert_contains(haystack, needle, msg?, *args).
func (m *Module) assertContains(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("assert_contains: requires at least 2 arguments (haystack, needle)")
	}
	result, err := starlark.Binary(syntax.IN, args[1], args[0])
	if err != nil {
		return nil, fmt.Errorf("assert_contains: containment check error: %w", err)
	}
	if result.Truth() == starlark.True {
		return starlark.None, nil
	}
	msg, err := formatMsg(args, 2)
	if err != nil {
		return nil, fmt.Errorf("assert_contains: %w", err)
	}
	if msg != "" {
		return nil, fmt.Errorf("assert_contains: %s", msg)
	}
	return nil, fmt.Errorf("assert_contains: %s does not contain %s", args[0].String(), args[1].String())
}

// assertTrue implements assert_true(condition, msg?, *args).
func (m *Module) assertTrue(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("assert_true: requires at least 1 argument (condition)")
	}
	if args[0].Truth() == starlark.True {
		return starlark.None, nil
	}
	msg, err := formatMsg(args, 1)
	if err != nil {
		return nil, fmt.Errorf("assert_true: %w", err)
	}
	if msg != "" {
		return nil, fmt.Errorf("assert_true: %s", msg)
	}
	return nil, fmt.Errorf("assert_true: got %s (falsy)", args[0].String())
}

// assertFalse implements assert_false(condition, msg?, *args).
func (m *Module) assertFalse(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("assert_false: requires at least 1 argument (condition)")
	}
	if args[0].Truth() == starlark.False {
		return starlark.None, nil
	}
	msg, err := formatMsg(args, 1)
	if err != nil {
		return nil, fmt.Errorf("assert_false: %w", err)
	}
	if msg != "" {
		return nil, fmt.Errorf("assert_false: %s", msg)
	}
	return nil, fmt.Errorf("assert_false: got %s (truthy)", args[0].String())
}
