package sql

import (
	"database/sql"
	"fmt"
	"time"

	"go.starlark.net/starlark"
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

// execResultDict builds the {rows_affected, last_insert_id} dict from a Result.
// last_insert_id is omitted when the driver does not report it (e.g. postgres).
func execResultDict(res sql.Result) *starlark.Dict {
	d := starlark.NewDict(2)
	if n, err := res.RowsAffected(); err == nil {
		d.SetKey(starlark.String("rows_affected"), starlark.MakeInt64(n))
	}
	if id, err := res.LastInsertId(); err == nil {
		d.SetKey(starlark.String("last_insert_id"), starlark.MakeInt64(id))
	}
	return d
}
