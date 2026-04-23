// Package vars provides the vars module for starkite.
// It exposes variable access functions with priority-based resolution
// (CLI > var-files > config > env > script defaults).
package vars

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "vars"

// Module implements the vars module.
type Module struct {
	once    sync.Once
	module  starlark.Value
	aliases starlark.StringDict
	config  *starbase.ModuleConfig
}

func New() *Module {
	return &Module{}
}

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "vars provides variable access: var_str(name, default) reads from the VarStore"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config

		members := starlark.StringDict{
			"var_str":   starlark.NewBuiltin("vars.var_str", m.varStr),
			"var_int":   starlark.NewBuiltin("vars.var_int", m.varInt),
			"var_bool":  starlark.NewBuiltin("vars.var_bool", m.varBool),
			"var_float": starlark.NewBuiltin("vars.var_float", m.varFloat),
			"var_names": starlark.NewBuiltin("vars.var_names", m.varNames),
			"var_list":  starlark.NewBuiltin("vars.var_list", m.varList),
			"var_dict":  starlark.NewBuiltin("vars.var_dict", m.varDict),
		}

		m.module = &starlarkstruct.Module{
			Name:    string(ModuleName),
			Members: members,
		}

		// Create global aliases so var_str()/var_int()/etc. are available without module prefix
		m.aliases = starlark.StringDict{
			"var_str":   starlark.NewBuiltin("var_str", m.varStr),
			"var_int":   starlark.NewBuiltin("var_int", m.varInt),
			"var_bool":  starlark.NewBuiltin("var_bool", m.varBool),
			"var_float": starlark.NewBuiltin("var_float", m.varFloat),
			"var_names": starlark.NewBuiltin("var_names", m.varNames),
			"var_list":  starlark.NewBuiltin("var_list", m.varList),
			"var_dict":  starlark.NewBuiltin("var_dict", m.varDict),
		}
	})

	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict {
	return m.aliases
}

func (m *Module) FactoryMethod() string { return "" }

// varStr returns the value of a variable as a Starlark string.
func (m *Module) varStr(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string `name:"name" position:"0" required:"true"`
		Default string `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "vars", "var_str", p.Name); err != nil {
		return nil, err
	}

	// Look up in VarStore if available
	if m.config != nil && m.config.VarStore != nil {
		if value, ok := m.config.VarStore.Get(p.Name); ok {
			return starlark.String(fmt.Sprintf("%v", value)), nil
		}
	}

	// Fall back to script default
	return starlark.String(p.Default), nil
}

// varInt returns the value of a variable as a Starlark int.
func (m *Module) varInt(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string `name:"name" position:"0" required:"true"`
		Default int    `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "vars", "var_int", p.Name); err != nil {
		return nil, err
	}

	if m.config != nil && m.config.VarStore != nil {
		if value, ok := m.config.VarStore.Get(p.Name); ok {
			switch v := value.(type) {
			case int:
				return starlark.MakeInt(v), nil
			case int64:
				return starlark.MakeInt64(v), nil
			case float64:
				return starlark.MakeInt(int(v)), nil
			case string:
				n, err := strconv.Atoi(v)
				if err != nil {
					return nil, fmt.Errorf("var_int: cannot convert %q to int for variable %q", v, p.Name)
				}
				return starlark.MakeInt(n), nil
			default:
				return nil, fmt.Errorf("var_int: unsupported type %T for variable %q", value, p.Name)
			}
		}
	}

	return starlark.MakeInt(p.Default), nil
}

// varBool returns the value of a variable as a Starlark bool.
func (m *Module) varBool(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string `name:"name" position:"0" required:"true"`
		Default bool   `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "vars", "var_bool", p.Name); err != nil {
		return nil, err
	}

	if m.config != nil && m.config.VarStore != nil {
		if value, ok := m.config.VarStore.Get(p.Name); ok {
			switch v := value.(type) {
			case bool:
				return starlark.Bool(v), nil
			case string:
				b, err := strconv.ParseBool(v)
				if err != nil {
					return nil, fmt.Errorf("var_bool: cannot convert %q to bool for variable %q", v, p.Name)
				}
				return starlark.Bool(b), nil
			default:
				return nil, fmt.Errorf("var_bool: unsupported type %T for variable %q", value, p.Name)
			}
		}
	}

	return starlark.Bool(p.Default), nil
}

// varFloat returns the value of a variable as a Starlark float.
func (m *Module) varFloat(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string  `name:"name" position:"0" required:"true"`
		Default float64 `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "vars", "var_float", p.Name); err != nil {
		return nil, err
	}

	if m.config != nil && m.config.VarStore != nil {
		if value, ok := m.config.VarStore.Get(p.Name); ok {
			switch v := value.(type) {
			case float64:
				return starlark.Float(v), nil
			case float32:
				return starlark.Float(float64(v)), nil
			case int:
				return starlark.Float(float64(v)), nil
			case int64:
				return starlark.Float(float64(v)), nil
			case string:
				f, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return nil, fmt.Errorf("var_float: cannot convert %q to float for variable %q", v, p.Name)
				}
				return starlark.Float(f), nil
			default:
				return nil, fmt.Errorf("var_float: unsupported type %T for variable %q", value, p.Name)
			}
		}
	}

	return starlark.Float(p.Default), nil
}

