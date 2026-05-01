package hash

import (
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Source is a Starlark value wrapping data for hashing.
type Source struct {
	data   []byte
	thread *starlark.Thread
	config *libkite.ModuleConfig
}

var (
	_ starlark.Value    = (*Source)(nil)
	_ starlark.HasAttrs = (*Source)(nil)
)

func (s *Source) String() string {
	return fmt.Sprintf("hash.source(%d bytes)", len(s.data))
}
func (s *Source) Type() string          { return "hash.source" }
func (s *Source) Freeze()               {} // immutable
func (s *Source) Truth() starlark.Bool  { return starlark.Bool(len(s.data) > 0) }
func (s *Source) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: hash.source") }

// Attr implements starlark.HasAttrs.
func (s *Source) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := s.methodBuiltin(base); method != nil {
			return libkite.TryWrap("hash.source."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	if name == "data" {
		return starlark.Bytes(s.data), nil
	}

	// Methods
	if method := s.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (s *Source) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "md5":
		return starlark.NewBuiltin("hash.source.md5", s.hashMethod("md5"))
	case "sha1":
		return starlark.NewBuiltin("hash.source.sha1", s.hashMethod("sha1"))
	case "sha256":
		return starlark.NewBuiltin("hash.source.sha256", s.hashMethod("sha256"))
	case "sha512":
		return starlark.NewBuiltin("hash.source.sha512", s.hashMethod("sha512"))
	}
	return nil
}

// AttrNames implements starlark.HasAttrs.
func (s *Source) AttrNames() []string {
	names := []string{
		"data",
		"md5", "sha1", "sha256", "sha512",
		"try_md5", "try_sha1", "try_sha256", "try_sha512",
	}
	sort.Strings(names)
	return names
}

func (s *Source) hashMethod(algo string) func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
	return func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(args) > 0 || len(kwargs) > 0 {
			return nil, fmt.Errorf("hash.source.%s: takes no arguments", algo)
		}
		h, err := computeHash(algo, s.data)
		if err != nil {
			return nil, err
		}
		return starlark.String(h), nil
	}
}
