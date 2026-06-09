package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// placeholderStyle maps a driver to its parameter placeholder style. The style
// is not discoverable through database/sql (the stdlib does not model SQL
// syntax), so it is documented knowledge carried here. Used when the module
// generates SQL (insert, migrate); hand-written SQL is driver-native.
var placeholderStyle = map[string]string{
	"sqlite":   "?",
	"mysql":    "?",
	"postgres": "$",
}

// resolveStyle returns the placeholder style for a generated statement: the
// explicit override when given, else the connection driver's default.
func (c *Connection) resolveStyle(override string) (string, error) {
	if override != "" {
		if override != "?" && override != "$" {
			return "", fmt.Errorf("placeholder must be \"?\" or \"$\", got %q", override)
		}
		return override, nil
	}
	if s, ok := placeholderStyle[c.driver]; ok {
		return s, nil
	}
	return "", fmt.Errorf("no default placeholder style for driver %q; pass placeholder=\"?\" or \"$\"", c.driver)
}

// renderPlaceholder renders the nth (1-based) placeholder for a style.
func renderPlaceholder(style string, n int) string {
	if style == "$" {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

func kwargStr(kwargs []starlark.Tuple, key string) string {
	for _, kv := range kwargs {
		if name, ok := kv[0].(starlark.String); ok && string(name) == key {
			if s, ok := starlark.AsString(kv[1]); ok {
				return s
			}
		}
	}
	return ""
}

// --- db.insert(table, row | rows, placeholder=?) ---

func (c *Connection) insert(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("sql.insert: requires (table, row | rows)")
	}
	table, ok := starlark.AsString(args[0])
	if !ok {
		return nil, fmt.Errorf("sql.insert: table must be a string")
	}
	style, err := c.resolveStyle(kwargStr(kwargs, "placeholder"))
	if err != nil {
		return nil, fmt.Errorf("sql.insert: %w", err)
	}

	rows, err := insertRows(args[1])
	if err != nil {
		return nil, fmt.Errorf("sql.insert: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("sql.insert: no rows to insert")
	}

	query, params, err := buildInsert(table, rows, style)
	if err != nil {
		return nil, fmt.Errorf("sql.insert: %w", err)
	}

	if c.dryRun {
		return dryRunResult(), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.insert: connection is closed")
	}
	res, err := c.db.ExecContext(context.Background(), query, params...)
	if err != nil {
		return nil, fmt.Errorf("sql.insert: %w", err)
	}
	return execResult(res), nil
}

// insertRows normalizes the second insert argument into a slice of row dicts.
func insertRows(v starlark.Value) ([]*starlark.Dict, error) {
	switch t := v.(type) {
	case *starlark.Dict:
		return []*starlark.Dict{t}, nil
	case *starlark.List:
		out := make([]*starlark.Dict, 0, t.Len())
		for i := 0; i < t.Len(); i++ {
			d, ok := t.Index(i).(*starlark.Dict)
			if !ok {
				return nil, fmt.Errorf("row %d is %s, want a dict", i, t.Index(i).Type())
			}
			out = append(out, d)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("second argument must be a dict or list of dicts, got %s", v.Type())
	}
}

// buildInsert renders a single- or multi-row INSERT and its flat parameter list.
// Columns come from the first row; every row must carry the same columns.
func buildInsert(table string, rows []*starlark.Dict, style string) (string, []any, error) {
	cols := make([]string, 0, rows[0].Len())
	for _, k := range rows[0].Keys() {
		s, ok := starlark.AsString(k)
		if !ok {
			return "", nil, fmt.Errorf("column name must be a string, got %s", k.Type())
		}
		cols = append(cols, s)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES ", table, strings.Join(cols, ", "))
	params := make([]any, 0, len(cols)*len(rows))
	n := 0
	for ri, row := range rows {
		if row.Len() != len(cols) {
			return "", nil, fmt.Errorf("row %d has different columns than the first row", ri)
		}
		if ri > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('(')
		for ci, col := range cols {
			val, found, err := row.Get(starlark.String(col))
			if err != nil {
				return "", nil, err
			}
			if !found {
				return "", nil, fmt.Errorf("row %d is missing column %q", ri, col)
			}
			g, err := starlarkToGo(val)
			if err != nil {
				return "", nil, fmt.Errorf("row %d column %q: %w", ri, col, err)
			}
			params = append(params, g)
			n++
			if ci > 0 {
				b.WriteString(", ")
			}
			b.WriteString(renderPlaceholder(style, n))
		}
		b.WriteByte(')')
	}
	return b.String(), params, nil
}

// --- db.migrate([sql.stmt(ddl, name="..."), ...], placeholder=?) ---

func (c *Connection) migrate(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("sql.migrate: requires a list of named sql.stmt")
	}
	list, ok := args[0].(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("sql.migrate: argument must be a list of sql.stmt, got %s", args[0].Type())
	}
	stmts := make([]*Stmt, 0, list.Len())
	for i := 0; i < list.Len(); i++ {
		s, ok := list.Index(i).(*Stmt)
		if !ok {
			return nil, fmt.Errorf("sql.migrate: element %d is %s, not a sql.stmt", i, list.Index(i).Type())
		}
		if s.name == "" {
			return nil, fmt.Errorf("sql.migrate: statement at index %d has no name; each migration needs name=\"…\"", i)
		}
		stmts = append(stmts, s)
	}
	style, err := c.resolveStyle(kwargStr(kwargs, "placeholder"))
	if err != nil {
		return nil, fmt.Errorf("sql.migrate: %w", err)
	}

	if c.dryRun {
		printMigratePlan(stmts)
		return migrateSummary(nil, nil), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.migrate: connection is closed")
	}

	ctx := context.Background()
	// VARCHAR(255) PK works across sqlite/postgres/mysql (TEXT cannot be a PK in MySQL).
	if _, err := c.db.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS schema_migrations (name VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP)"); err != nil {
		return nil, fmt.Errorf("sql.migrate: creating tracking table: %w", err)
	}

	checkQ := "SELECT 1 FROM schema_migrations WHERE name = " + renderPlaceholder(style, 1)
	recordQ := "INSERT INTO schema_migrations (name) VALUES (" + renderPlaceholder(style, 1) + ")"

	var applied, skipped []string
	for _, s := range stmts {
		var one int
		switch err := c.db.QueryRowContext(ctx, checkQ, s.name).Scan(&one); {
		case err == nil:
			skipped = append(skipped, s.name)
			continue
		case errors.Is(err, sql.ErrNoRows):
			// not applied — fall through to apply
		default:
			return nil, fmt.Errorf("sql.migrate: checking %q: %w", s.name, err)
		}

		tx, err := c.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("sql.migrate: %w", err)
		}
		if _, err := tx.ExecContext(ctx, s.sql, s.params...); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("sql.migrate: applying %q: %w (rolled back)", s.name, err)
		}
		if _, err := tx.ExecContext(ctx, recordQ, s.name); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("sql.migrate: recording %q: %w", s.name, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("sql.migrate: committing %q: %w", s.name, err)
		}
		applied = append(applied, s.name)
	}
	return migrateSummary(applied, skipped), nil
}

func migrateSummary(applied, skipped []string) *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("sql.migration"), starlark.StringDict{
		"applied": stringList(applied),
		"skipped": stringList(skipped),
	})
}

func printMigratePlan(stmts []*Stmt) {
	fmt.Printf("sql.migrate (dry-run): %d migration(s)\n", len(stmts))
	for _, s := range stmts {
		fmt.Printf("  %s\n", s.name)
	}
}

func stringList(ss []string) *starlark.List {
	vals := make([]starlark.Value, len(ss))
	for i, s := range ss {
		vals[i] = starlark.String(s)
	}
	return starlark.NewList(vals)
}
