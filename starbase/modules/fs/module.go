// Package fs provides file system operations for starkite.
package fs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/starbase"
)

const ModuleName starbase.ModuleName = "fs"

// Module implements file system operations.
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
	return "fs provides file system operations: read, write, copy, move, path manipulation"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		// fs module exposes only the Path factory
		members := starlark.StringDict{
			"path": starlark.NewBuiltin("fs.path", m.pathFactory),
		}

		m.module = starbase.NewTryModule(string(ModuleName), members)

		// Global aliases for common one-liners
		m.aliases = starlark.StringDict{
			"read_text":   starlark.NewBuiltin("read_text", m.readFile),
			"read_bytes":  starlark.NewBuiltin("read_bytes", m.readBytes),
			"write_text":  starlark.NewBuiltin("write_text", m.write),
			"write_bytes": starlark.NewBuiltin("write_bytes", m.write),
			"exists":      starlark.NewBuiltin("exists", m.exists),
			"glob":        starlark.NewBuiltin("glob", m.glob),
			"path":        starlark.NewBuiltin("path", m.pathFactory),
		}
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return m.aliases
}

func (m *Module) FactoryMethod() string { return "path" }

// pathFactory creates a new Path object.
// Usage: fs.path("/etc/hostname") or path("/etc/hostname")
func (m *Module) pathFactory(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s: takes exactly 1 argument (path string)", fn.Name())
	}
	p, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("%s: argument must be a string, got %s", fn.Name(), args[0].Type())
	}
	return &Path{path: p, thread: thread, config: m.config}, nil
}

// ============================================================================
// Read/Write operations
// ============================================================================

func (m *Module) readFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if err := starbase.Check(thread, "fs", "read_file", p.Path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, err
	}
	return starlark.String(data), nil
}

// write writes content (string or bytes) to a file.
// Usage: fs.write(path, content, mode=0644)
func (m *Module) write(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("%s: expected at least 2 arguments (path, content)", fn.Name())
	}

	path, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("%s: path must be a string, got %s", fn.Name(), args[0].Type())
	}

	var data []byte
	switch v := args[1].(type) {
	case starlark.String:
		data = []byte(string(v))
	case starlark.Bytes:
		data = []byte(string(v))
	default:
		return nil, fmt.Errorf("%s: content must be string or bytes, got %s", fn.Name(), args[1].Type())
	}

	mode := 0644
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key == "mode" {
			if modeInt, err := starlark.AsInt32(kv[1]); err == nil {
				mode = int(modeInt)
			}
		}
	}

	if err := starbase.Check(thread, "fs", "write", path); err != nil {
		return nil, err
	}

	if m.config != nil && m.config.DryRun {
		return starlark.None, nil
	}

	if err := os.WriteFile(path, data, fs.FileMode(mode)); err != nil {
		return nil, err
	}
	return starlark.None, nil
}

func (m *Module) readBytes(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if err := starbase.Check(thread, "fs", "read_bytes", p.Path); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, err
	}
	return starlark.Bytes(data), nil
}

// ============================================================================
// File checks
// ============================================================================

func (m *Module) exists(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if err := starbase.Check(thread, "fs", "exists", p.Path); err != nil {
		return nil, err
	}
	_, err := os.Stat(p.Path)
	return starlark.Bool(err == nil), nil
}

// ============================================================================
// Search
// ============================================================================

func (m *Module) glob(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if err := starbase.Check(thread, "fs", "glob", p.Pattern); err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(p.Pattern)
	if err != nil {
		return nil, err
	}
	elems := make([]starlark.Value, len(matches))
	for i, match := range matches {
		elems[i] = starlark.String(match)
	}
	return starlark.NewList(elems), nil
}
