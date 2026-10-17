// Package strings provides string manipulation functions for starkite.
package strings

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

const ModuleName libkite.ModuleName = "strings"

// Module implements string manipulation functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "strings provides unique string functions: ljust, rjust, center, cut, equal, has_any, quote, unquote"
}

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = libkite.NewTryModule(
			string(ModuleName),
			starlark.StringDict{
				"ljust":   starlark.NewBuiltin("strings.ljust", m.ljust),
				"rjust":   starlark.NewBuiltin("strings.rjust", m.rjust),
				"center":  starlark.NewBuiltin("strings.center", m.center),
				"cut":     starlark.NewBuiltin("strings.cut", m.cut),
				"equal":   starlark.NewBuiltin("strings.equal", m.equal),
				"has_any": starlark.NewBuiltin("strings.has_any", m.hasAny),
				"quote":   starlark.NewBuiltin("strings.quote", m.quote),
				"unquote": starlark.NewBuiltin("strings.unquote", m.unquote),
			},
		)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

func (m *Module) ljust(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S        string `name:"s" position:"0" required:"true"`
		Width    int    `name:"width" position:"1" required:"true"`
		Fillchar string `name:"fillchar" position:"2"`
	}
	p.Fillchar = " " // default
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(p.Fillchar) != 1 {
		return nil, fmt.Errorf("strings.ljust: fillchar must be exactly one character")
	}
	runeCount := utf8.RuneCountInString(p.S)
	if runeCount >= p.Width {
		return starlark.String(p.S), nil
	}
	return starlark.String(p.S + strings.Repeat(p.Fillchar, p.Width-runeCount)), nil
}

func (m *Module) rjust(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S        string `name:"s" position:"0" required:"true"`
		Width    int    `name:"width" position:"1" required:"true"`
		Fillchar string `name:"fillchar" position:"2"`
	}
	p.Fillchar = " " // default
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(p.Fillchar) != 1 {
		return nil, fmt.Errorf("strings.rjust: fillchar must be exactly one character")
	}
	runeCount := utf8.RuneCountInString(p.S)
	if runeCount >= p.Width {
		return starlark.String(p.S), nil
	}
	return starlark.String(strings.Repeat(p.Fillchar, p.Width-runeCount) + p.S), nil
}

func (m *Module) center(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S        string `name:"s" position:"0" required:"true"`
		Width    int    `name:"width" position:"1" required:"true"`
		Fillchar string `name:"fillchar" position:"2"`
	}
	p.Fillchar = " " // default
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(p.Fillchar) != 1 {
		return nil, fmt.Errorf("strings.center: fillchar must be exactly one character")
	}
	runeCount := utf8.RuneCountInString(p.S)
	if runeCount >= p.Width {
		return starlark.String(p.S), nil
	}
	total := p.Width - runeCount
	left := total / 2
	right := total - left
	return starlark.String(strings.Repeat(p.Fillchar, left) + p.S + strings.Repeat(p.Fillchar, right)), nil
}

// CutResult represents the result of strings.cut, supporting both attribute access
// (.before, .after, .found) and tuple-style indexing/unpacking.
type CutResult struct {
	Before string
	After  string
	Found  bool
}

var (
	_ starlark.Value     = (*CutResult)(nil)
	_ starlark.HasAttrs  = (*CutResult)(nil)
	_ starlark.Indexable = (*CutResult)(nil)
	_ starlark.Sequence  = (*CutResult)(nil)
)

func (c *CutResult) String() string {
	return fmt.Sprintf("struct(after=%q, before=%q, found=%t)", c.After, c.Before, c.Found)
}
func (c *CutResult) Type() string          { return "struct" }
func (c *CutResult) Freeze()               {}
func (c *CutResult) Truth() starlark.Bool  { return starlark.Bool(c.Found) }
func (c *CutResult) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: struct") }

func (c *CutResult) Attr(name string) (starlark.Value, error) {
	switch name {
	case "before":
		return starlark.String(c.Before), nil
	case "after":
		return starlark.String(c.After), nil
	case "found":
		return starlark.Bool(c.Found), nil
	default:
		return nil, nil
	}
}

func (c *CutResult) AttrNames() []string {
	return []string{"after", "before", "found"}
}

func (c *CutResult) Len() int { return 3 }
func (c *CutResult) Index(i int) starlark.Value {
	switch i {
	case 0:
		return starlark.String(c.Before)
	case 1:
		return starlark.String(c.After)
	case 2:
		return starlark.Bool(c.Found)
	default:
		return nil
	}
}
func (c *CutResult) Slice(start, end, step int) starlark.Value {
	vals := []starlark.Value{starlark.String(c.Before), starlark.String(c.After), starlark.Bool(c.Found)}
	var res []starlark.Value
	for i := start; i < end; i += step {
		res = append(res, vals[i])
	}
	return starlark.Tuple(res)
}
func (c *CutResult) Iterate() starlark.Iterator {
	return starlark.Tuple{starlark.String(c.Before), starlark.String(c.After), starlark.Bool(c.Found)}.Iterate()
}

func (m *Module) cut(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S   string `name:"s" position:"0" required:"true"`
		Sep string `name:"sep" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	before, after, found := strings.Cut(p.S, p.Sep)
	return &CutResult{Before: before, After: after, Found: found}, nil
}

func (m *Module) equal(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S string `name:"s" position:"0" required:"true"`
		T string `name:"t" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return starlark.Bool(strings.EqualFold(p.S, p.T)), nil
}

func (m *Module) hasAny(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(kwargs) > 0 {
		return nil, fmt.Errorf("strings.has_any: unexpected keyword arguments")
	}
	if len(args) != 2 {
		return nil, fmt.Errorf("strings.has_any: expected 2 arguments, got %d", len(args))
	}
	s, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("strings.has_any: expected string for first argument, got %s", args[0].Type())
	}
	switch v := args[1].(type) {
	case starlark.String:
		return starlark.Bool(strings.ContainsAny(s, string(v))), nil
	case *starlark.List:
		for i := 0; i < v.Len(); i++ {
			elem, ok := starlark.AsString(v.Index(i))
			if !ok {
				return nil, fmt.Errorf("strings.has_any: list elements must be strings, got %s", v.Index(i).Type())
			}
			if strings.Contains(s, elem) {
				return starlark.True, nil
			}
		}
		return starlark.False, nil
	case starlark.Tuple:
		for i := 0; i < v.Len(); i++ {
			elem, ok := starlark.AsString(v.Index(i))
			if !ok {
				return nil, fmt.Errorf("strings.has_any: tuple elements must be strings, got %s", v.Index(i).Type())
			}
			if strings.Contains(s, elem) {
				return starlark.True, nil
			}
		}
		return starlark.False, nil
	default:
		return nil, fmt.Errorf("strings.has_any: expected string, list, or tuple for second argument, got %s", args[1].Type())
	}
}

func (m *Module) quote(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S string `name:"s" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return starlark.String(strconv.Quote(p.S)), nil
}

func (m *Module) unquote(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		S string `name:"s" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	result, err := strconv.Unquote(p.S)
	if err != nil {
		return nil, err
	}
	return starlark.String(result), nil
}
