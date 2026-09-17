package args

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// parse parses the script arguments according to the declared schema and returns an immutable ArgsResult.
func (m *Module) parse(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs); err != nil {
		return nil, err
	}

	ctx := libkite.EnsureScriptArgs(thread)
	if ctx.Parsed && ctx.Result != nil {
		return ctx.Result, nil
	}

	// Intercept --help / -h requests before flag evaluation
	for _, arg := range ctx.RawArgs {
		if arg == "--help" || arg == "-h" {
			helpText := FormatScriptHelp(ctx)
			if thread != nil && thread.Print != nil {
				thread.Print(thread, helpText)
			} else {
				fmt.Println(helpText)
			}
			ctx.Parsed = true
			return nil, libkite.NewExitError(libkite.ExitSuccess)
		}
	}

	fs := pflag.NewFlagSet("script", pflag.ContinueOnError)

	stringVars := make(map[string]*string)
	intVars := make(map[string]*int64)
	boolVars := make(map[string]*bool)
	noBoolVars := make(map[string]*bool)
	floatVars := make(map[string]*float64)
	listVars := make(map[string]*[]string)

	// Register all declared flags into pflag.FlagSet
	for _, f := range ctx.Flags {
		switch f.Type {
		case libkite.ArgTypeString:
			v := new(string)
			if f.Default != starlark.None {
				if s, ok := f.Default.(starlark.String); ok {
					*v = string(s)
				}
			}
			fs.StringVarP(v, f.Flag, f.Shorthand, *v, f.Help)
			stringVars[f.Flag] = v

		case libkite.ArgTypeInt:
			v := new(int64)
			if f.Default != starlark.None {
				if i, ok := f.Default.(starlark.Int); ok {
					if i64, ok := i.Int64(); ok {
						*v = i64
					}
				}
			}
			fs.Int64VarP(v, f.Flag, f.Shorthand, *v, f.Help)
			intVars[f.Flag] = v

		case libkite.ArgTypeBool:
			v := new(bool)
			if f.Default != starlark.None {
				if b, ok := f.Default.(starlark.Bool); ok {
					*v = bool(b)
				}
			}
			fs.BoolVarP(v, f.Flag, f.Shorthand, *v, f.Help)
			boolVars[f.Flag] = v
			if !strings.HasPrefix(f.Flag, "no-") {
				noV := new(bool)
				fs.BoolVar(noV, "no-"+f.Flag, false, "")
				noBoolVars[f.Flag] = noV
			}

		case libkite.ArgTypeFloat:
			v := new(float64)
			if f.Default != starlark.None {
				if fl, ok := f.Default.(starlark.Float); ok {
					*v = float64(fl)
				} else if i, ok := f.Default.(starlark.Int); ok {
					if i64, ok := i.Int64(); ok {
						*v = float64(i64)
					}
				}
			}
			fs.Float64VarP(v, f.Flag, f.Shorthand, *v, f.Help)
			floatVars[f.Flag] = v

		case libkite.ArgTypeList:
			v := new([]string)
			if f.Default != starlark.None {
				if list, ok := f.Default.(*starlark.List); ok {
					for i := 0; i < list.Len(); i++ {
						elem := list.Index(i)
						if s, ok := elem.(starlark.String); ok {
							*v = append(*v, string(s))
						} else {
							*v = append(*v, elem.String())
						}
					}
				}
			}
			fs.StringSliceVarP(v, f.Flag, f.Shorthand, *v, f.Help)
			listVars[f.Flag] = v
		}
	}

	// Parse forwarded command-line tokens
	if err := fs.Parse(ctx.RawArgs); err != nil {
		return nil, err
	}

	resultData := make(map[string]starlark.Value)

	// Resolve flags in order
	for _, f := range ctx.Flags {
		cliChanged := fs.Changed(f.Flag)
		noCliChanged := false
		if _, ok := noBoolVars[f.Flag]; ok && fs.Changed("no-"+f.Flag) {
			noCliChanged = true
		}

		switch f.Type {
		case libkite.ArgTypeString:
			var val string
			if cliChanged {
				val = *stringVars[f.Flag]
			} else if f.VarFallback != "" && m.hasVar(f.VarFallback) {
				val = m.getVarString(f.VarFallback)
			} else if f.Default != starlark.None {
				val = string(f.Default.(starlark.String))
			} else if f.Required {
				return nil, fmt.Errorf("missing required flag: --%s", f.Flag)
			} else {
				val = ""
			}

			// Validate choices if defined
			if len(f.Choices) > 0 {
				found := slices.Contains(f.Choices, val)
				if !found {
					return nil, fmt.Errorf("invalid value for --%s: %q (allowed choices: %s)", f.Flag, val, strings.Join(f.Choices, ", "))
				}
			}
			resultData[f.Name] = starlark.String(val)

		case libkite.ArgTypeInt:
			var val int64
			if cliChanged {
				val = *intVars[f.Flag]
			} else if f.VarFallback != "" && m.hasVar(f.VarFallback) {
				var err error
				val, err = m.getVarInt(f.VarFallback)
				if err != nil {
					return nil, fmt.Errorf("invalid int value for var_fallback %q (--%s): %w", f.VarFallback, f.Flag, err)
				}
			} else if f.Default != starlark.None {
				var err error
				val, err = extractInt(f.Default, "args.parse", f.Flag)
				if err != nil {
					return nil, err
				}
			} else if f.Required {
				return nil, fmt.Errorf("missing required flag: --%s", f.Flag)
			} else {
				val = 0
			}

			// Validate bounds
			if f.Min != nil && float64(val) < *f.Min {
				return nil, fmt.Errorf("value for --%s (%d) cannot be less than %d", f.Flag, val, int64(*f.Min))
			}
			if f.Max != nil && float64(val) > *f.Max {
				return nil, fmt.Errorf("value for --%s (%d) cannot be greater than %d", f.Flag, val, int64(*f.Max))
			}
			resultData[f.Name] = starlark.MakeInt64(val)

		case libkite.ArgTypeBool:
			var val bool
			if noCliChanged {
				val = false
			} else if cliChanged {
				val = *boolVars[f.Flag]
			} else if f.VarFallback != "" && m.hasVar(f.VarFallback) {
				var err error
				val, err = m.getVarBool(f.VarFallback)
				if err != nil {
					return nil, fmt.Errorf("invalid bool value for var_fallback %q (--%s): %w", f.VarFallback, f.Flag, err)
				}
			} else if f.Default != starlark.None {
				val = bool(f.Default.(starlark.Bool))
			} else {
				val = false
			}
			resultData[f.Name] = starlark.Bool(val)

		case libkite.ArgTypeFloat:
			var val float64
			if cliChanged {
				val = *floatVars[f.Flag]
			} else if f.VarFallback != "" && m.hasVar(f.VarFallback) {
				var err error
				val, err = m.getVarFloat(f.VarFallback)
				if err != nil {
					return nil, fmt.Errorf("invalid float value for var_fallback %q (--%s): %w", f.VarFallback, f.Flag, err)
				}
			} else if f.Default != starlark.None {
				var err error
				val, err = extractFloat(f.Default, "args.parse", f.Flag)
				if err != nil {
					return nil, err
				}
			} else if f.Required {
				return nil, fmt.Errorf("missing required flag: --%s", f.Flag)
			} else {
				val = 0.0
			}

			// Validate bounds
			if f.Min != nil && val < *f.Min {
				return nil, fmt.Errorf("value for --%s (%v) cannot be less than %v", f.Flag, val, *f.Min)
			}
			if f.Max != nil && val > *f.Max {
				return nil, fmt.Errorf("value for --%s (%v) cannot be greater than %v", f.Flag, val, *f.Max)
			}
			resultData[f.Name] = starlark.Float(val)

		case libkite.ArgTypeList:
			var items []starlark.Value
			if cliChanged {
				rawSlice := *listVars[f.Flag]
				for _, rawItem := range rawSlice {
					elem, err := parseListItem(rawItem, f.ItemType, f.Flag)
					if err != nil {
						return nil, err
					}
					items = append(items, elem)
				}
			} else if f.VarFallback != "" && m.hasVar(f.VarFallback) {
				var err error
				items, err = m.getVarList(f.VarFallback, f.ItemType, f.Flag)
				if err != nil {
					return nil, err
				}
			} else if f.Default != starlark.None {
				if list, ok := f.Default.(*starlark.List); ok {
					for i := 0; i < list.Len(); i++ {
						items = append(items, list.Index(i))
					}
				}
			}
			resultData[f.Name] = starlark.NewList(items)
		}
	}

	// Resolve positional arguments
	posArgs := fs.Args()
	for i, p := range ctx.Positionals {
		if i < len(posArgs) {
			resultData[p.Attr] = starlark.String(posArgs[i])
		} else if p.Required {
			return nil, fmt.Errorf("missing required positional argument: <%s>", p.Name)
		} else if p.Default != starlark.None {
			resultData[p.Attr] = p.Default
		} else {
			resultData[p.Attr] = starlark.None
		}
	}

	if len(posArgs) > len(ctx.Positionals) {
		return nil, fmt.Errorf("unexpected positional argument: %s", posArgs[len(ctx.Positionals)])
	}

	result := NewArgsResult(resultData)
	ctx.Parsed = true
	ctx.Result = result
	return result, nil
}

