package containers

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// AttrDict wraps a map for dot-access in Starlark (box.id, box.name, box.status).
// It also supports dict-style access (box["id"]), standard dictionary methods
// (get, keys, values, items, to_dict), and startype.DictConvertible for zero-friction
// serialization (json.encode(box), yaml.encode(box)).
type AttrDict struct {
	data map[string]any
	mu   *sync.RWMutex
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
	return &AttrDict{data: data, mu: &sync.RWMutex{}}
}

func (d *AttrDict) String() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if id, ok := d.data["id"].(string); ok && id != "" {
		name, _ := d.data["name"].(string)
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
func (d *AttrDict) Freeze()      {}
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
	return d.data
}

func (d *AttrDict) ToDict() *starlark.Dict {
	d.mu.RLock()
	defer d.mu.RUnlock()
	dict, _ := startype.Map(d.data).ToDict()
	return dict
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
		items = append(items, starlark.Tuple{starlark.String(k), goToStarlarkValue(v, d.mu)})
	}
	return items
}

func (d *AttrDict) Attr(name string) (starlark.Value, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

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
			if val, ok := d.data[s]; ok {
				return goToStarlarkValue(val, d.mu), nil
			}
			if val, ok := d.data[strings.ToLower(s)]; ok {
				return goToStarlarkValue(val, d.mu), nil
			}
			return defaultVal, nil
		}), nil
	case "keys":
		return starlark.NewBuiltin("AttrDict.keys", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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
			d.mu.RLock()
			defer d.mu.RUnlock()
			vals := make([]starlark.Value, 0, len(d.data))
			for _, v := range d.data {
				vals = append(vals, goToStarlarkValue(v, d.mu))
			}
			return starlark.NewList(vals), nil
		}), nil
	case "items":
		return starlark.NewBuiltin("AttrDict.items", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			d.mu.RLock()
			defer d.mu.RUnlock()
			items := make([]starlark.Value, 0, len(d.data))
			for k, v := range d.data {
				items = append(items, starlark.Tuple{starlark.String(k), goToStarlarkValue(v, d.mu)})
			}
			return starlark.NewList(items), nil
		}), nil
	case "to_dict":
		return starlark.NewBuiltin("AttrDict.to_dict", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			return d.ToDict(), nil
		}), nil
	}

	val, ok := d.data[name]
	if !ok {
		val, ok = d.data[strings.ToLower(name)]
	}
	if !ok {
		return starlark.None, nil
	}
	return goToStarlarkValue(val, d.mu), nil
}

func (d *AttrDict) AttrNames() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	names := make([]string, 0, len(d.data)+5)
	names = append(names, "get", "keys", "values", "items", "to_dict")
	for k := range d.data {
		names = append(names, k)
	}
	sort.Strings(names)
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
	val, ok := d.data[s]
	if !ok {
		val, ok = d.data[strings.ToLower(s)]
	}
	if !ok {
		return starlark.None, false, nil
	}
	return goToStarlarkValue(val, d.mu), true, nil
}

// SetKey implements starlark.HasSetKey for dict-style write: obj["key"] = value
func (d *AttrDict) SetKey(k, v starlark.Value) error {
	key, ok := starlark.AsString(k)
	if !ok {
		return fmt.Errorf("AttrDict key must be string, got %s", k.Type())
	}
	var goVal any
	if err := startype.Starlark(v).Go(&goVal); err != nil {
		return fmt.Errorf("AttrDict SetKey: %w", err)
	}
	d.mu.Lock()
	d.data[key] = goVal
	d.mu.Unlock()
	return nil
}

// goToStarlarkValue converts a Go value to a Starlark value.
// Maps are wrapped as nested AttrDict (sharing the root mutex).
func goToStarlarkValue(val any, mu *sync.RWMutex) starlark.Value {
	if val == nil {
		return starlark.None
	}
	switch v := val.(type) {
	case map[string]any:
		return &AttrDict{data: v, mu: mu}
	case []any:
		elems := make([]starlark.Value, len(v))
		for i, item := range v {
			elems[i] = goToStarlarkValue(item, mu)
		}
		return starlark.NewList(elems)
	default:
		sv, err := startype.Go(val).ToStarlarkValue()
		if err != nil {
			return starlark.String(fmt.Sprintf("%v", val))
		}
		return sv
	}
}

// newContainerAttrDict constructs an AttrDict representation of a container.
func newContainerAttrDict(id, name, image, status string, ports map[string]any) *AttrDict {
	cleanName := strings.TrimPrefix(name, "/")
	if ports == nil {
		ports = make(map[string]any)
	}
	data := map[string]any{
		"id":     id,
		"name":   cleanName,
		"image":  image,
		"status": status,
		"ports":  ports,
	}
	return NewAttrDict(data)
}

// extractTargetID extracts a container ID or name string from an AttrDict, dict, or string.
func extractTargetID(target starlark.Value) (string, error) {
	if target == nil || target == starlark.None {
		return "", fmt.Errorf("containers: target container cannot be None")
	}
	switch v := target.(type) {
	case starlark.String:
		s := string(v)
		if s == "" {
			return "", fmt.Errorf("containers: target container ID/name cannot be empty")
		}
		return s, nil
	case *AttrDict:
		v.mu.RLock()
		defer v.mu.RUnlock()
		if id, ok := v.data["id"].(string); ok && id != "" {
			return id, nil
		}
		if id, ok := v.data["Id"].(string); ok && id != "" {
			return id, nil
		}
		if name, ok := v.data["name"].(string); ok && name != "" {
			return name, nil
		}
		if name, ok := v.data["Name"].(string); ok && name != "" {
			return name, nil
		}
		return "", fmt.Errorf("containers: target AttrDict has no 'id' or 'name' field")
	case *starlark.Dict:
		for _, key := range []string{"id", "Id", "name", "Name"} {
			if val, found, _ := v.Get(starlark.String(key)); found {
				if s, ok := starlark.AsString(val); ok && s != "" {
					return s, nil
				}
			}
		}
		return "", fmt.Errorf("containers: target dict has no 'id' or 'name' field")
	case *Container:
		if v.id != "" {
			return v.id, nil
		}
		return v.name, nil
	default:
		return "", fmt.Errorf("containers: expected container AttrDict/dict or string ID/name, got %s", target.Type())
	}
}

// extractTargetName extracts a container name or returns a fallback string.
func extractTargetName(target starlark.Value, fallback string) string {
	switch v := target.(type) {
	case *AttrDict:
		v.mu.RLock()
		defer v.mu.RUnlock()
		if name, ok := v.data["name"].(string); ok && name != "" {
			return name
		}
		if name, ok := v.data["Name"].(string); ok && name != "" {
			return name
		}
	case *starlark.Dict:
		for _, key := range []string{"name", "Name"} {
			if val, found, _ := v.Get(starlark.String(key)); found {
				if s, ok := starlark.AsString(val); ok && s != "" {
					return s
				}
			}
		}
	case *Container:
		if v.name != "" {
			return v.name
		}
	}
	return fallback
}
