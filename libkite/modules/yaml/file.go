package yaml

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	goyaml "gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

// YamlFile is a Starlark value representing a file path for YAML reading.
type YamlFile struct {
	path   string
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*YamlFile)(nil)
	_ starlark.HasAttrs = (*YamlFile)(nil)
)

func (f *YamlFile) String() string        { return fmt.Sprintf("yaml.file(%q)", f.path) }
func (f *YamlFile) Type() string          { return "yaml.file" }
func (f *YamlFile) Freeze()               {}
func (f *YamlFile) Truth() starlark.Bool  { return starlark.Bool(f.path != "") }
func (f *YamlFile) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: yaml.file") }

func (f *YamlFile) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := f.methodBuiltin(base); method != nil {
			return libkite.TryWrap("yaml.file."+name, method), nil
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

func (f *YamlFile) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "decode":
		return starlark.NewBuiltin("yaml.file.decode", f.decodeMethod)
	case "decode_all":
		return starlark.NewBuiltin("yaml.file.decode_all", f.decodeAllMethod)
	}
	return nil
}

func (f *YamlFile) AttrNames() []string {
	names := []string{"path", "decode", "decode_all", "try_decode", "try_decode_all"}
	sort.Strings(names)
	return names
}

func (f *YamlFile) isDryRun() bool {
	return f.config != nil && f.config.DryRun
}

// decodeMethod reads and decodes a single YAML document.
// Usage: yaml.file(path).decode()
func (f *YamlFile) decodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("yaml.file.decode: takes no arguments")
	}

	if err := libkite.Check(f.thread, "fs", "read_file", f.path); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.None, nil
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("yaml.file.decode: %w", err)
	}

	var v interface{}
	if err := goyaml.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("yaml.file.decode: %w", err)
	}

	var result starlark.Value
	if err := startype.Go(v).Starlark(&result); err != nil {
		return nil, fmt.Errorf("yaml.file.decode: %w", err)
	}
	return result, nil
}

// decodeAllMethod reads and decodes a multi-document YAML file.
// Usage: yaml.file(path).decode_all()
func (f *YamlFile) decodeAllMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("yaml.file.decode_all: takes no arguments")
	}

	if err := libkite.Check(f.thread, "fs", "read_file", f.path); err != nil {
		return nil, err
	}

	if f.isDryRun() {
		return starlark.NewList(nil), nil
	}

	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("yaml.file.decode_all: %w", err)
	}

	decoder := goyaml.NewDecoder(bytes.NewReader(data))
	var results []starlark.Value
	for {
		var v interface{}
		if err := decoder.Decode(&v); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("yaml.file.decode_all: %w", err)
		}
		var starVal starlark.Value
		if err := startype.Go(v).Starlark(&starVal); err != nil {
			return nil, fmt.Errorf("yaml.file.decode_all: %w", err)
		}
		results = append(results, starVal)
	}
	return starlark.NewList(results), nil
}
