package json

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// JsonFile is a Starlark value representing a file path for JSON reading.
type JsonFile struct {
	path   string
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*JsonFile)(nil)
	_ starlark.HasAttrs = (*JsonFile)(nil)
)

func (f *JsonFile) String() string        { return fmt.Sprintf("json.file(%q)", f.path) }
func (f *JsonFile) Type() string          { return "json.file" }
func (f *JsonFile) Freeze()               {}
func (f *JsonFile) Truth() starlark.Bool  { return starlark.Bool(f.path != "") }
func (f *JsonFile) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: json.file") }

func (f *JsonFile) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := f.methodBuiltin(base); method != nil {
			return libkite.TryWrap("json.file."+name, method), nil
		}
		return nil, nil
	}

	if name == "path" {
		return starlark.String(f.path), nil
	}

	if method := f.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (f *JsonFile) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "decode":
		return starlark.NewBuiltin("json.file.decode", f.decodeMethod)
	}
	return nil
}

func (f *JsonFile) AttrNames() []string {
	names := []string{"path", "decode", "try_decode"}
	sort.Strings(names)
	return names
}

func (f *JsonFile) isDryRun() bool {
	return f.config != nil && f.config.DryRun
}

// decodeMethod reads and decodes a JSON file.
// Usage: json.file(path).decode()
func (f *JsonFile) decodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("json.file.decode: takes no arguments")
	}

	if err := libkite.Check(f.thread, "fs", "read_file", f.path); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.None, nil
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("json.file.decode: %w", err)
	}

	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("json.file.decode: %w", err)
	}

	var result starlark.Value
	if err := startype.Go(v).Starlark(&result); err != nil {
		return nil, fmt.Errorf("json.file.decode: %w", err)
	}
	return result, nil
}
