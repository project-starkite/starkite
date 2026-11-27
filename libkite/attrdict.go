package libkite

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// AttrDict wraps a map for dot-access in Starlark (obj.field or obj["field"]).
// It supports dict-style access and mutation, standard dictionary methods
// (get, keys, values, items, to_dict), startype.DictConvertible for zero-friction
// serialization (json.encode(obj), yaml.encode(obj)), and case-folding lookup.
type AttrDict struct {
	data   map[string]any
	mu     *sync.RWMutex
	frozen *atomic.Bool
}

var (
	_ starlark.Value           = (*AttrDict)(nil)
	_ starlark.HasAttrs        = (*AttrDict)(nil)
	_ starlark.Mapping         = (*AttrDict)(nil)
	_ starlark.IterableMapping = (*AttrDict)(nil)
	_ starlark.Iterable        = (*AttrDict)(nil)
	_ starlark.Sequence        = (*AttrDict)(nil)
	_ starlark.HasSetKey       = (*AttrDict)(nil)
	_ startype.DictConvertible = (*AttrDict)(nil)
)

// NewAttrDict creates an AttrDict wrapping the provided data map.
func NewAttrDict(data map[string]any) *AttrDict {
	if data == nil {
		data = make(map[string]any)
	}
	return &AttrDict{
		data:   data,
		mu:     &sync.RWMutex{},
		frozen: &atomic.Bool{},
	}
}

func (d *AttrDict) String() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	idVal, _ := lookupCaseFold(d.data, "id")
	if id, ok := idVal.(string); ok && id != "" {
		nameVal, _ := lookupCaseFold(d.data, "name")
		name, _ := nameVal.(string)
		shortID := id
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		if name != "" {
			return fmt.Sprintf("AttrDict(id=%q, name=%q)", shortID, name)
		}
		return fmt.Sprintf("AttrDict(id=%q)", shortID)
	}
	return fmt.Sprintf("AttrDict(%d keys)", len(d.data))
}

func (d *AttrDict) Type() string { return "AttrDict" }

func (d *AttrDict) isFrozen() bool {
	return d.frozen != nil && d.frozen.Load()
}

func (d *AttrDict) Freeze() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.frozen == nil {
		d.frozen = &atomic.Bool{}
	}
	d.frozen.Store(true)
	freezeNestedValues(d.data)
}

func freezeNestedValues(m map[string]any) {
	for _, v := range m {
		switch tv := v.(type) {
		case map[string]any:
			freezeNestedValues(tv)
		case []any:
			for _, item := range tv {
				if sub, ok := item.(map[string]any); ok {
					freezeNestedValues(sub)
				} else if sv, ok := item.(starlark.Value); ok {
					sv.Freeze()
				}
			}
		case starlark.Value:
			tv.Freeze()
		}
	}
}

func cloneAnyValue(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return cloneAnyMap(tv)
	case []any:
		out := make([]any, len(tv))
		for i, item := range tv {
			out[i] = cloneAnyValue(item)
		}
		return out
	default:
		return v
	}
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAnyValue(v)
	}
	return out
}

func (d *AttrDict) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: AttrDict")
}

func (d *AttrDict) Truth() starlark.Bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return starlark.Bool(len(d.data) > 0)
}

func (d *AttrDict) ToMap() map[string]any {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return cloneAnyMap(d.data)
}

func (d *AttrDict) ToDict() *starlark.Dict {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return mapToPureStarlarkDict(d.data, d.isFrozen())
}

func mapToPureStarlarkDict(data map[string]any, frozen bool) *starlark.Dict {
	dict := starlark.NewDict(len(data))
	keys := slices.Sorted(maps.Keys(data))
	for _, k := range keys {
		_ = dict.SetKey(starlark.String(k), goToPureStarlarkValue(data[k], frozen))
	}
	if frozen {
		dict.Freeze()
	}
	return dict
}

func goToPureStarlarkValue(val any, frozen bool) starlark.Value {
	if val == nil {
		return starlark.None
	}
	switch v := val.(type) {
	case map[string]any:
		return mapToPureStarlarkDict(v, frozen)
	case *AttrDict:
		return v.ToDict()
	case []any:
		elems := make([]starlark.Value, len(v))
		for i, item := range v {
			elems[i] = goToPureStarlarkValue(item, frozen)
		}
		list := starlark.NewList(elems)
		if frozen {
			list.Freeze()
		}
		return list
	case starlark.Value:
		if frozen {
			v.Freeze()
		}
		return v
	default:
		sv, err := startype.Go(val).ToStarlarkValue()
		if err != nil {
			return starlark.String(fmt.Sprintf("%v", val))
		}
		if frozen {
			sv.Freeze()
		}
		return sv
	}
}

func (d *AttrDict) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.data)
}

func (d *AttrDict) Iterate() starlark.Iterator {
	d.mu.RLock()
	defer d.mu.RUnlock()
	keys := make([]starlark.Value, 0, len(d.data))
	for k := range d.data {
		keys = append(keys, starlark.String(k))
	}
	return starlark.NewList(keys).Iterate()
}

func (d *AttrDict) Items() []starlark.Tuple {
	d.mu.RLock()
	defer d.mu.RUnlock()
	items := make([]starlark.Tuple, 0, len(d.data))
	for k, v := range d.data {
		items = append(items, starlark.Tuple{starlark.String(k), goToStarlarkValue(v, d.mu, d.frozen)})
	}
	return items
}

