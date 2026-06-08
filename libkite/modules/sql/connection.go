package sql

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

// Connection is a Starlark value wrapping a *sql.DB connection pool.
type Connection struct {
	db     *sql.DB
	driver string
	dsn    string
	dryRun bool

	mu     sync.Mutex
	closed bool
}

// --- starlark.Value ---

func (c *Connection) String() string        { return fmt.Sprintf("<sql.connection %s>", c.driver) }
func (c *Connection) Type() string          { return "sql.connection" }
func (c *Connection) Freeze()               {} // no-op: callable from deferred cleanup after globals freeze
func (c *Connection) Truth() starlark.Bool  { return starlark.Bool(!c.closed) }
func (c *Connection) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: sql.connection") }

// --- starlark.HasAttrs ---

func (c *Connection) Attr(name string) (starlark.Value, error) {
	if base, ok := strings.CutPrefix(name, "try_"); ok {
		if b := c.method(base); b != nil {
			return libkite.TryWrap("sql.connection."+name, b), nil
		}
		return nil, nil
	}
	if name == "driver" {
		return starlark.String(c.driver), nil
	}
	if b := c.method(name); b != nil {
		return b, nil
	}
	return nil, nil
}

func (c *Connection) method(name string) *starlark.Builtin {
	switch name {
	case "query":
		return starlark.NewBuiltin("sql.connection.query", c.query)
	case "query_row":
		return starlark.NewBuiltin("sql.connection.query_row", c.queryRow)
	case "query_value":
		return starlark.NewBuiltin("sql.connection.query_value", c.queryValue)
	case "query_column":
		return starlark.NewBuiltin("sql.connection.query_column", c.queryColumn)
	case "query_each":
		return starlark.NewBuiltin("sql.connection.query_each", c.queryEach)
	case "exec":
		return starlark.NewBuiltin("sql.connection.exec", c.execMethod)
	case "exec_many":
		return starlark.NewBuiltin("sql.connection.exec_many", c.execMany)
	case "tx":
		return starlark.NewBuiltin("sql.connection.tx", c.tx)
	case "batch":
		return starlark.NewBuiltin("sql.connection.batch", c.batch)
	case "begin":
		return starlark.NewBuiltin("sql.connection.begin", c.begin)
	case "ping":
		return starlark.NewBuiltin("sql.connection.ping", c.ping)
	case "close":
		return starlark.NewBuiltin("sql.connection.close", c.closeMethod)
	case "stats":
		return starlark.NewBuiltin("sql.connection.stats", c.stats)
	}
	return nil
}

func (c *Connection) AttrNames() []string {
	names := []string{
		"batch", "begin", "close", "driver", "exec", "exec_many", "ping",
		"query", "query_column", "query_each", "query_row", "query_value", "stats", "tx",
	}
	for _, n := range []string{"query", "query_row", "query_value", "query_column", "exec", "exec_many", "tx", "batch", "begin", "ping"} {
		names = append(names, "try_"+n)
	}
	sort.Strings(names)
	return names
}

// queryErr wraps an execution error, adding a driver-aware placeholder hint when
// a likely dialect mismatch is detected.
func (c *Connection) queryErr(op, query string, err error) error {
	if c.driver == "postgres" && strings.Contains(query, "?") {
		return fmt.Errorf("sql.%s: %w (postgres uses $1, $2 placeholders, not ?)", op, err)
	}
	return fmt.Errorf("sql.%s: %w", op, err)
}

// --- methods ---

func (c *Connection) query(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query", args)
	if err != nil {
		return nil, err
	}
	if c.dryRun {
		return starlark.NewList(nil), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.query: connection is closed")
	}
	rows, err := c.db.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, c.queryErr("query", q, err)
	}
	defer rows.Close()
	return rowsToList(rows)
}

func (c *Connection) queryRow(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.query_row", args)
	if err != nil {
		return nil, err
	}
	if c.dryRun {
		return starlark.None, nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.query_row: connection is closed")
	}
	rows, err := c.db.QueryContext(context.Background(), q, params...)
	if err != nil {
		return nil, c.queryErr("query_row", q, err)
	}
	defer rows.Close()
	return rowsFirst(rows)
}

func (c *Connection) execMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	q, params, err := queryArgs("sql.exec", args)
	if err != nil {
		return nil, err
	}
	if c.dryRun {
		return dryRunResult(), nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.exec: connection is closed")
	}
	res, err := c.db.ExecContext(context.Background(), q, params...)
	if err != nil {
		return nil, c.queryErr("exec", q, err)
	}
	return execResult(res), nil
}

func (c *Connection) begin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.dryRun {
		return &Transaction{conn: c, dryRun: true}, nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.begin: connection is closed")
	}
	tx, err := c.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, fmt.Errorf("sql.begin: %w", err)
	}
	return &Transaction{tx: tx, conn: c}, nil
}

func (c *Connection) ping(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.dryRun {
		return starlark.None, nil
	}
	if c.closed {
		return nil, fmt.Errorf("sql.ping: connection is closed")
	}
	if err := c.db.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("sql.ping: %w", err)
	}
	return starlark.None, nil
}

func (c *Connection) closeMethod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := c.shutdown(); err != nil {
		return nil, fmt.Errorf("sql.close: %w", err)
	}
	return starlark.None, nil
}

// shutdown closes the underlying pool exactly once. Safe to call from both the
// close() method and runtime auto-cleanup.
func (c *Connection) shutdown() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.dryRun {
		return nil
	}
	c.closed = true
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func (c *Connection) stats(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if c.dryRun || c.db == nil {
		return statsStruct(sql.DBStats{}), nil
	}
	return statsStruct(c.db.Stats()), nil
}
