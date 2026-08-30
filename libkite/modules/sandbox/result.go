package sandbox

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"go.starlark.net/starlark"
)

// BoxExecResult is a Starlark value returned by box.exec() and sandbox.run_script()
// providing access to stdout, stderr, exit code, error, duration, and OOM/timeout telemetry.
type BoxExecResult struct {
	stdout      string
	stderr      string
	exitCode    int
	errMsg      string
	duration    time.Duration
	killedByOOM bool
	timedOut    bool
}

var (
	_ starlark.Value    = (*BoxExecResult)(nil)
	_ starlark.HasAttrs = (*BoxExecResult)(nil)
)

func NewBoxExecResult(stdout, stderr string, exitCode int, errMsg string, duration time.Duration, oom, timedOut bool) *BoxExecResult {
	return &BoxExecResult{
		stdout:      stdout,
		stderr:      stderr,
		exitCode:    exitCode,
		errMsg:      errMsg,
		duration:    duration,
		killedByOOM: oom,
		timedOut:    timedOut,
	}
}

func (r *BoxExecResult) isOK() bool {
	return r.exitCode == 0 && r.errMsg == ""
}

func (r *BoxExecResult) String() string {
	if r.isOK() {
		return fmt.Sprintf("BoxExecResult(ok=True, code=%d, duration=%v)", r.exitCode, r.duration)
	}
	return fmt.Sprintf("BoxExecResult(ok=False, code=%d, error=%q, duration=%v)", r.exitCode, r.errorString(), r.duration)
}

func (r *BoxExecResult) Type() string         { return "BoxExecResult" }
func (r *BoxExecResult) Freeze()              {}
func (r *BoxExecResult) Truth() starlark.Bool { return starlark.Bool(r.isOK()) }
func (r *BoxExecResult) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: BoxExecResult")
}

func (r *BoxExecResult) errorString() string {
	if r.errMsg != "" {
		return r.errMsg
	}
	if r.exitCode != 0 {
		return strings.TrimSpace(r.stderr)
	}
	return ""
}

func (r *BoxExecResult) Attr(name string) (starlark.Value, error) {
	switch name {
	case "ok":
		return starlark.Bool(r.isOK()), nil
	case "exit_code", "code":
		return starlark.MakeInt(r.exitCode), nil
	case "stdout":
		return starlark.String(r.stdout), nil
	case "stderr":
		return starlark.String(r.stderr), nil
	case "error":
		return starlark.String(r.errorString()), nil
	case "duration":
		return starlark.String(r.duration.String()), nil
	case "duration_seconds":
		return starlark.Float(r.duration.Seconds()), nil
	case "killed_by_oom":
		return starlark.Bool(r.killedByOOM), nil
	case "timed_out":
		return starlark.Bool(r.timedOut), nil
	default:
		return nil, nil
	}
}

func (r *BoxExecResult) AttrNames() []string {
	names := []string{
		"code", "duration", "duration_seconds", "error",
		"exit_code", "killed_by_oom", "ok", "stderr", "stdout", "timed_out",
	}
	sort.Strings(names)
	return names
}
