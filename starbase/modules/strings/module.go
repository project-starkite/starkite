// Package strings provides string manipulation functions for starkite.
package strings

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/project-starkite/starkite/starbase"
)

const ModuleName starbase.ModuleName = "strings"

// Module implements string manipulation functions.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "strings provides unique string functions: ljust, rjust, center, cut, equal, has_any, quote, unquote"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = &starlarkstruct.Module{
			Name: string(ModuleName),
			Members: starlark.StringDict{
				"ljust":   starlark.NewBuiltin("strings.ljust", m.ljust),
				"rjust":   starlark.NewBuiltin("strings.rjust", m.rjust),
				"center":  starlark.NewBuiltin("strings.center", m.center),
				"cut":     starlark.NewBuiltin("strings.cut", m.cut),
				"equal":   starlark.NewBuiltin("strings.equal", m.equal),
				"has_any": starlark.NewBuiltin("strings.has_any", m.hasAny),
				"quote":   starlark.NewBuiltin("strings.quote", m.quote),
				"unquote": starlark.NewBuiltin("strings.unquote", m.unquote),
			},
		}
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
	runeCount := utf8.RuneCountInString(p.S)
	if runeCount >= p.Width {
		return starlark.String(p.S), nil
	}
	total := p.Width - runeCount
	left := total / 2
	right := total - left
	return starlark.String(strings.Repeat(p.Fillchar, left) + p.S + strings.Repeat(p.Fillchar, right)), nil
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
	return starlark.Tuple{starlark.String(before), starlark.String(after), starlark.Bool(found)}, nil
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
	var p struct {
		S     string `name:"s" position:"0" required:"true"`
		Chars string `name:"chars" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	return starlark.Bool(strings.ContainsAny(p.S, p.Chars)), nil
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
