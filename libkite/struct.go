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
// Go serialization adapters handle them uniformly.
// Values containing no structs are returned unchanged.
func ExpandStructs(v starlark.Value) (starlark.Value, error) {
	expanded, _, err := expandStructValue(v, 0)
	return expanded, err
}

// ValueToDict coerces a Starlark dict, struct, or startype.DictConvertible to
// a *starlark.Dict (with any nested structs expanded). Non-mapping values
// (including None) return (nil, false, nil).
func ValueToDict(v starlark.Value) (*starlark.Dict, bool, error) {
	switch val := v.(type) {
	case nil, starlark.NoneType:
		return nil, false, nil
	case *starlark.Dict:
		expanded, err := ExpandStructs(val)
		if err != nil {
			return nil, false, err
		}
		return expanded.(*starlark.Dict), true, nil
	case *starlarkstruct.Struct:
		expanded, err := ExpandStructs(val)
		if err != nil {
			return nil, false, err
		}
		return expanded.(*starlark.Dict), true, nil
	case startype.DictConvertible:
		expanded, err := ExpandStructs(val.ToDict())
		if err != nil {
			return nil, false, err
		}
		return expanded.(*starlark.Dict), true, nil
	default:
		return nil, false, nil
	}
}

// ValueToMap coerces a Starlark dict, struct, *AttrDict, or
// startype.DictConvertible directly into map[string]any without intermediate
// *starlark.Dict allocations for structs. Non-mapping values (including None)
// return (nil, false, nil).
func ValueToMap(v starlark.Value) (map[string]any, bool, error) {
	switch val := v.(type) {
	case nil, starlark.NoneType:
		return nil, false, nil
	case *AttrDict:
		return val.ToMap(), true, nil
	case *starlarkstruct.Struct:
		m, err := structToGoMap(val, 0)
		return m, err == nil, err
	case *starlark.Dict:
		m, err := dictToGoMap(val, 0)
		return m, err == nil, err
	case startype.DictConvertible:
		m, err := dictToGoMap(val.ToDict(), 0)
		return m, err == nil, err
	default:
		return nil, false, nil
	}
}

// ValueToGo converts any Starlark value into idiomatic Go values, mapping both
// *starlark.Dict and *starlarkstruct.Struct into map[string]any recursively.
func ValueToGo(v starlark.Value) (any, error) {
	return starlarkToGoValue(v, 0)
}

func starlarkToGoValue(v starlark.Value, depth int) (any, error) {
	switch val := v.(type) {
	case nil, starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(val), nil
	case starlark.Int:
		if i, ok := val.Int64(); ok {
			return i, nil
		}
		if u, ok := val.Uint64(); ok {
			return u, nil
		}
		return float64(val.Float()), nil
	case starlark.Float:
		return float64(val), nil
	case starlark.String:
		return string(val), nil
	case starlark.Bytes:
		return []byte(val), nil
	case *starlarkstruct.Struct:
		return structToGoMap(val, depth)
	case *starlark.Dict:
		return dictToGoMap(val, depth)
	case *AttrDict:
		return val.ToMap(), nil
	case startype.DictConvertible:
		return dictToGoMap(val.ToDict(), depth)
	case *starlark.List:
		if depth >= maxStructExpandDepth {
			return nil, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
		}
		out := make([]any, val.Len())
		for i := range val.Len() {
			item, err := starlarkToGoValue(val.Index(i), depth+1)
			if err != nil {
				return nil, fmt.Errorf("list[%d]: %w", i, err)
			}
			out[i] = item
		}
		return out, nil
	case starlark.Tuple:
		if depth >= maxStructExpandDepth {
			return nil, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
		}
		out := make([]any, len(val))
		for i, elem := range val {
			item, err := starlarkToGoValue(elem, depth+1)
			if err != nil {
				return nil, fmt.Errorf("tuple[%d]: %w", i, err)
			}
			out[i] = item
		}
		return out, nil
	default:
		var goVal any
		if err := startype.Starlark(v).Go(&goVal); err != nil {
			return nil, err
		}
		return goVal, nil
	}
}

func structToGoMap(s *starlarkstruct.Struct, depth int) (map[string]any, error) {
	if depth >= maxStructExpandDepth {
		return nil, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
	}
	names := s.AttrNames()
	out := make(map[string]any, len(names))
	for _, name := range names {
		fv, err := s.Attr(name)
		if err != nil {
			return nil, fmt.Errorf("struct field %q: %w", name, err)
		}
		goVal, err := starlarkToGoValue(fv, depth+1)
		if err != nil {
			return nil, fmt.Errorf("struct.%s: %w", name, err)
		}
		out[name] = goVal
	}
	return out, nil
}

func dictToGoMap(d *starlark.Dict, depth int) (map[string]any, error) {
	if depth >= maxStructExpandDepth {
		return nil, fmt.Errorf("struct expansion exceeded maximum nesting depth (%d): possible cyclic reference", maxStructExpandDepth)
	}
	items := d.Items()
	out := make(map[string]any, len(items))
	for _, kv := range items {
		k, ok := starlark.AsString(kv[0])
		if !ok {
			return nil, fmt.Errorf("dict key must be string, got %s", kv[0].Type())
		}
		goVal, err := starlarkToGoValue(kv[1], depth+1)
		if err != nil {
			return nil, fmt.Errorf("dict[%q]: %w", k, err)
		}
		out[k] = goVal
	}
	return out, nil
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
