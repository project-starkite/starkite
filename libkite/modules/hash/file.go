package hash

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// HashFile is a Starlark value representing a file path for hashing.
// Methods read the file and compute the hash.
type HashFile struct {
	path   string
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*HashFile)(nil)
	_ starlark.HasAttrs = (*HashFile)(nil)
)

func (f *HashFile) String() string        { return fmt.Sprintf("hash.file(%q)", f.path) }
func (f *HashFile) Type() string          { return "hash.file" }
func (f *HashFile) Freeze()               {} // immutable
func (f *HashFile) Truth() starlark.Bool  { return starlark.Bool(f.path != "") }
func (f *HashFile) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: hash.file") }

func (f *HashFile) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := f.methodBuiltin(base); method != nil {
			return libkite.TryWrap("hash.file."+name, method), nil
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

func (f *HashFile) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "md5":
		return starlark.NewBuiltin("hash.file.md5", f.hashMethod("md5"))
	case "sha1":
		return starlark.NewBuiltin("hash.file.sha1", f.hashMethod("sha1"))
	case "sha256":
		return starlark.NewBuiltin("hash.file.sha256", f.hashMethod("sha256"))
	case "sha512":
		return starlark.NewBuiltin("hash.file.sha512", f.hashMethod("sha512"))
	}
	return nil
}

func (f *HashFile) AttrNames() []string {
	names := []string{
		"path",
		"md5", "sha1", "sha256", "sha512",
		"try_md5", "try_sha1", "try_sha256", "try_sha512",
	}
	sort.Strings(names)
	return names
}

func (f *HashFile) isDryRun() bool {
	return f.config != nil && f.config.DryRun
}

func (f *HashFile) hashMethod(algo string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(args) > 0 || len(kwargs) > 0 {
			return nil, fmt.Errorf("hash.file.%s: takes no arguments", algo)
		}
		if err := libkite.Check(f.thread, "fs", "read_file", f.path); err != nil {
			return nil, err
		}
		if f.isDryRun() {
			return starlark.String(strings.Repeat("0", hashHexLen(algo))), nil
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, fmt.Errorf("hash.file.%s: %w", algo, err)
		}
		h, err := computeHash(algo, data)
		if err != nil {
			return nil, err
		}
		return starlark.String(h), nil
	}
}
