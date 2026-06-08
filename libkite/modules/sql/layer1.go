package sql

import (
	"context"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// --- managed transaction: db.tx(fn, retry=N) ---

func (c *Connection) tx(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("sql.tx: missing function argument")
	}
	callback, ok := args[0].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("sql.tx: first argument must be a function, got %s", args[0].Type())
	}
	retries := intKwarg(kwargs, "retry", 0)

	if c.dryRun {
		return starlark.Call(thread, callback, starlark.Tuple{&Transaction{conn: c, dryRun: true}}, nil)
	}
	if c.closed {
		return nil, fmt.Errorf("sql.tx: connection is closed")
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		v, err := c.runManaged(thread, callback)
		if err == nil {
			return v, nil
		}
		lastErr = err
		if !isRetryable(c.driver, err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("sql.tx: failed after %d attempt(s): %w", retries+1, lastErr)
}

// runManaged executes the callback inside a single transaction: commit on a
// clean return, rollback on error. If the callback committed or rolled back
// itself, that decision is honored.
func (c *Connection) runManaged(thread *starlark.Thread, callback starlark.Callable) (starlark.Value, error) {
	sqlTx, err := c.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("sql.tx: %w", err)
	}
	txv := &Transaction{tx: sqlTx, conn: c}

	v, callErr := starlark.Call(thread, callback, starlark.Tuple{txv}, nil)
	if callErr != nil {
		if !txv.done {
			txv.done = true
			sqlTx.Rollback()
		}
		return nil, callErr
	}
	if !txv.done {
		txv.done = true
		if err := sqlTx.Commit(); err != nil {
			return nil, fmt.Errorf("sql.tx: commit failed: %w", err)
		}
	}
	return v, nil
}

// --- declarative batch: db.batch([sql.stmt(...), ...]) ---

func (c *Connection) batch(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sql.batch: expects a single list of sql.stmt")
	}
	list, ok := args[0].(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("sql.batch: argument must be a list of sql.stmt, got %s", args[0].Type())
	}
	stmts := make([]*Stmt, 0, list.Len())
	named := 0
	for i := 0; i < list.Len(); i++ {
		s, ok := list.Index(i).(*Stmt)
		if !ok {
			return nil, fmt.Errorf("sql.batch: element %d is %s, not a sql.stmt", i, list.Index(i).Type())
		}
		stmts = append(stmts, s)
		if s.name != "" {
			named++
		}
	}
	if named != 0 && named != len(stmts) {
		return nil, fmt.Errorf("sql.batch: name all statements or none")
	}
	allNamed := len(stmts) > 0 && named == len(stmts)

	if c.dryRun {
		printBatchPlan(stmts)
		out := make([]*starlarkstruct.Struct, len(stmts))
		for i := range stmts {
			out[i] = dryRunResult()
		}
		return assembleBatch(stmts, out, allNamed), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.batch: connection is closed")
	}

	sqlTx, err := c.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("sql.batch: %w", err)
	}
	results := make([]*starlarkstruct.Struct, len(stmts))
	for i, s := range stmts {
		res, err := sqlTx.ExecContext(context.Background(), s.sql, s.params...)
		if err != nil {
			sqlTx.Rollback()
			return nil, fmt.Errorf("sql.batch: statement %s failed: %w (all statements rolled back)", stmtLabel(s, i), err)
		}
		results[i] = execResult(res)
	}
	if err := sqlTx.Commit(); err != nil {
		return nil, fmt.Errorf("sql.batch: commit failed: %w", err)
	}
	return assembleBatch(stmts, results, allNamed), nil
}

// assembleBatch returns the batch results keyed by name (when all named) or as
// a positional list.
func assembleBatch(stmts []*Stmt, results []*starlarkstruct.Struct, named bool) starlark.Value {
	if named {
		d := starlark.NewDict(len(stmts))
		for i, s := range stmts {
			d.SetKey(starlark.String(s.name), results[i])
		}
		return d
	}
	vals := make([]starlark.Value, len(results))
	for i, r := range results {
		vals[i] = r
	}
	return starlark.NewList(vals)
}

func stmtLabel(s *Stmt, i int) string {
	if s.name != "" {
		return fmt.Sprintf("%q", s.name)
	}
	return fmt.Sprintf("at index %d", i)
}

func printBatchPlan(stmts []*Stmt) {
	fmt.Printf("sql.batch (dry-run): %d statement(s), atomic\n", len(stmts))
	for i, s := range stmts {
		fmt.Printf("  [%d] %s%s\n", i, namePrefix(s), s.sql)
	}
}

func namePrefix(s *Stmt) string {
	if s.name != "" {
		return s.name + ": "
	}
	return ""
}

// --- read helpers ---