// parseListItem converts a CLI string token into a typed Starlark value based on item_type.
func parseListItem(raw string, itemType string, flag string) (starlark.Value, error) {
	switch itemType {
	case "string":
		return starlark.String(raw), nil
	case "int":
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid int value for --%s: %q", flag, raw)
		}
		return starlark.MakeInt64(n), nil
	case "float":
		fl, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float value for --%s: %q", flag, raw)
		}
		return starlark.Float(fl), nil
	default:
		return starlark.String(raw), nil
	}
}

// --- VarStore helper methods on Module ---

func (m *Module) hasVar(name string) bool {
	if m.config == nil || m.config.VarStore == nil {
		return false
	}
	_, ok := m.config.VarStore.Get(name)
	return ok
}

func (m *Module) getVarString(name string) string {
	if m.config == nil || m.config.VarStore == nil {
		return ""
	}
	val, ok := m.config.VarStore.Get(name)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

func (m *Module) getVarInt(name string) (int64, error) {
	if m.config == nil || m.config.VarStore == nil {
		return 0, fmt.Errorf("variable %q not found", name)
	}
	val, ok := m.config.VarStore.Get(name)
	if !ok {
		return 0, fmt.Errorf("variable %q not found", name)
	}
	switch v := val.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type %T for int variable %q", val, name)
	}
}

