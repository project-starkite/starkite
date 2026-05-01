package base64

import (
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Base64File is a Starlark value representing a file path for base64 operations.
// Methods read the file and then encode or decode.
type Base64File struct {
	path   string
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*Base64File)(nil)
	_ starlark.HasAttrs = (*Base64File)(nil)
)

func (f *Base64File) String() string        { return fmt.Sprintf("base64.file(%q)", f.path) }
func (f *Base64File) Type() string          { return "base64.file" }
func (f *Base64File) Freeze()               {} // immutable
func (f *Base64File) Truth() starlark.Bool  { return starlark.Bool(f.path != "") }
func (f *Base64File) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: base64.file") }

func (f *Base64File) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := f.methodBuiltin(base); method != nil {
			return libkite.TryWrap("base64.file."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	if name == "path" {
		return starlark.String(f.path), nil
	}

	// Methods
	if method := f.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (f *Base64File) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "encode":
		return starlark.NewBuiltin("base64.file.encode", f.encodeMethod)
	case "decode":
		return starlark.NewBuiltin("base64.file.decode", f.decodeMethod)
	case "encode_url":
		return starlark.NewBuiltin("base64.file.encode_url", f.encodeURLMethod)
	case "decode_url":
		return starlark.NewBuiltin("base64.file.decode_url", f.decodeURLMethod)
	}
	return nil
}

func (f *Base64File) AttrNames() []string {
	names := []string{
		"path",
		"decode", "decode_url", "encode", "encode_url",
		"try_decode", "try_decode_url", "try_encode", "try_encode_url",
	}
	sort.Strings(names)
	return names
}

func (f *Base64File) isDryRun() bool {
	return f.config != nil && f.config.DryRun
}

func (f *Base64File) readFile() ([]byte, error) {
	if err := libkite.Check(f.thread, "fs", "read_file", f.path); err != nil {
		return nil, err
	}
	return os.ReadFile(f.path)
}

func (f *Base64File) encodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.file.encode: takes no arguments")
	}
	if f.isDryRun() {
		return starlark.String(""), nil
	}
	data, err := f.readFile()
	if err != nil {
		return nil, fmt.Errorf("base64.file.encode: %w", err)
	}
	return starlark.String(base64.StdEncoding.EncodeToString(data)), nil
}

func (f *Base64File) decodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.file.decode: takes no arguments")
	}
	if f.isDryRun() {
		return starlark.Bytes(""), nil
	}
	data, err := f.readFile()
	if err != nil {
		return nil, fmt.Errorf("base64.file.decode: %w", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("base64.file.decode: %w", err)
	}
	return starlark.Bytes(decoded), nil
}

func (f *Base64File) encodeURLMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.file.encode_url: takes no arguments")
	}
	if f.isDryRun() {
		return starlark.String(""), nil
	}
	data, err := f.readFile()
	if err != nil {
		return nil, fmt.Errorf("base64.file.encode_url: %w", err)
	}
	return starlark.String(base64.URLEncoding.EncodeToString(data)), nil
}

func (f *Base64File) decodeURLMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.file.decode_url: takes no arguments")
	}
	if f.isDryRun() {
		return starlark.Bytes(""), nil
	}
	data, err := f.readFile()
	if err != nil {
		return nil, fmt.Errorf("base64.file.decode_url: %w", err)
	}
	decoded, err := base64.URLEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("base64.file.decode_url: %w", err)
	}
	return starlark.Bytes(decoded), nil
}
