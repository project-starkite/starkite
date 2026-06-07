package libkite

import (
	"go.starlark.net/syntax"
)

// LoadTargets returns the module references named in a script's top-level load()
// statements, in source order. It parses only — the script is not executed.
func LoadTargets(filename string, src []byte) ([]string, error) {
	f, err := syntax.Parse(filename, src, 0)
	if err != nil {
		return nil, err
	}
	var targets []string
	for _, stmt := range f.Stmts {
		ls, ok := stmt.(*syntax.LoadStmt)
		if !ok {
			continue
		}
		if ls.Module != nil {
			if s, ok := ls.Module.Value.(string); ok {
				targets = append(targets, s)
			}
		}
	}
	return targets, nil
}
