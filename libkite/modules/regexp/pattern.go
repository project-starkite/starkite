package regexp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
	"github.com/vladimirvivien/startype"
)

// Pattern is a Starlark value wrapping a compiled regexp.
type Pattern struct {
	re      *regexp.Regexp
	pattern string
}

var (
	_ starlark.Value    = (*Pattern)(nil)
	_ starlark.HasAttrs = (*Pattern)(nil)
)

func (p *Pattern) String() string        { return fmt.Sprintf("regexp.pattern(%q)", p.pattern) }
func (p *Pattern) Type() string          { return "regexp.pattern" }
func (p *Pattern) Freeze()               {}
func (p *Pattern) Truth() starlark.Bool  { return starlark.True }
func (p *Pattern) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: regexp.pattern") }

func (p *Pattern) Attr(name string) (starlark.Value, error) {
	// try_ dispatch
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if method := p.methodBuiltin(base); method != nil {
			return libkite.TryWrap("regexp.pattern."+name, method), nil
		}
		return nil, nil
	}

	// Properties
	switch name {
	case "pattern":
		return starlark.String(p.pattern), nil
	case "group_count":
		return starlark.MakeInt(p.re.NumSubexp()), nil
	case "group_names":
		names := p.re.SubexpNames()
		var elems []starlark.Value
		for _, n := range names {
			if n != "" {
				elems = append(elems, starlark.String(n))
			}
		}
		return starlark.NewList(elems), nil
	}

	// Methods
	if method := p.methodBuiltin(name); method != nil {
		return method, nil
	}
	return nil, nil
}

func (p *Pattern) methodBuiltin(name string) *starlark.Builtin {
	switch name {
	case "match":
		return starlark.NewBuiltin("regexp.pattern.match", p.matchMethod)
	case "find":
		return starlark.NewBuiltin("regexp.pattern.find", p.findMethod)
	case "find_all":
		return starlark.NewBuiltin("regexp.pattern.find_all", p.findAllMethod)
	case "replace":
		return starlark.NewBuiltin("regexp.pattern.replace", p.replaceMethod)
	case "split":
		return starlark.NewBuiltin("regexp.pattern.split", p.splitMethod)
	}
	return nil
}

func (p *Pattern) AttrNames() []string {
	names := []string{
		"pattern", "group_count", "group_names",
		"match", "find", "find_all", "replace", "split",
		"try_match", "try_find", "try_find_all", "try_replace", "try_split",
	}
	sort.Strings(names)
	return names
}

func (p *Pattern) matchMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		S string `name:"s" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	return starlark.Bool(p.re.MatchString(params.S)), nil
}

func (p *Pattern) findMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		S string `name:"s" position:"0" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	loc := p.re.FindStringSubmatchIndex(params.S)
	if loc == nil {
		return starlark.None, nil
	}
	return newMatch(p.re, params.S, loc), nil
}

func (p *Pattern) findAllMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		S string `name:"s" position:"0" required:"true"`
		N int    `name:"n" position:"1"`
	}
	params.N = -1
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	locs := p.re.FindAllStringSubmatchIndex(params.S, params.N)
	elems := make([]starlark.Value, len(locs))
	for i, loc := range locs {
		elems[i] = newMatch(p.re, params.S, loc)
	}
	return starlark.NewList(elems), nil
}

func (p *Pattern) replaceMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		S    string `name:"s" position:"0" required:"true"`
		Repl string `name:"repl" position:"1" required:"true"`
	}
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	return starlark.String(p.re.ReplaceAllString(params.S, params.Repl)), nil
}

func (p *Pattern) splitMethod(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var params struct {
		S string `name:"s" position:"0" required:"true"`
		N int    `name:"n" position:"1"`
	}
	params.N = -1
	if err := startype.Args(args, kwargs).Go(&params); err != nil {
		return nil, err
	}
	parts := p.re.Split(params.S, params.N)
	elems := make([]starlark.Value, len(parts))
	for i, part := range parts {
		elems[i] = starlark.String(part)
	}
	return starlark.NewList(elems), nil
}

// compilePattern compiles a pattern with optional flags.
func compilePattern(pattern, flags string) (*regexp.Regexp, error) {
	resolved, err := resolvePattern(pattern, flags)
	if err != nil {
		return nil, err
	}
	return regexp.Compile(resolved)
}

// resolvePattern prepends inline flags to a pattern string.
func resolvePattern(pattern, flags string) (string, error) {
	if flags == "" {
		return pattern, nil
	}
	for _, c := range flags {
		switch c {
		case 'i', 'm', 's', 'U':
			// valid
		default:
			return "", fmt.Errorf("regexp: unknown flag %q (valid: i, m, s, U)", string(c))
		}
	}
	return "(?" + flags + ")" + pattern, nil
}
