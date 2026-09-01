package fleet

import (
	"fmt"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
)

// Fleet represents an immutable collection of compute resources.
type Fleet struct {
	resources []Resource
}

// New creates a new Fleet with the given resources.
func New(resources []Resource) *Fleet {
	copied := make([]Resource, len(resources))
	copy(copied, resources)
	return &Fleet{resources: copied}
}

// Resources returns a copy of the slice of resources.
func (f *Fleet) Resources() []Resource {
	copied := make([]Resource, len(f.resources))
	copy(copied, f.resources)
	return copied
}

// String returns the string representation of the fleet.
func (f *Fleet) String() string { return fmt.Sprintf("<fleet count=%d>", len(f.resources)) }

// Type returns the Starlark type name.
func (f *Fleet) Type() string { return "fleet" }

// Freeze marks the fleet as immutable.
func (f *Fleet) Freeze() {}

// Truth returns whether the fleet is non-empty.
func (f *Fleet) Truth() starlark.Bool { return starlark.Bool(len(f.resources) > 0) }

// Hash returns an unhashable error for Fleet.
func (f *Fleet) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: fleet") }

// Attr exposes attributes and methods to Starlark.
func (f *Fleet) Attr(name string) (starlark.Value, error) {
	switch name {
	case "count":
		return starlark.MakeInt(len(f.resources)), nil
	case "items":
		return f.toStarlarkList(), nil
	case "filter":
		return starlark.NewBuiltin("fleet.filter", f.filterBuiltin), nil
	case "group_by":
		return starlark.NewBuiltin("fleet.group_by", f.groupByBuiltin), nil
	case "addresses":
		return starlark.NewBuiltin("fleet.addresses", f.addressesBuiltin), nil
	case "names":
		return starlark.NewBuiltin("fleet.names", f.namesBuiltin), nil
	case "ids":
		return starlark.NewBuiltin("fleet.ids", f.idsBuiltin), nil
	case "first":
		return starlark.NewBuiltin("fleet.first", f.firstBuiltin), nil
	default:
		return nil, nil
	}
}

// AttrNames lists all accessible attributes and methods.
func (f *Fleet) AttrNames() []string {
	return []string{"addresses", "count", "filter", "first", "group_by", "ids", "items", "names"}
}

func (f *Fleet) toStarlarkList() starlark.Value {
	elems := make([]starlark.Value, len(f.resources))
	for i, r := range f.resources {
		elems[i] = r.ToStarlarkDict()
	}
	return starlark.NewList(elems)
}

// filterBuiltin filters the fleet by exact keyword matches or predicate function.
// Usage: fleet.filter(role="web", env="prod") OR fleet.filter(lambda r: r.get("cpu", 0) >= 8)
func (f *Fleet) filterBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var predicateFn starlark.Callable

	if len(args) > 1 {
		return nil, fmt.Errorf("fleet.filter expects at most 1 positional argument (predicate function), got %d", len(args))
	}
	if len(args) == 1 {
		var ok bool
		predicateFn, ok = args[0].(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("fleet.filter: positional argument must be a callable predicate function, got %s", args[0].Type())
		}
	}

	var filtered []Resource

	for _, r := range f.resources {
		include := true

		if predicateFn != nil {
			dict := r.ToStarlarkDict()
			res, err := starlark.Call(thread, predicateFn, starlark.Tuple{dict}, nil)
			if err != nil {
				return nil, fmt.Errorf("fleet.filter: predicate execution failed: %w", err)
			}
			include = bool(res.Truth())
		} else if len(kwargs) > 0 {
			for _, kv := range kwargs {
				k := string(kv[0].(starlark.String))
				var expectedVal any
				if err := startype.Starlark(kv[1]).Go(&expectedVal); err != nil {
					expectedVal = kv[1].String()
				}
				expectedStr := fmt.Sprintf("%v", expectedVal)

				// Match against primary struct fields or labels or data
				var actualVal string
				var found bool

				switch k {
				case "name":
					actualVal = r.Name
					found = true
				case "address":
					actualVal = r.Address
					found = true
				case "id":
					actualVal = r.ID
					found = true
				case "kind":
					actualVal = r.Kind
					found = true
				default:
					if val, ok := r.Labels[k]; ok {
						actualVal = val
						found = true
					} else if val, ok := r.Data[k]; ok {
						actualVal = fmt.Sprintf("%v", val)
						found = true
					}
				}

				if !found || actualVal != expectedStr {
					include = false
					break
				}
			}
		}

		if include {
			filtered = append(filtered, r)
		}
	}

	return New(filtered), nil
}

// groupByBuiltin groups resources by attribute value into a dictionary of sub-fleets.
// Usage: fleet.group_by("role")
func (f *Fleet) groupByBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var key string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "key", &key); err != nil {
		return nil, err
	}

	groups := make(map[string][]Resource)
	for _, r := range f.resources {
		var groupVal string
		switch key {
		case "name":
			groupVal = r.Name
		case "address":
			groupVal = r.Address
		case "id":
			groupVal = r.ID
		case "kind":
			groupVal = r.Kind
		default:
			if val, ok := r.Labels[key]; ok {
				groupVal = val
			} else if val, ok := r.Data[key]; ok {
				groupVal = fmt.Sprintf("%v", val)
			} else {
				groupVal = ""
			}
		}
		groups[groupVal] = append(groups[groupVal], r)
	}

	result := starlark.NewDict(len(groups))
	for groupName, groupItems := range groups {
		result.SetKey(starlark.String(groupName), New(groupItems))
	}
	return result, nil
}

// addressesBuiltin extracts all resource addresses into a list of strings.
// Usage: fleet.addresses() OR fleet.addresses(key="private_ip")
func (f *Fleet) addressesBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	key := "address"
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "key?", &key); err != nil {
		return nil, err
	}

	addrs := make([]starlark.Value, 0, len(f.resources))
	for _, r := range f.resources {
		if key == "address" {
			addrs = append(addrs, starlark.String(r.Address))
		} else if val, ok := r.Labels[key]; ok {
			addrs = append(addrs, starlark.String(val))
		} else if val, ok := r.Data[key]; ok {
			addrs = append(addrs, starlark.String(fmt.Sprintf("%v", val)))
		}
	}
	return starlark.NewList(addrs), nil
}

// namesBuiltin extracts all resource names into a list of strings.
// Usage: fleet.names()
func (f *Fleet) namesBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("names() takes no arguments")
	}
	names := make([]starlark.Value, len(f.resources))
	for i, r := range f.resources {
		names[i] = starlark.String(r.Name)
	}
	return starlark.NewList(names), nil
}

// idsBuiltin extracts all resource IDs into a list of strings.
// Usage: fleet.ids()
func (f *Fleet) idsBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("ids() takes no arguments")
	}
	ids := make([]starlark.Value, len(f.resources))
	for i, r := range f.resources {
		ids[i] = starlark.String(r.ID)
	}
	return starlark.NewList(ids), nil
}

// firstBuiltin returns the first resource as a dictionary, or None if the fleet is empty.
// Usage: fleet.first()
func (f *Fleet) firstBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 || len(kwargs) > 0 {
		return nil, fmt.Errorf("first() takes no arguments")
	}
	if len(f.resources) == 0 {
		return starlark.None, nil
	}
	return f.resources[0].ToStarlarkDict(), nil
}
