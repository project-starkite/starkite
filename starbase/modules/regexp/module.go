package regexp

import (
	"sync"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/vladimirvivien/starkite/starbase"
)

const ModuleName starbase.ModuleName = "regexp"

type Module struct {
	once   sync.Once
	module starlark.Value
	config *starbase.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() starbase.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "regexp provides regular expression functions: match, find, find_all, replace, split, compile"
}

func (m *Module) Load(config *starbase.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		m.module = starbase.NewTryModule(string(ModuleName), starlark.StringDict{
			"match":    starlark.NewBuiltin("regexp.match", m.match),
			"find":     starlark.NewBuiltin("regexp.find", m.find),
			"find_all": starlark.NewBuiltin("regexp.find_all", m.findAll),
			"replace":  starlark.NewBuiltin("regexp.replace", m.replace),
			"split":    starlark.NewBuiltin("regexp.split", m.split),
			"compile":  starlark.NewBuiltin("regexp.compile", m.compile),
		})
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

func (m *Module) Aliases() starlark.StringDict { return nil }
func (m *Module) FactoryMethod() string        { return "" }

func (m *Module) compile(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
		Flags   string `name:"flags"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	re, err := compilePattern(p.Pattern, p.Flags)
	if err != nil {
		return nil, err
	}
	return &Pattern{re: re, pattern: p.Pattern}, nil
}

func (m *Module) match(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
		S       string `name:"s" position:"1" required:"true"`
		Flags   string `name:"flags"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	re, err := compilePattern(p.Pattern, p.Flags)
	if err != nil {
		return nil, err
	}
	return starlark.Bool(re.MatchString(p.S)), nil
}

func (m *Module) find(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
		S       string `name:"s" position:"1" required:"true"`
		Flags   string `name:"flags"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	re, err := compilePattern(p.Pattern, p.Flags)
	if err != nil {
		return nil, err
	}
	loc := re.FindStringSubmatchIndex(p.S)
	if loc == nil {
		return starlark.None, nil
	}
	return newMatch(re, p.S, loc), nil
}

func (m *Module) findAll(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
		S       string `name:"s" position:"1" required:"true"`
		N       int    `name:"n" position:"2"`
		Flags   string `name:"flags"`
	}
	p.N = -1
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	re, err := compilePattern(p.Pattern, p.Flags)
	if err != nil {
		return nil, err
	}
	locs := re.FindAllStringSubmatchIndex(p.S, p.N)
	elems := make([]starlark.Value, len(locs))
	for i, loc := range locs {
		elems[i] = newMatch(re, p.S, loc)
	}
	return starlark.NewList(elems), nil
}

func (m *Module) replace(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
		S       string `name:"s" position:"1" required:"true"`
		Repl    string `name:"repl" position:"2" required:"true"`
		Flags   string `name:"flags"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	re, err := compilePattern(p.Pattern, p.Flags)
	if err != nil {
		return nil, err
	}
	return starlark.String(re.ReplaceAllString(p.S, p.Repl)), nil
}

func (m *Module) split(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Pattern string `name:"pattern" position:"0" required:"true"`
		S       string `name:"s" position:"1" required:"true"`
		N       int    `name:"n" position:"2"`
		Flags   string `name:"flags"`
	}
	p.N = -1
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}
	re, err := compilePattern(p.Pattern, p.Flags)
	if err != nil {
		return nil, err
	}
	parts := re.Split(p.S, p.N)
	elems := make([]starlark.Value, len(parts))
	for i, part := range parts {
		elems[i] = starlark.String(part)
	}
	return starlark.NewList(elems), nil
}
