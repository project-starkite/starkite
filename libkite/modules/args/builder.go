package args

import (
	"fmt"
	"slices"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// buildString declares a string flag (--action install, -a install).
func (m *Module) buildString(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		flag        string
		defaultVal  starlark.Value = starlark.None
		shorthand   string
		required    bool
		choicesVal  starlark.Value = starlark.None
		help        string
		varFallback string
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"flag?", &flag,
		"default?", &defaultVal,
		"shorthand?", &shorthand,
		"required?", &required,
		"choices?", &choicesVal,
		"help?", &help,
		"var_fallback?", &varFallback,
	); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("%s: name cannot be empty", fn.Name())
	}

	// Validate default value if provided
	if defaultVal != starlark.None {
		if _, ok := defaultVal.(starlark.String); !ok {
			return nil, fmt.Errorf("%s: default value must be a string, got %s", fn.Name(), defaultVal.Type())
		}
	}

	// Validate choices
	var choices []string
	if choicesVal != starlark.None {
		var err error
		choices, err = unpackIterableStrings(choicesVal, fn.Name(), "choices")
		if err != nil {
			return nil, err
		}
		if defaultVal != starlark.None && len(choices) > 0 {
			defStr := string(defaultVal.(starlark.String))
			found := slices.Contains(choices, defStr)
			if !found {
				return nil, fmt.Errorf("%s: default value %q is not in allowed choices %v", fn.Name(), defStr, choices)
			}
		}
	}

	flagName := libkite.NormalizeFlagName(name, flag)
	attrName := libkite.NormalizeAttributeName(name)

	ctx := libkite.EnsureScriptArgs(thread)
	err := ctx.AddFlag(libkite.FlagDef{
		Name:        attrName,
		Flag:        flagName,
		Type:        libkite.ArgTypeString,
		Shorthand:   shorthand,
		Default:     defaultVal,
		Required:    required,
		Choices:     choices,
		Help:        help,
		VarFallback: varFallback,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.None, nil
}

// buildInt declares an integer flag (--replicas 3, -r 3).
func (m *Module) buildInt(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		flag        string
		defaultVal  starlark.Value = starlark.None
		shorthand   string
		required    bool
		minVal      starlark.Value = starlark.None
		maxVal      starlark.Value = starlark.None
		help        string
		varFallback string
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"flag?", &flag,
		"default?", &defaultVal,
		"shorthand?", &shorthand,
		"required?", &required,
		"min?", &minVal,
		"max?", &maxVal,
		"help?", &help,
		"var_fallback?", &varFallback,
	); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("%s: name cannot be empty", fn.Name())
	}

	// Validate bounds
	var (
		minFloat *float64
		maxFloat *float64
	)
	if minVal != starlark.None {
		minInt, err := extractInt(minVal, fn.Name(), "min")
		if err != nil {
			return nil, err
		}
		mf := float64(minInt)
		minFloat = &mf
	}
	if maxVal != starlark.None {
		maxInt, err := extractInt(maxVal, fn.Name(), "max")
		if err != nil {
			return nil, err
		}
		mf := float64(maxInt)
		maxFloat = &mf
	}
	if minFloat != nil && maxFloat != nil && *minFloat > *maxFloat {
		return nil, fmt.Errorf("%s: min (%v) cannot be greater than max (%v)", fn.Name(), *minFloat, *maxFloat)
	}

	// Validate default value if provided
	if defaultVal != starlark.None {
		defInt, err := extractInt(defaultVal, fn.Name(), "default")
		if err != nil {
			return nil, err
		}
		defFloat := float64(defInt)
		if minFloat != nil && defFloat < *minFloat {
			return nil, fmt.Errorf("%s: default value (%v) cannot be less than min (%v)", fn.Name(), defInt, *minFloat)
		}
		if maxFloat != nil && defFloat > *maxFloat {
			return nil, fmt.Errorf("%s: default value (%v) cannot be greater than max (%v)", fn.Name(), defInt, *maxFloat)
		}
	} else if !required {
		// Standard default for non-required int flags is 0
		defaultVal = starlark.MakeInt(0)
	}

	flagName := libkite.NormalizeFlagName(name, flag)
	attrName := libkite.NormalizeAttributeName(name)

	ctx := libkite.EnsureScriptArgs(thread)
	err := ctx.AddFlag(libkite.FlagDef{
		Name:        attrName,
		Flag:        flagName,
		Type:        libkite.ArgTypeInt,
		Shorthand:   shorthand,
		Default:     defaultVal,
		Required:    required,
		Min:         minFloat,
		Max:         maxFloat,
		Help:        help,
		VarFallback: varFallback,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.None, nil
}

// buildBool declares a boolean flag (--dry-run, --verbose, -v).
func (m *Module) buildBool(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		flag        string
		defaultVal  starlark.Value = starlark.Bool(false)
		shorthand   string
		help        string
		varFallback string
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"flag?", &flag,
		"default?", &defaultVal,
		"shorthand?", &shorthand,
		"help?", &help,
		"var_fallback?", &varFallback,
	); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("%s: name cannot be empty", fn.Name())
	}

	if defaultVal != starlark.None {
		if _, ok := defaultVal.(starlark.Bool); !ok {
			return nil, fmt.Errorf("%s: default value must be a bool, got %s", fn.Name(), defaultVal.Type())
		}
	} else {
		defaultVal = starlark.Bool(false)
	}

	flagName := libkite.NormalizeFlagName(name, flag)
	attrName := libkite.NormalizeAttributeName(name)

	ctx := libkite.EnsureScriptArgs(thread)
	err := ctx.AddFlag(libkite.FlagDef{
		Name:        attrName,
		Flag:        flagName,
		Type:        libkite.ArgTypeBool,
		Shorthand:   shorthand,
		Default:     defaultVal,
		Required:    false,
		Help:        help,
		VarFallback: varFallback,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.None, nil
}

// buildFloat declares a floating-point flag (--ratio 0.75).
func (m *Module) buildFloat(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		flag        string
		defaultVal  starlark.Value = starlark.None
		shorthand   string
		required    bool
		minVal      starlark.Value = starlark.None
		maxVal      starlark.Value = starlark.None
		help        string
		varFallback string
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"flag?", &flag,
		"default?", &defaultVal,
		"shorthand?", &shorthand,
		"required?", &required,
		"min?", &minVal,
		"max?", &maxVal,
		"help?", &help,
		"var_fallback?", &varFallback,
	); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("%s: name cannot be empty", fn.Name())
	}

	var (
		minFloat *float64
		maxFloat *float64
	)
	if minVal != starlark.None {
		mf, err := extractFloat(minVal, fn.Name(), "min")
		if err != nil {
			return nil, err
		}
		minFloat = &mf
	}
	if maxVal != starlark.None {
		mf, err := extractFloat(maxVal, fn.Name(), "max")
		if err != nil {
			return nil, err
		}
		maxFloat = &mf
	}
	if minFloat != nil && maxFloat != nil && *minFloat > *maxFloat {
		return nil, fmt.Errorf("%s: min (%v) cannot be greater than max (%v)", fn.Name(), *minFloat, *maxFloat)
	}

	if defaultVal != starlark.None {
		defFloat, err := extractFloat(defaultVal, fn.Name(), "default")
		if err != nil {
			return nil, err
		}
		if minFloat != nil && defFloat < *minFloat {
			return nil, fmt.Errorf("%s: default value (%v) cannot be less than min (%v)", fn.Name(), defFloat, *minFloat)
		}
		if maxFloat != nil && defFloat > *maxFloat {
			return nil, fmt.Errorf("%s: default value (%v) cannot be greater than max (%v)", fn.Name(), defFloat, *maxFloat)
		}
	} else if !required {
		defaultVal = starlark.Float(0.0)
	}

	flagName := libkite.NormalizeFlagName(name, flag)
	attrName := libkite.NormalizeAttributeName(name)

	ctx := libkite.EnsureScriptArgs(thread)
	err := ctx.AddFlag(libkite.FlagDef{
		Name:        attrName,
		Flag:        flagName,
		Type:        libkite.ArgTypeFloat,
		Shorthand:   shorthand,
		Default:     defaultVal,
		Required:    required,
		Min:         minFloat,
		Max:         maxFloat,
		Help:        help,
		VarFallback: varFallback,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.None, nil
}

// buildList declares a multi-value / repeated flag (--worker 10.0.0.1 --worker 10.0.0.2).
func (m *Module) buildList(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		flag        string
		defaultVal  starlark.Value = starlark.None
		shorthand   string
		itemType    = "string"
		help        string
		varFallback string
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"flag?", &flag,
		"default?", &defaultVal,
		"shorthand?", &shorthand,
		"item_type?", &itemType,
		"help?", &help,
		"var_fallback?", &varFallback,
	); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("%s: name cannot be empty", fn.Name())
	}

	switch itemType {
	case "string", "int", "float":
		// valid
	default:
		return nil, fmt.Errorf("%s: invalid item_type %q (allowed: 'string', 'int', 'float')", fn.Name(), itemType)
	}

	if defaultVal != starlark.None {
		iter, ok := defaultVal.(starlark.Iterable)
		if !ok {
			return nil, fmt.Errorf("%s: default must be a list or tuple, got %s", fn.Name(), defaultVal.Type())
		}
		// Validate default items match itemType
		it := iter.Iterate()
		defer it.Done()
		var elem starlark.Value
		for it.Next(&elem) {
			switch itemType {
			case "string":
				if _, ok := elem.(starlark.String); !ok {
					return nil, fmt.Errorf("%s: default list element must be string, got %s", fn.Name(), elem.Type())
				}
			case "int":
				if _, ok := elem.(starlark.Int); !ok {
					return nil, fmt.Errorf("%s: default list element must be int, got %s", fn.Name(), elem.Type())
				}
			case "float":
				if _, ok := elem.(starlark.Float); !ok {
					if _, ok := elem.(starlark.Int); !ok {
						return nil, fmt.Errorf("%s: default list element must be float, got %s", fn.Name(), elem.Type())
					}
				}
			}
		}
	} else {
		defaultVal = starlark.NewList(nil)
	}

	flagName := libkite.NormalizeFlagName(name, flag)
	attrName := libkite.NormalizeAttributeName(name)

	ctx := libkite.EnsureScriptArgs(thread)
	err := ctx.AddFlag(libkite.FlagDef{
		Name:        attrName,
		Flag:        flagName,
		Type:        libkite.ArgTypeList,
		Shorthand:   shorthand,
		Default:     defaultVal,
		Required:    false,
		ItemType:    itemType,
		Help:        help,
		VarFallback: varFallback,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.None, nil
}

// buildPositional declares a positional argument (<cluster-name>).
func (m *Module) buildPositional(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var (
		name        string
		requiredVal starlark.Value = starlark.None
		defaultVal  starlark.Value = starlark.None
		help        string
	)
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		"name", &name,
		"required?", &requiredVal,
		"default?", &defaultVal,
		"help?", &help,
	); err != nil {
		return nil, err
	}

	if name == "" {
		return nil, fmt.Errorf("%s: name cannot be empty", fn.Name())
	}

	required := true
	if defaultVal != starlark.None {
		// If default is provided, it is optional by default unless explicitly marked required
		if requiredVal == starlark.None {
			required = false
		} else if boolVal, ok := requiredVal.(starlark.Bool); ok && bool(boolVal) {
			return nil, fmt.Errorf("%s: positional argument %q cannot be both required and have a default value", fn.Name(), name)
		} else if boolVal, ok := requiredVal.(starlark.Bool); ok {
			required = bool(boolVal)
		}
	} else if requiredVal != starlark.None {
		if boolVal, ok := requiredVal.(starlark.Bool); ok {
			required = bool(boolVal)
		} else {
			return nil, fmt.Errorf("%s: required must be a bool, got %s", fn.Name(), requiredVal.Type())
		}
	}

	attrName := libkite.NormalizeAttributeName(name)

	ctx := libkite.EnsureScriptArgs(thread)
	err := ctx.AddPositional(libkite.PositionalDef{
		Name:     name,
		Attr:     attrName,
		Required: required,
		Default:  defaultVal,
		Help:     help,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return starlark.None, nil
}

// --- Helpers ---

func unpackIterableStrings(v starlark.Value, fnName, paramName string) ([]string, error) {
	iter, ok := v.(starlark.Iterable)
	if !ok {
		return nil, fmt.Errorf("%s: %s must be a list or tuple, got %s", fnName, paramName, v.Type())
	}
	it := iter.Iterate()
	defer it.Done()
	var (
		res  []string
		elem starlark.Value
	)
	for it.Next(&elem) {
		str, ok := elem.(starlark.String)
		if !ok {
			return nil, fmt.Errorf("%s: %s items must be strings, got %s", fnName, paramName, elem.Type())
		}
		res = append(res, string(str))
	}
	return res, nil
}

func extractInt(v starlark.Value, fnName, paramName string) (int64, error) {
	i, ok := v.(starlark.Int)
	if !ok {
		return 0, fmt.Errorf("%s: %s must be an int, got %s", fnName, paramName, v.Type())
	}
	i64, ok := i.Int64()
	if !ok {
		return 0, fmt.Errorf("%s: %s is out of range for 64-bit integer", fnName, paramName)
	}
	return i64, nil
}

func extractFloat(v starlark.Value, fnName, paramName string) (float64, error) {
	switch val := v.(type) {
	case starlark.Float:
		return float64(val), nil
	case starlark.Int:
		if i64, ok := val.Int64(); ok {
			return float64(i64), nil
		}
		if bi := val.BigInt(); bi != nil {
			f, _ := bi.Float64()
			return f, nil
		}
	}
	return 0, fmt.Errorf("%s: %s must be a float or int, got %s", fnName, paramName, v.Type())
}
