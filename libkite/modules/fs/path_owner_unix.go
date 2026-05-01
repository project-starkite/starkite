//go:build unix

package fs

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func (p *Path) ownerMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "owner", p.path); err != nil {
		return nil, err
	}
	info, err := os.Stat(p.path)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("fs.path.owner: unsupported platform")
	}
	u, err := user.LookupId(strconv.Itoa(int(stat.Uid)))
	if err != nil {
		return starlark.String(strconv.Itoa(int(stat.Uid))), nil
	}
	return starlark.String(u.Username), nil
}

func (p *Path) groupMethod(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	if err := libkite.Check(p.thread, "fs", "group", p.path); err != nil {
		return nil, err
	}
	info, err := os.Stat(p.path)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("fs.path.group: unsupported platform")
	}
	g, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid)))
	if err != nil {
		return starlark.String(strconv.Itoa(int(stat.Gid))), nil
	}
	return starlark.String(g.Name), nil
}
