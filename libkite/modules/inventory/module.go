// Package inventory provides resource discovery and management for starkite.
package inventory

import (
	"fmt"
	"os"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gopkg.in/yaml.v3"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "inventory"

// Module implements inventory/resource discovery.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "inventory provides resource discovery: file, list, filter, group_by, merge, addresses"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"file":      starlark.NewBuiltin("inventory.file", m.loadFile),
				"list":      starlark.NewBuiltin("inventory.list", m.list),
				"filter":    starlark.NewBuiltin("inventory.filter", m.filter),
				"group_by":  starlark.NewBuiltin("inventory.group_by", m.groupBy),
				"merge":     starlark.NewBuiltin("inventory.merge", m.merge),
				"addresses": starlark.NewBuiltin("inventory.addresses", m.addresses),
			},
		}
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

// InventoryValue represents an inventory of hosts/resources.
type InventoryValue struct {
	items []map[string]interface{}
}

func (inv *InventoryValue) String() string {
	return fmt.Sprintf("<inventory %d items>", len(inv.items))
}
func (inv *InventoryValue) Type() string          { return "inventory" }
func (inv *InventoryValue) Freeze()               {}
func (inv *InventoryValue) Truth() starlark.Bool  { return starlark.Bool(len(inv.items) > 0) }
func (inv *InventoryValue) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: inventory") }

func (inv *InventoryValue) Attr(name string) (starlark.Value, error) {
	switch name {
	case "count":
		return starlark.MakeInt(len(inv.items)), nil
	case "items":
		return inv.toStarlarkList(), nil
	default:
		return nil, nil
	}
}

func (inv *InventoryValue) AttrNames() []string {
	return []string{"count", "items"}
}

func (inv *InventoryValue) toStarlarkList() starlark.Value {
	elems := make([]starlark.Value, len(inv.items))
	for i, item := range inv.items {
		dict := starlark.NewDict(len(item))
		for k, v := range item {
			var starVal starlark.Value
			if err := startype.Go(v).Starlark(&starVal); err != nil {
				dict.SetKey(starlark.String(k), starlark.String(fmt.Sprintf("%v", v)))
				continue
			}
			dict.SetKey(starlark.String(k), starVal)
		}
		elems[i] = dict
	}
	return starlark.NewList(elems)
}

// loadFile loads an inventory from a YAML file.
// Usage: inventory.file("hosts.yaml")
func (m *Module) loadFile(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Path string `name:"path" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, err
	}

	var items []map[string]interface{}
	if err := yaml.Unmarshal(data, &items); err != nil {
		// Try parsing as a dict with groups
		var grouped map[string][]map[string]interface{}
		if err2 := yaml.Unmarshal(data, &grouped); err2 != nil {
			return nil, fmt.Errorf("failed to parse inventory file: %w", err)
		}
		// Flatten grouped inventory
		for group, groupItems := range grouped {
			for _, item := range groupItems {
				item["_group"] = group
				items = append(items, item)
			}
		}
	}

	return &InventoryValue{items: items}, nil
}

// list lists all items in an inventory.
// Usage: inventory.list(inv)
func (m *Module) list(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inv *InventoryValue
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "inventory", &inv); err != nil {
		return nil, err
	}
	return inv.toStarlarkList(), nil
}

// filter filters inventory items.
// Usage: inventory.filter(inv, func) or inventory.filter(inv, key="value")
func (m *Module) filter(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inv *InventoryValue
	var filterFn starlark.Callable

	if len(args) >= 2 {
		var ok bool
		inv, ok = args[0].(*InventoryValue)
		if !ok {
			return nil, fmt.Errorf("first argument must be an inventory")
		}
		filterFn, ok = args[1].(starlark.Callable)
		if !ok {
			return nil, fmt.Errorf("second argument must be a callable")
		}
	} else if len(args) == 1 {
		var ok bool
		inv, ok = args[0].(*InventoryValue)
		if !ok {
			return nil, fmt.Errorf("first argument must be an inventory")
		}
	} else {
		return nil, fmt.Errorf("filter requires at least an inventory argument")
	}

	var filtered []map[string]interface{}

	for _, item := range inv.items {
		include := true

		if filterFn != nil {
			// Use function filter
			dict := starlark.NewDict(len(item))
			for k, v := range item {
				var starVal starlark.Value
				if err := startype.Go(v).Starlark(&starVal); err != nil {
					dict.SetKey(starlark.String(k), starlark.String(fmt.Sprintf("%v", v)))
					continue
				}
				dict.SetKey(starlark.String(k), starVal)
			}
			result, err := starlark.Call(thread, filterFn, starlark.Tuple{dict}, nil)
			if err != nil {
				return nil, err
			}
			include = bool(result.Truth())
		} else if len(kwargs) > 0 {
			// Use keyword filter
			for _, kv := range kwargs {
				key := string(kv[0].(starlark.String))
				var expectedVal any
				if err := startype.Starlark(kv[1]).Go(&expectedVal); err != nil {
					include = false
					break
				}
				if actualVal, ok := item[key]; ok {
					if actualVal != expectedVal {
						include = false
						break
					}
				} else {
					include = false
					break
				}
			}
		}

		if include {
			filtered = append(filtered, item)
		}
	}

	return &InventoryValue{items: filtered}, nil
}

// groupBy groups inventory items by a key.
// Usage: inventory.group_by(inv, "environment")
func (m *Module) groupBy(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inv *InventoryValue
	var key string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "inventory", &inv, "key", &key); err != nil {
		return nil, err
	}

	groups := make(map[string][]map[string]interface{})
	for _, item := range inv.items {
		groupVal := ""
		if v, ok := item[key]; ok {
			groupVal = fmt.Sprintf("%v", v)
		}
		groups[groupVal] = append(groups[groupVal], item)
	}

	result := starlark.NewDict(len(groups))
	for groupName, groupItems := range groups {
		inv := &InventoryValue{items: groupItems}
		result.SetKey(starlark.String(groupName), inv)
	}
	return result, nil
}

// merge merges multiple inventories.
// Usage: inventory.merge(inv1, inv2, ...)
func (m *Module) merge(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var merged []map[string]interface{}

	for _, arg := range args {
		inv, ok := arg.(*InventoryValue)
		if !ok {
			return nil, fmt.Errorf("all arguments must be inventories")
		}
		merged = append(merged, inv.items...)
	}

	return &InventoryValue{items: merged}, nil
}

// addresses extracts addresses from an inventory.
// Usage: inventory.addresses(inv, key="address")
func (m *Module) addresses(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var inv *InventoryValue
	key := "address"
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "inventory", &inv, "key?", &key); err != nil {
		return nil, err
	}

	var addresses []starlark.Value
	for _, item := range inv.items {
		if addr, ok := item[key]; ok {
			var starVal starlark.Value
			if err := startype.Go(addr).Starlark(&starVal); err != nil {
				addresses = append(addresses, starlark.String(fmt.Sprintf("%v", addr)))
				continue
			}
			addresses = append(addresses, starVal)
		}
	}

	return starlark.NewList(addresses), nil
}
