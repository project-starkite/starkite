package libkite

import (
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const maxStructExpandDepth = 64

// ExpandStructs recursively converts any *starlarkstruct.Struct values (including
// values nested inside dicts, lists, and tuples) into *starlark.Dict values so
// Go serialization adapters (JSON, YAML, templates, HTTP) handle them uniformly.
// Values containing no structs are returned unchanged.
func ExpandStructs(v starlark.Value) (starlark.Value, error) {
	expanded, _, err := expandStructValue(v, 0)
	return expanded, err
}

func expandStructValue(v starlark.Value, depth int) (starlark.Value, bool, error) {
	switch val := v.(type) {
	case nil, starlark.NoneType, starlark.Bool, starlark.Int, starlark.Float, starlark.String, starlark.Bytes:
		return v, false, nil
	case *starlarkstruct.Struct:
		if depth >= maxStructExpandDepth {
			return nil, false, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
		}
		names := val.AttrNames()
		out := starlark.NewDict(len(names))
		for _, name := range names {
			fv, err := val.Attr(name)
			if err != nil {
				return nil, false, fmt.Errorf("struct field %q: %w", name, err)
			}
			exp, _, err := expandStructValue(fv, depth+1)
			if err != nil {
				return nil, false, err
			}
			if err := out.SetKey(starlark.String(name), exp); err != nil {
				return nil, false, err
			}
		}
		return out, true, nil
	case *starlark.Dict:
		if depth >= maxStructExpandDepth {
			return nil, false, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
		}
		return expandStructDict(val, depth)
	case *starlark.List:
		if depth >= maxStructExpandDepth {
			return nil, false, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
		}
		elems, changed, err := expandStructSeq(val.Len(), val.Index, depth)
		if err != nil || !changed {
			return v, false, err
		}
		return starlark.NewList(elems), true, nil
	case starlark.Tuple:
		if depth >= maxStructExpandDepth {
			return nil, false, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
		}
		elems, changed, err := expandStructSeq(len(val), func(i int) starlark.Value { return val[i] }, depth)
		if err != nil || !changed {
			return v, false, err
		}
		return starlark.Tuple(elems), true, nil
	case startype.DictConvertible:
		return v, false, nil
	default:
		return v, false, nil
	}
}

func expandStructDict(d *starlark.Dict, depth int) (starlark.Value, bool, error) {
	items := d.Items()
	var out *starlark.Dict
	for i, kv := range items {
		exp, changed, err := expandStructValue(kv[1], depth+1)
		if err != nil {
			return nil, false, err
		}
		if changed && out == nil {
			out = starlark.NewDict(len(items))
			for j := range i {
				if err := out.SetKey(items[j][0], items[j][1]); err != nil {
					return nil, false, err
				}
			}
		}
		if out != nil {
			if err := out.SetKey(kv[0], exp); err != nil {
				return nil, false, err
			}
		}
	}
	if out == nil {
		return d, false, nil
	}
	return out, true, nil
}

func expandStructSeq(n int, at func(int) starlark.Value, depth int) ([]starlark.Value, bool, error) {
	var elems []starlark.Value
	for i := range n {
		exp, changed, err := expandStructValue(at(i), depth+1)
		if err != nil {
			return nil, false, err
		}
		if changed && elems == nil {
			elems = make([]starlark.Value, n)
			for j := range i {
				elems[j] = at(j)
			}
		}
		if elems != nil {
			elems[i] = exp
		}
	}
	return elems, elems != nil, nil
}

func unboundGlobalBuiltin(name string) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(*starlark.Thread, *starlark.Builtin, starlark.Tuple, []starlark.Tuple) (starlark.Value, error) {
		return nil, fmt.Errorf("%s must be called inside a Starkite runtime", name)
	})
}

// RuntimeGlobalBuiltins returns descriptors for runtime-level predeclared
// builtins so the documentation catalog and language server can index them.
func RuntimeGlobalBuiltins() starlark.StringDict {
	return starlark.StringDict{
		"struct":    starlark.NewBuiltin("struct", starlarkstruct.Make),
		"Result":    unboundGlobalBuiltin("Result"),
		"defer":     unboundGlobalBuiltin("defer"),
		"on_signal": unboundGlobalBuiltin("on_signal"),
		"fail":      unboundGlobalBuiltin("fail"),
		"exit":      unboundGlobalBuiltin("exit"),
	}
}
