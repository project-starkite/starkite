//go:build windows

package fs

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Windows file ownership maps to SIDs, not POSIX uid/gid. Reporting a SID
// without a corresponding LookupID surface would confuse users; for now,
// return a clear "unsupported" error and let scripts handle it.

func (p *Path) ownerMethod(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "fs", "read", "owner", checkPath(p.path)); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("fs.path.owner: unsupported on windows")
}

func (p *Path) groupMethod(thread *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(thread, "fs", "read", "group", checkPath(p.path)); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("fs.path.group: unsupported on windows")
}