func (c *Connection) queryValue(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query_value", args)
	if err != nil {
		return nil, err
	}
	if c.dryRun {
		return starlark.None, nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.query_value: connection is closed")
	}
	rows, err := c.db.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, c.queryErr("query_value", q, err)
	}
	defer rows.Close()
	return scanFirstValue(rows)
}

func (c *Connection) queryColumn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query_column", args)
	if err != nil {
		return nil, err
	}
	if c.dryRun {
		return starlark.NewList(nil), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.query_column: connection is closed")
	}
	rows, err := c.db.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, c.queryErr("query_column", q, err)
	}
	defer rows.Close()
	return scanColumn(rows)
}

func (c *Connection) queryEach(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("sql.query_each: requires (query, fn, *params)")
	}
	q, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("sql.query_each: query must be a string")
	}
	callback, ok := args[1].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf("sql.query_each: second argument must be a function, got %s", args[1].Type())
	}
	params := make([]any, 0, len(args)-2)
	for i, a := range args[2:] {
		v, err := starlarkToGo(a)
		if err != nil {
			return nil, fmt.Errorf("sql.query_each: parameter %d: %w", i+1, err)
		}
		params = append(params, v)
	}
	if c.dryRun {
		return starlark.None, nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.query_each: connection is closed")
	}
	rows, err := c.db.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, c.queryErr("query_each", q, err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		dict, err := rowToDict(rows, cols)
		if err != nil {
			return nil, err
		}
		if _, err := starlark.Call(thread, callback, starlark.Tuple{dict}, nil); err != nil {
			return nil, err
		}
	}
	return starlark.None, rows.Err()
}

func (c *Connection) execMany(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("sql.exec_many: requires (query, [param-sets])")
	}
	q, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("sql.exec_many: query must be a string")
	}
	sets, ok := args[1].(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("sql.exec_many: second argument must be a list of parameter lists")
	}
	if c.dryRun {
		return dryRunResult(), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.exec_many: connection is closed")
	}

	sqlTx, err := c.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("sql.exec_many: %w", err)
	}
	prepared, err := sqlTx.PrepareContext(context.Background(), q)
	if err != nil {
		sqlTx.Rollback()
		return nil, c.queryErr("exec_many", q, err)
	}
	defer prepared.Close()

	var total int64
	for i := 0; i < sets.Len(); i++ {
		params, err := paramList(sets.Index(i))
		if err != nil {
			sqlTx.Rollback()
			return nil, fmt.Errorf("sql.exec_many: set %d: %w", i, err)
		}
		res, err := prepared.ExecContext(context.Background(), params...)
		if err != nil {
			sqlTx.Rollback()
			return nil, fmt.Errorf("sql.exec_many: set %d: %w (rolled back)", i, err)
		}
		if n, err := res.RowsAffected(); err == nil {
			total += n
		}
	}
	if err := sqlTx.Commit(); err != nil {
		return nil, fmt.Errorf("sql.exec_many: commit failed: %w", err)
	}
	return starlarkstruct.FromStringDict(starlark.String("sql.result"), starlark.StringDict{
		"rows_affected":  starlark.MakeInt64(total),
		"last_insert_id": starlark.None,
	}), nil
}

// paramList converts a Starlark list/tuple of values into a Go param slice.
func paramList(v starlark.Value) ([]any, error) {
	var iter starlark.Iterator
	var n int
	switch t := v.(type) {
	case *starlark.List:
		iter, n = t.Iterate(), t.Len()
	case starlark.Tuple:
		iter, n = t.Iterate(), t.Len()
	default:
		return nil, fmt.Errorf("each parameter set must be a list or tuple, got %s", v.Type())
	}
	defer iter.Done()
	out := make([]any, 0, n)
	var elem starlark.Value
	for iter.Next(&elem) {
		g, err := starlarkToGo(elem)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func intKwarg(kwargs []starlark.Tuple, key string, def int) int {
	for _, kv := range kwargs {
		if name, ok := kv[0].(starlark.String); ok && string(name) == key {
			if i, ok := kv[1].(starlark.Int); ok {
				if v, ok := i.Int64(); ok {
					return int(v)
				}
			}
		}
	}
	return def
}

// isRetryable reports whether a transaction error is a transient
// contention/serialization failure worth retrying.
func isRetryable(driver string, err error) bool {
	msg := strings.ToLower(err.Error())
	switch driver {
	case "sqlite":
		return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database table is locked") || strings.Contains(msg, "sqlite_busy")
	case "postgres":
		return strings.Contains(msg, "40001") || strings.Contains(msg, "40p01") ||
			strings.Contains(msg, "serialization failure") || strings.Contains(msg, "deadlock")
	case "mysql":
		return strings.Contains(msg, "1213") || strings.Contains(msg, "1205") || strings.Contains(msg, "deadlock")
	}
	return false
}
