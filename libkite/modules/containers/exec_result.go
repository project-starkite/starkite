package containers

import (
	"fmt"

	"go.starlark.net/starlark"
)

// ExecResult represents the outcome of executing a command inside a container.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

var (
	_ starlark.Value    = (*ExecResult)(nil)
	_ starlark.HasAttrs = (*ExecResult)(nil)
)

func (r *ExecResult) String() string {
	return fmt.Sprintf("<containers.ExecResult ok=%t exit_code=%d>", r.ExitCode == 0, r.ExitCode)
}

func (r *ExecResult) Type() string         { return "containers.ExecResult" }
func (r *ExecResult) Freeze()              {}
func (r *ExecResult) Truth() starlark.Bool { return starlark.Bool(r.ExitCode == 0) }
func (r *ExecResult) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: containers.ExecResult")
}

func (r *ExecResult) AttrNames() []string {
	return []string{"exit_code", "ok", "stderr", "stdout"}
}

func (r *ExecResult) Attr(name string) (starlark.Value, error) {
	switch name {
	case "exit_code":
		return starlark.MakeInt(r.ExitCode), nil
	case "stdout":
		return starlark.String(r.Stdout), nil
	case "stderr":
		return starlark.String(r.Stderr), nil
	case "ok":
		return starlark.Bool(r.ExitCode == 0), nil
	default:
		return nil, nil
	}
}
