package regexp

import (
	"fmt"
	"regexp"
	"sort"

	"go.starlark.net/starlark"

	"github.com/vladimirvivien/startype"
)

// Match is a Starlark value representing a regexp match result.
type Match struct {
	text       string
	start, end int
	groups     []string
	groupNames []string
	matched    []bool // false = unmatched optional group (loc == -1)
}

var (
	_ starlark.Value    = (*Match)(nil)
	_ starlark.HasAttrs = (*Match)(nil)
)

func (m *Match) String() string        { return m.text }
func (m *Match) Type() string          { return "regexp.match" }
func (m *Match) Freeze()               {}
func (m *Match) Truth() starlark.Bool  { return starlark.True }
func (m *Match) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: regexp.match") }

func (m *Match) Attr(name string) (starlark.Value, error) {
	switch name {
	case "text":
		return starlark.String(m.text), nil
	case "start":
		return starlark.MakeInt(m.start), nil
	case "end":
		return starlark.MakeInt(m.end), nil
	case "groups":
		elems := make([]starlark.Value, len(m.groups))
		for i := range m.groups {
			if !m.matched[i] {
				elems[i] = starlark.None
			} else {
				elems[i] = starlark.String(m.groups[i])
			}
		}
		return starlark.Tuple(elems), nil
	case "group":
		return starlark.NewBuiltin("regexp.match.group", m.group), nil
	default:
		return nil, nil
	}
}

func (m *Match) AttrNames() []string {
	names := []string{"end", "group", "groups", "start", "text"}
	sort.Strings(names)
	return names
}

func (m *Match) group(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		N    int    `name:"n" position:"0"`
		Name string `name:"name"`
	}
	if err := startype.Args(args, kwargs).Go(&p); err != nil {
		return nil, err
	}

	if p.N != 0 && p.Name != "" {
		return nil, fmt.Errorf("regexp.match.group: cannot specify both n and name")
	}

	if p.Name != "" {
		for i, gn := range m.groupNames {
			if gn == p.Name {
				if !m.matched[i] {
					return starlark.None, nil
				}
				return starlark.String(m.groups[i]), nil
			}
		}
		return nil, fmt.Errorf("regexp.match.group: no group named %q", p.Name)
	}

	if p.N < 0 || p.N >= len(m.groups) {
		return nil, fmt.Errorf("regexp.match.group: index %d out of range [0, %d)", p.N, len(m.groups))
	}
	if !m.matched[p.N] {
		return starlark.None, nil
	}
	return starlark.String(m.groups[p.N]), nil
}

// newMatch constructs a Match from a compiled regexp and FindStringSubmatchIndex result.
// loc must be non-nil (caller checks for match existence).
func newMatch(re *regexp.Regexp, input string, loc []int) *Match {
	numGroups := len(loc) / 2
	groups := make([]string, numGroups)
	matched := make([]bool, numGroups)

	for i := 0; i < numGroups; i++ {
		s, e := loc[2*i], loc[2*i+1]
		if s == -1 {
			matched[i] = false
		} else {
			matched[i] = true
			groups[i] = input[s:e]
		}
	}

	return &Match{
		text:       input[loc[0]:loc[1]],
		start:      loc[0],
		end:        loc[1],
		groups:     groups,
		groupNames: re.SubexpNames(),
		matched:    matched,
	}
}
