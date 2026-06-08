package sql

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Stmt is a prepared statement value: a SQL string, its bound parameters, and
// an optional name used in batch results and errors. It is connection- and
// driver-independent until executed by db.batch.
type Stmt struct {
	name   string
	sql    string
	params []any
}

func (s *Stmt) String() string {
	if s.name != "" {
		return fmt.Sprintf("<sql.stmt %s>", s.name)
	}
	return fmt.Sprintf("<sql.stmt %q>", s.sql)
}
func (s *Stmt) Type() string          { return "sql.stmt" }
func (s *Stmt) Freeze()               {}
func (s *Stmt) Truth() starlark.Bool  { return starlark.True }
func (s *Stmt) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: sql.stmt") }

// stmt builds a Stmt value: sql.stmt(sql, *params, name="...").
func (m *Module) stmt(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sql.stmt: missing query string")
	}
	q, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("sql.stmt: query must be a string")
	}
	params := make([]any, 0, len(args)-1)
	for i, a := range args[1:] {
		v, err := starlarkToGo(a)
		if err != nil {
			return nil, fmt.Errorf("sql.stmt: parameter %d: %w", i+1, err)
		}
		params = append(params, v)
	}

	var name string
	for _, kv := range kwargs {
		if k, ok := kv[0].(starlark.String); ok && string(k) == "name" {
			if s, ok := starlark.AsString(kv[1]); ok {
				name = s
			}
		}
	}
	return &Stmt{name: name, sql: q, params: params}, nil
}
