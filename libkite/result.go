package libkite

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"go.starlark.net/starlark"
)

// Result is a Starlark value representing the outcome of a try_ call.
// It implements starlark.Value and starlark.HasAttrs.
//
// Attributes:
//   - ok:    Bool  — True on success
//   - value: any   — return value, or None on error
//   - error: String — error message, or "" on success
//
// Truth() returns ok, enabling `if result:` shorthand.
type Result struct {
	ok    bool
	value starlark.Value
	err   string
}

var (
	_ starlark.Value    = (*Result)(nil)
	_ starlark.HasAttrs = (*Result)(nil)
)

// ResultOK creates a successful Result wrapping val.
func ResultOK(val starlark.Value) *Result {
	if val == nil {
		val = starlark.None
	}
	return &Result{ok: true, value: val}
}

// ResultErr creates a failed Result with the given error message.
func ResultErr(msg string) *Result {
	return &Result{ok: false, value: starlark.None, err: msg}
}

func (r *Result) String() string {
	if r.ok {
		return fmt.Sprintf("Result(ok=True, value=%s)", r.value.String())
	}
	return fmt.Sprintf("Result(ok=False, error=%q)", r.err)
}

func (r *Result) Type() string         { return "Result" }
func (r *Result) Freeze()              { r.value.Freeze() }
func (r *Result) Truth() starlark.Bool { return starlark.Bool(r.ok) }

func (r *Result) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: Result")
}

// ErrorString returns the error message string.
func (r *Result) ErrorString() string { return r.err }

func (r *Result) Attr(name string) (starlark.Value, error) {
	switch name {
	case "ok":
		return starlark.Bool(r.ok), nil
	case "value":
		return r.value, nil
	case "error":
		return starlark.String(r.err), nil
	default:
		return nil, nil
	}
}

func (r *Result) AttrNames() []string {
	return []string{"error", "ok", "value"}
}

// TryWrap wraps a *starlark.Builtin so that it returns a Result instead of
// propagating errors. On success the original return value is wrapped in
// ResultOK; on error the error message is wrapped in ResultErr and no
// Starlark error is returned.
func TryWrap(name string, fn *starlark.Builtin) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		val, err := starlark.Call(thread, fn, args, kwargs)
		if err != nil {
			return ResultErr(err.Error()), nil
		}
		return ResultOK(val), nil
	})
}

// TryModule is a drop-in replacement for starlarkstruct.Module that supports
// dynamic try_ prefix dispatch. Any attribute lookup for "try_X" where "X"
// is a registered *starlark.Builtin will return a TryWrap'd version.
type TryModule struct {
	name    string
	members starlark.StringDict
}

var (
	_ starlark.Value    = (*TryModule)(nil)
	_ starlark.HasAttrs = (*TryModule)(nil)
)

// NewTryModule creates a TryModule with the given name and members.
func NewTryModule(name string, members starlark.StringDict) *TryModule {
	return &TryModule{name: name, members: members}
}

func (m *TryModule) String() string        { return fmt.Sprintf("<module %q>", m.name) }
func (m *TryModule) Type() string          { return "module" }
func (m *TryModule) Truth() starlark.Bool  { return starlark.True }
func (m *TryModule) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: module") }

func (m *TryModule) Freeze() {
	for _, v := range m.members {
		v.Freeze()
	}
}

func (m *TryModule) Attr(name string) (starlark.Value, error) {
	// Direct lookup
	if v, ok := m.members[name]; ok {
		return v, nil
	}
	// try_ prefix dispatch
	if baseName, ok := strings.CutPrefix(name, "try_"); ok {
		if v, ok := m.members[baseName]; ok {
			if b, ok := v.(*starlark.Builtin); ok {
				return TryWrap(m.name+"."+name, b), nil
			}
		}
	}
	return nil, nil
}

func (m *TryModule) AttrNames() []string {
	names := make([]string, 0, len(m.members)*2)
	for k, v := range m.members {
		names = append(names, k)
		if _, ok := v.(*starlark.Builtin); ok {
			names = append(names, "try_"+k)
		}
	}
	sort.Strings(names)
	return names
}

// RetryResult is a Starlark value representing the outcome of a retry operation.
// It embeds *Result and adds retry-specific metrics: attempts, elapsed time, and error history.
type RetryResult struct {
	*Result
	attempts int
	elapsed  time.Duration
	errors   []string
}

var (
	_ starlark.Value    = (*RetryResult)(nil)
	_ starlark.HasAttrs = (*RetryResult)(nil)
)

// NewRetryResult creates a RetryResult with all fields.
func NewRetryResult(ok bool, value starlark.Value, errStr string, attempts int, elapsed time.Duration, errors []string) *RetryResult {
	var r *Result
	if ok {
		r = ResultOK(value)
	} else {
		r = ResultErr(errStr)
	}
	if errors == nil {
		errors = []string{}
	}
	return &RetryResult{
		Result:   r,
		attempts: attempts,
		elapsed:  elapsed,
		errors:   errors,
	}
}

func (rr *RetryResult) String() string {
	if rr.Result.ok {
		return fmt.Sprintf("RetryResult(ok=True, attempts=%d, elapsed=%q)", rr.attempts, rr.elapsed.String())
	}
	return fmt.Sprintf("RetryResult(ok=False, attempts=%d, elapsed=%q, error=%q)", rr.attempts, rr.elapsed.String(), rr.Result.err)
}

func (rr *RetryResult) Type() string { return "RetryResult" }

func (rr *RetryResult) Freeze() { rr.Result.Freeze() }

func (rr *RetryResult) Truth() starlark.Bool { return rr.Result.Truth() }

func (rr *RetryResult) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: RetryResult")
}

func (rr *RetryResult) Attr(name string) (starlark.Value, error) {
	switch name {
	case "ok", "value", "error":
		return rr.Result.Attr(name)
	case "attempts":
		return starlark.MakeInt(rr.attempts), nil
	case "elapsed":
		return starlark.String(rr.elapsed.String()), nil
	case "errors":
		elems := make([]starlark.Value, len(rr.errors))
		for i, e := range rr.errors {
			elems[i] = starlark.String(e)
		}
		return starlark.NewList(elems), nil
	default:
		return nil, nil
	}
}

func (rr *RetryResult) AttrNames() []string {
	return []string{"attempts", "elapsed", "error", "errors", "ok", "value"}
}