func (d *AttrDict) Attr(name string) (starlark.Value, error) {
	switch name {
	case "get":
		return starlark.NewBuiltin("AttrDict.get", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var key starlark.Value
			var defaultVal starlark.Value = starlark.None
			if err := starlark.UnpackPositionalArgs("get", args, kwargs, 1, &key, &defaultVal); err != nil {
				return nil, err
			}
			s, ok := starlark.AsString(key)
			if !ok {
				return defaultVal, nil
			}
			d.mu.RLock()
			defer d.mu.RUnlock()
			if val, ok := lookupCaseFold(d.data, s); ok {
				return goToStarlarkValue(val, d.mu, d.frozen), nil
			}
			return defaultVal, nil
		}), nil
	case "keys":
		return starlark.NewBuiltin("AttrDict.keys", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs("keys", args, kwargs, 0); err != nil {
				return nil, err
			}
			d.mu.RLock()
			defer d.mu.RUnlock()
			keys := make([]starlark.Value, 0, len(d.data))
			for k := range d.data {
				keys = append(keys, starlark.String(k))
			}
			return starlark.NewList(keys), nil
		}), nil
	case "values":
		return starlark.NewBuiltin("AttrDict.values", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs("values", args, kwargs, 0); err != nil {
				return nil, err
			}
			d.mu.RLock()
			defer d.mu.RUnlock()
			vals := make([]starlark.Value, 0, len(d.data))
			for _, v := range d.data {
				vals = append(vals, goToStarlarkValue(v, d.mu, d.frozen))
			}
			return starlark.NewList(vals), nil
		}), nil
	case "items":
		return starlark.NewBuiltin("AttrDict.items", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs("items", args, kwargs, 0); err != nil {
				return nil, err
			}
			d.mu.RLock()
			defer d.mu.RUnlock()
			items := make([]starlark.Value, 0, len(d.data))
			for k, v := range d.data {
				items = append(items, starlark.Tuple{starlark.String(k), goToStarlarkValue(v, d.mu, d.frozen)})
			}
			return starlark.NewList(items), nil
		}), nil
	case "to_dict":
		return starlark.NewBuiltin("AttrDict.to_dict", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			if err := starlark.UnpackPositionalArgs("to_dict", args, kwargs, 0); err != nil {
				return nil, err
			}
			return d.ToDict(), nil
		}), nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	val, ok := lookupCaseFold(d.data, name)
	if !ok {
		return starlark.None, nil
	}
	return goToStarlarkValue(val, d.mu, d.frozen), nil
}

func (d *AttrDict) AttrNames() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	seen := map[string]bool{
		"get":     true,
		"keys":    true,
		"values":  true,
		"items":   true,
		"to_dict": true,
	}
	names := make([]string, 0, len(d.data)+5)
	names = append(names, "get", "keys", "values", "items", "to_dict")
	for k := range d.data {
		if !seen[k] {
			seen[k] = true
			names = append(names, k)
		}
	}
	slices.Sort(names)
	return names
}

// Get implements starlark.Mapping for dict-style read: obj["key"]
func (d *AttrDict) Get(key starlark.Value) (v starlark.Value, found bool, err error) {
	s, ok := starlark.AsString(key)
	if !ok {
		return starlark.None, false, nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	val, ok := lookupCaseFold(d.data, s)
	if !ok {
		return starlark.None, false, nil
	}
	return goToStarlarkValue(val, d.mu, d.frozen), true, nil
}

func (d *AttrDict) SetKey(k, v starlark.Value) error {
	key, ok := starlark.AsString(k)
	if !ok {
		return fmt.Errorf("AttrDict key must be string, got %s", k.Type())
	}
	goVal, err := ValueToGo(v)
	if err != nil {
		return fmt.Errorf("AttrDict SetKey: %w", err)
	}
	return d.Set(key, goVal)
}

// Set sets a key-value pair directly in Go, respecting Freeze.
func (d *AttrDict) Set(key string, val any) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.isFrozen() {
		return fmt.Errorf("cannot modify frozen AttrDict")
	}
	d.data[key] = val
	return nil
}

// GetField retrieves a field by name using case-folding lookup in Go.
func (d *AttrDict) GetField(name string) (any, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return lookupCaseFold(d.data, name)
}

func lookupCaseFold(data map[string]any, name string) (any, bool) {
	if val, ok := data[name]; ok {
		return val, true
	}
	nameLower := strings.ToLower(name)
	if val, ok := data[nameLower]; ok {
		return val, true
	}
	for k, v := range data {
		if strings.ToLower(k) == nameLower {
			return v, true
		}
	}
	return nil, false
}

func goToStarlarkValue(val any, mu *sync.RWMutex, frozen *atomic.Bool) starlark.Value {
	if val == nil {
		return starlark.None
	}
	isFrozen := frozen != nil && frozen.Load()
	switch v := val.(type) {
	case map[string]any:
		return &AttrDict{data: v, mu: mu, frozen: frozen}
	case []any:
		elems := make([]starlark.Value, len(v))
		for i, item := range v {
			elems[i] = goToStarlarkValue(item, mu, frozen)
		}
		list := starlark.NewList(elems)
		if isFrozen {
			list.Freeze()
		}
		return list
	case starlark.Value:
		if isFrozen {
			v.Freeze()
		}
		return v
	default:
		sv, err := startype.Go(val).ToStarlarkValue()
		if err != nil {
			return starlark.String(fmt.Sprintf("%v", val))
		}
		if isFrozen {
			sv.Freeze()
		}
		return sv
	}
}
