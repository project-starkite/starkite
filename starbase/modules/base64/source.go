package base64

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

// Source is a Starlark value wrapping data for base64 encoding/decoding.
type Source struct {
	data   []byte
	thread *starlark.Thread
	config *starbase.ModuleConfig
}

var (
	_ starlark.Value    = (*Source)(nil)
	_ starlark.HasAttrs = (*Source)(nil)
)

func (s *Source) String() string {
	return fmt.Sprintf("base64.source(%d bytes)", len(s.data))
}
func (s *Source) Type() string          { return "base64.source" }
func (s *Source) Freeze()               {} // immutable
func (s *Source) Truth() starlark.Bool  { return starlark.Bool(len(s.data) > 0) }
func (s *Source) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: base64.source") }

// Attr implements starlark.HasAttrs.
func (s *Source) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := s.methodBuiltin(base); method != nil {
			return starbase.TryWrap("base64.source."+name, method), nil
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
	case "encode":
		return starlark.NewBuiltin("base64.source.encode", s.encodeMethod)
	case "decode":
		return starlark.NewBuiltin("base64.source.decode", s.decodeMethod)
	case "encode_url":
		return starlark.NewBuiltin("base64.source.encode_url", s.encodeURLMethod)
	case "decode_url":
		return starlark.NewBuiltin("base64.source.decode_url", s.decodeURLMethod)
	}
	return nil
}

// AttrNames implements starlark.HasAttrs.
func (s *Source) AttrNames() []string {
	names := []string{
		"data",
		"decode", "decode_url", "encode", "encode_url",
		"try_decode", "try_decode_url", "try_encode", "try_encode_url",
	}
	sort.Strings(names)
	return names
}

func (s *Source) encodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.source.encode: takes no arguments")
	}
	return starlark.String(base64.StdEncoding.EncodeToString(s.data)), nil
}

func (s *Source) decodeMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.source.decode: takes no arguments")
	}
	decoded, err := base64.StdEncoding.DecodeString(string(s.data))
	if err != nil {
		return nil, fmt.Errorf("base64.source.decode: %w", err)
	}
	return starlark.Bytes(decoded), nil
}

func (s *Source) encodeURLMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.source.encode_url: takes no arguments")
	}
	return starlark.String(base64.URLEncoding.EncodeToString(s.data)), nil
}

func (s *Source) decodeURLMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("base64.source.decode_url: takes no arguments")
	}
	decoded, err := base64.URLEncoding.DecodeString(string(s.data))
	if err != nil {
		return nil, fmt.Errorf("base64.source.decode_url: %w", err)
	}
	return starlark.Bytes(decoded), nil
}