// varNames returns a sorted list of all available variable names.
func (m *Module) varNames(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starbase.Check(thread, "vars", "var_names", ""); err != nil {
		return nil, err
	}
	if m.config == nil || m.config.VarStore == nil {
		return starlark.NewList(nil), nil
	}
	keys := m.config.VarStore.Keys()
	elems := make([]starlark.Value, len(keys))
	for i, k := range keys {
		elems[i] = starlark.String(k)
	}
	return starlark.NewList(elems), nil
}

// varList returns the value of a variable as a Starlark list.
func (m *Module) varList(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string         `name:"name" position:"0" required:"true"`
		Default starlark.Value `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "vars", "var_list", p.Name); err != nil {
		return nil, err
	}

	if m.config != nil && m.config.VarStore != nil {
		if value, ok := m.config.VarStore.Get(p.Name); ok {
			return toStarlarkList(value, p.Name)
		}
	}

	if p.Default != nil {
		return p.Default, nil
	}
	return starlark.NewList(nil), nil
}

// varDict returns the value of a variable as a Starlark dict.
func (m *Module) varDict(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Name    string         `name:"name" position:"0" required:"true"`
		Default starlark.Value `name:"default" position:"1"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if err := starbase.Check(thread, "vars", "var_dict", p.Name); err != nil {
		return nil, err
	}

	if m.config != nil && m.config.VarStore != nil {
		if value, ok := m.config.VarStore.Get(p.Name); ok {
			return toStarlarkDict(value, p.Name)
		}
	}

	if p.Default != nil {
		return p.Default, nil
	}
	return starlark.NewDict(0), nil
}

// toStarlarkList converts a Go value to *starlark.List.
// Handles: []interface{} (from YAML/JSON), string (attempt JSON parse).
func toStarlarkList(value interface{}, name string) (starlark.Value, error) {
	switch v := value.(type) {
	case []interface{}:
		return goSliceToStarlarkList(v)
	case string:
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("var_list: cannot parse %q as list for variable %q", v, name)
		}
		slice, ok := parsed.([]interface{})
		if !ok {
			return nil, fmt.Errorf("var_list: value for %q is not a list", name)
		}
		return goSliceToStarlarkList(slice)
	default:
		return nil, fmt.Errorf("var_list: unsupported type %T for variable %q", value, name)
	}
}

// toStarlarkDict converts a Go value to *starlark.Dict.
// Handles: map[string]interface{} (from YAML/JSON), string (attempt JSON parse).
func toStarlarkDict(value interface{}, name string) (starlark.Value, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		return goMapToStarlarkDict(v)
	case string:
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("var_dict: cannot parse %q as dict for variable %q", v, name)
		}
		m, ok := parsed.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("var_dict: value for %q is not a dict", name)
		}
		return goMapToStarlarkDict(m)
	default:
		return nil, fmt.Errorf("var_dict: unsupported type %T for variable %q", value, name)
	}
}

// goSliceToStarlarkList converts []interface{} to *starlark.List using startype.
func goSliceToStarlarkList(slice []interface{}) (*starlark.List, error) {
	elems := make([]starlark.Value, 0, len(slice))
	for _, item := range slice {
		sv, err := startype.Go(item).ToStarlarkValue()
		if err != nil {
			return nil, fmt.Errorf("var_list: conversion error: %w", err)
		}
		elems = append(elems, sv)
	}
	return starlark.NewList(elems), nil
}

// goMapToStarlarkDict converts map[string]interface{} to *starlark.Dict using startype.
func goMapToStarlarkDict(m map[string]interface{}) (*starlark.Dict, error) {
	dict := starlark.NewDict(len(m))
	for k, v := range m {
		sv, err := startype.Go(v).ToStarlarkValue()
		if err != nil {
			return nil, fmt.Errorf("var_dict: conversion error for key %q: %w", k, err)
		}
		if err := dict.SetKey(starlark.String(k), sv); err != nil {
			return nil, fmt.Errorf("var_dict: set key error: %w", err)
		}
	}
	return dict, nil
}
