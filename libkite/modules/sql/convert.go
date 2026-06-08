package sql

import (
	"database/sql"
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// queryArgs splits a method's positional args into the SQL string and its
// parameters. Parameters are passed positionally after the query, matching the
// driver-native placeholder style (? for sqlite/mysql, $N for postgres).
func queryArgs(label string, args starlark.Tuple) (string, []any, error) {
	if len(args) < 1 {
		return "", nil, fmt.Errorf("%s: missing query string", label)
	}
	q, ok := starlark.AsString(args[0])
	if !ok {
		return "", nil, fmt.Errorf("%s: query must be a string", label)
	}
	params := make([]any, 0, len(args)-1)
	for i, a := range args[1:] {
		v, err := starlarkToGo(a)
		if err != nil {
			return "", nil, fmt.Errorf("%s: parameter %d: %w", label, i+1, err)
		}
		params = append(params, v)
	}
	return q, params, nil
}

// starlarkToGo converts a Starlark parameter value to a Go value for binding.
// None binds SQL NULL.
func starlarkToGo(v starlark.Value) (any, error) {
	switch t := v.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(t), nil
	case starlark.Int:
		if i, ok := t.Int64(); ok {
			return i, nil
		}
		return t.String(), nil // out of int64 range — bind as text
	case starlark.Float:
		return float64(t), nil
	case starlark.String:
		return string(t), nil
	case starlark.Bytes:
		return []byte(t), nil
	default:
		return nil, fmt.Errorf("unsupported parameter type %s", v.Type())
	}
}

// goToStarlark converts a scanned database value to a Starlark value.
func goToStarlark(val any) starlark.Value {
	switch v := val.(type) {
	case nil:
		return starlark.None
	case int64:
		return starlark.MakeInt64(v)
	case float64:
		return starlark.Float(v)
	case bool:
		return starlark.Bool(v)
	case string:
		return starlark.String(v)
	case []byte:
		return starlark.String(string(v))
	case time.Time:
		return starlark.String(v.Format(time.RFC3339))
	default:
		return starlark.String(fmt.Sprintf("%v", v))
	}
}

// rowToDict scans the current row into a dict keyed by column name.
func rowToDict(rows *sql.Rows, columns []string) (*starlark.Dict, error) {
	values := make([]any, len(columns))
	ptrs := make([]any, len(columns))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	dict := starlark.NewDict(len(columns))
	for i, col := range columns {
		if err := dict.SetKey(starlark.String(col), goToStarlark(values[i])); err != nil {
			return nil, err
		}
	}
	return dict, nil
}

// rowsToList materializes all rows as a Starlark list of dicts.
func rowsToList(rows *sql.Rows) (*starlark.List, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []starlark.Value
	for rows.Next() {
		dict, err := rowToDict(rows, columns)
		if err != nil {
			return nil, err
		}
		out = append(out, dict)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return starlark.NewList(out), nil
}

// scanFirstValue returns the first column of the first row, or None.
func scanFirstValue(rows *sql.Rows) (starlark.Value, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		return starlark.None, rows.Err()
	}
	values := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range values {
		ptrs[i] = &values[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	return goToStarlark(values[0]), nil
}

// scanColumn returns the first column of every row as a flat list.
func scanColumn(rows *sql.Rows) (*starlark.List, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []starlark.Value
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		out = append(out, goToStarlark(values[0]))
	}
	return starlark.NewList(out), rows.Err()
}

// rowsFirst returns the first row as a dict, or None when there are no rows.
func rowsFirst(rows *sql.Rows) (starlark.Value, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return starlark.None, nil
	}
	dict, err := rowToDict(rows, columns)
	if err != nil {
		return nil, err
	}
	return dict, nil
}

// execResult builds the result struct from a Result. Both fields are always
// present: last_insert_id is None when the driver does not report it (e.g.
// postgres, which uses RETURNING instead).
func execResult(res sql.Result) *starlarkstruct.Struct {
	rows := starlark.Value(starlark.MakeInt(0))
	if n, err := res.RowsAffected(); err == nil {
		rows = starlark.MakeInt64(n)
	}
	last := starlark.Value(starlark.None)
	if id, err := res.LastInsertId(); err == nil {
		last = starlark.MakeInt64(id)
	}
	return starlarkstruct.FromStringDict(starlark.String("sql.result"), starlark.StringDict{
		"rows_affected":  rows,
		"last_insert_id": last,
	})
}

// dryRunResult is the exec result returned in dry-run mode.
func dryRunResult() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("sql.result"), starlark.StringDict{
		"rows_affected":  starlark.MakeInt(0),
		"last_insert_id": starlark.None,
	})
}

// statsStruct builds the connection-pool stats struct.
func statsStruct(s sql.DBStats) *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("sql.stats"), starlark.StringDict{
		"open":       starlark.MakeInt(s.OpenConnections),
		"in_use":     starlark.MakeInt(s.InUse),
		"idle":       starlark.MakeInt(s.Idle),
		"max_open":   starlark.MakeInt(s.MaxOpenConnections),
		"wait_count": starlark.MakeInt64(s.WaitCount),
	})
}
