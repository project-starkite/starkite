package sql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Transaction is a Starlark value wrapping a *sql.Tx. It must be committed or
// rolled back; commit/rollback after completion are no-ops.
type Transaction struct {
	tx     *sql.Tx
	conn   *Connection
	done   bool
	dryRun bool
}

// --- starlark.Value ---

func (t *Transaction) String() string       { return "<sql.transaction>" }
func (t *Transaction) Type() string         { return "sql.transaction" }
func (t *Transaction) Freeze()              {}
func (t *Transaction) Truth() starlark.Bool { return starlark.Bool(!t.done) }
func (t *Transaction) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable type: sql.transaction")
}

// --- starlark.HasAttrs ---

func (t *Transaction) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if b := t.method(base); b != nil {
			return libkite.TryWrap("sql.transaction."+name, b), nil
		}
		return nil, nil
	}
	if b := t.method(name); b != nil {
		return b, nil
	}
	return nil, nil
}

func (t *Transaction) method(name string) *starlark.Builtin {
	switch name {
	case "query":
		return starlark.NewBuiltin("sql.transaction.query", t.query)
	case "query_row":
		return starlark.NewBuiltin("sql.transaction.query_row", t.queryRow)
	case "query_value":
		return starlark.NewBuiltin("sql.transaction.query_value", t.queryValue)
	case "query_column":
		return starlark.NewBuiltin("sql.transaction.query_column", t.queryColumn)
	case "exec":
		return starlark.NewBuiltin("sql.transaction.exec", t.execMethod)
	case "commit":
		return starlark.NewBuiltin("sql.transaction.commit", t.commit)
	case "rollback":
		return starlark.NewBuiltin("sql.transaction.rollback", t.rollback)
	}
	return nil
}

func (t *Transaction) AttrNames() []string {
	names := []string{"commit", "exec", "query", "query_column", "query_row", "query_value", "rollback"}
	for _, n := range []string{"query", "query_row", "query_value", "query_column", "exec"} {
		names = append(names, "try_"+n)
	}
	sort.Strings(names)
	return names
}

// --- methods ---

func (t *Transaction) query(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query", args)
	if err != nil {
		return nil, err
	}
	if t.dryRun {
		return starlark.NewList(nil), nil
	}
	if t.done {
		return nil, fmt.Errorf("sql.query: transaction already finished")
	}
	rows, err := t.tx.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, t.conn.queryErr("query", q, err)
	}
	defer rows.Close()
	return rowsToList(rows)
}

func (t *Transaction) queryRow(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query_row", args)
	if err != nil {
		return nil, err
	}
	if t.dryRun {
		return starlark.None, nil
	}
	if t.done {
		return nil, fmt.Errorf("sql.query_row: transaction already finished")
	}
	rows, err := t.tx.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, t.conn.queryErr("query_row", q, err)
	}
	defer rows.Close()
	return rowsFirst(rows)
}

func (t *Transaction) queryValue(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query_value", args)
	if err != nil {
		return nil, err
	}
	if t.dryRun {
		return starlark.None, nil
	}
	if t.done {
		return nil, fmt.Errorf("sql.query_value: transaction already finished")
	}
	rows, err := t.tx.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, t.conn.queryErr("query_value", q, err)
	}
	defer rows.Close()
	return scanFirstValue(rows)
}

func (t *Transaction) queryColumn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query_column", args)
	if err != nil {
		return nil, err
	}
	if t.dryRun {
		return starlark.NewList(nil), nil
	}
	if t.done {
		return nil, fmt.Errorf("sql.query_column: transaction already finished")
	}
	rows, err := t.tx.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, t.conn.queryErr("query_column", q, err)
	}
	defer rows.Close()
	return scanColumn(rows)
}

func (t *Transaction) execMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.exec", args)
	if err != nil {
		return nil, err
	}
	if t.dryRun {
		return dryRunResult(), nil
	}
	if t.done {
		return nil, fmt.Errorf("sql.exec: transaction already finished")
	}
	res, err := t.tx.ExecContext(context.Background(), q, params...)
	if err != nil {
		return nil, t.conn.queryErr("exec", q, err)
	}
	return execResult(res), nil
}

func (t *Transaction) commit(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if t.done || t.dryRun {
		return starlark.None, nil
	}
	t.done = true
	if err := t.tx.Commit(); err != nil {
		return nil, fmt.Errorf("sql.commit: %w", err)
	}
	return starlark.None, nil
}

func (t *Transaction) rollback(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if t.done || t.dryRun {
		return starlark.None, nil // no-op if already committed/rolled back
	}
	t.done = true
	if err := t.tx.Rollback(); err != nil {
		return nil, fmt.Errorf("sql.rollback: %w", err)
	}
	return starlark.None, nil
}