func (m *Module) getVarBool(name string) (bool, error) {
	if m.config == nil || m.config.VarStore == nil {
		return false, fmt.Errorf("variable %q not found", name)
	}
	val, ok := m.config.VarStore.Get(name)
	if !ok {
		return false, fmt.Errorf("variable %q not found", name)
	}
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	default:
		return false, fmt.Errorf("unsupported type %T for bool variable %q", val, name)
	}
}

func (m *Module) getVarFloat(name string) (float64, error) {
	if m.config == nil || m.config.VarStore == nil {
		return 0.0, fmt.Errorf("variable %q not found", name)
	}
	val, ok := m.config.VarStore.Get(name)
	if !ok {
		return 0.0, fmt.Errorf("variable %q not found", name)
	}
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0.0, fmt.Errorf("unsupported type %T for float variable %q", val, name)
	}
}

func (m *Module) getVarList(name string, itemType string, flag string) ([]starlark.Value, error) {
	if m.config == nil || m.config.VarStore == nil {
		return nil, fmt.Errorf("variable %q not found", name)
	}
	val, ok := m.config.VarStore.Get(name)
	if !ok {
		return nil, fmt.Errorf("variable %q not found", name)
	}
	var rawItems []string
	switch v := val.(type) {
	case []string:
		rawItems = v
	case []any:
		for _, itm := range v {
			rawItems = append(rawItems, fmt.Sprintf("%v", itm))
		}
	case string:
		var jsonArr []string
		if err := json.Unmarshal([]byte(v), &jsonArr); err == nil {
			rawItems = jsonArr
		} else {
			parts := strings.SplitSeq(v, ",")
			for p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					rawItems = append(rawItems, p)
				}
			}
		}
	default:
		return nil, fmt.Errorf("unsupported type %T for list variable %q", val, name)
	}

	var result []starlark.Value
	for _, raw := range rawItems {
		elem, err := parseListItem(raw, itemType, flag)
		if err != nil {
			return nil, err
		}
		result = append(result, elem)
	}
	return result, nil
}
