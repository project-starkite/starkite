// Package sql provides database connectivity for starkite.
// This is a factory module: sql.open() returns a database connection.
package sql

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vladimirvivien/startype"
	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver (no CGO)
)

const ModuleName libkite.ModuleName = "sql"

// driverMapping maps friendly driver names to the registered database/sql
// driver name. A driver is usable only when its package is built into the
// running edition; sqlite ships in libkite, postgres/mysql in the all-in-one.
var driverMapping = map[string]string{
	"sqlite":   "sqlite",
	"postgres": "pgx",
	"mysql":    "mysql",
}

// Module implements database connectivity.
type Module struct {
	once   sync.Once
	module starlark.Value
	config *libkite.ModuleConfig
}

func New() *Module { return &Module{} }

func (m *Module) Name() libkite.ModuleName { return ModuleName }

func (m *Module) Description() string {
	return "sql provides database connectivity: open() returns a connection for queries, execs, and transactions"
}

func (m *Module) Aliases() starlark.StringDict { return nil }

func (m *Module) FactoryMethod() string { return "open" }

func (m *Module) Load(config *libkite.ModuleConfig) (starlark.StringDict, error) {
	m.once.Do(func() {
		m.config = config
		members := starlark.StringDict{
			"open": starlark.NewBuiltin("sql.open", m.open),
			"stmt": starlark.NewBuiltin("sql.stmt", m.stmt),
		}
		m.module = libkite.NewTryModule(string(ModuleName), members)
	})
	return starlark.StringDict{string(ModuleName): m.module}, nil
}

// poolKeys are the open() kwargs handled here rather than by startype.
var poolKeys = map[string]bool{
	"max_open": true, "max_idle": true, "max_lifetime": true, "max_idle_time": true,
}

// open connects to a database and returns a Connection.
//
// Usage: sql.open("sqlite", "app.db", max_open=25)
func (m *Module) open(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var p struct {
		Driver string `name:"driver" position:"0" required:"true"`
		DSN    string `name:"dsn" position:"1" required:"true"`
	}
	simple, pool := splitKwargs(kwargs)
	if err := startype.Args(args, simple).Go(&p); err != nil {
		return nil, err
	}

	// Permission check. The driver is the function so the ladder can gate
	// per-driver via rules like sql.open(sqlite:**) / sql.open(postgres:**):
	// sqlite under allow-fs, postgres/mysql under allow-net.
	if err := libkite.Check(thread, "sql", "open", p.Driver, sanitizeDSN(p.DSN)); err != nil {
		return nil, err
	}

	regName, known := driverMapping[p.Driver]
	if !known {
		return nil, fmt.Errorf("sql.open: unknown driver %q (known: sqlite, postgres, mysql)", p.Driver)
	}
	if !driverRegistered(regName) {
		return nil, fmt.Errorf("sql.open: driver %q is not available in this build; it ships in the all-in-one kite binary", p.Driver)
	}

	if m.config != nil && m.config.DryRun {
		return &Connection{driver: p.Driver, dsn: p.DSN, dryRun: true}, nil
	}

	dsn := p.DSN
	maxOpen := pool.intOr("max_open", 25)
	if p.Driver == "sqlite" {
		if isMemoryDSN(dsn) {
			// Each pooled connection to :memory: is a separate database; pin to one.
			maxOpen = 1
		} else {
			// Concurrent handlers sharing a file DB need a busy timeout and WAL
			// to avoid SQLITE_BUSY.
			dsn = sqliteFileDSN(dsn)
		}
	}

	db, err := sql.Open(regName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.open: %w", err)
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(pool.intOr("max_idle", 5))
	db.SetConnMaxLifetime(time.Duration(pool.intOr("max_lifetime", 300)) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(pool.intOr("max_idle_time", 60)) * time.Second)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sql.open: connection failed: %w", err)
	}

	return &Connection{db: db, driver: p.Driver, dsn: p.DSN}, nil
}

// driverRegistered reports whether the named database/sql driver is built in.
func driverRegistered(name string) bool {
	for _, d := range sql.Drivers() {
		if d == name {
			return true
		}
	}
	return false
}

// isMemoryDSN reports whether a sqlite DSN names an in-memory database.
func isMemoryDSN(dsn string) bool {
	return dsn == ":memory:" || strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory")
}

// sqliteFileDSN adds busy-timeout and WAL pragmas to a sqlite file DSN.
// modernc.org/sqlite reads _pragma query parameters.
func sqliteFileDSN(dsn string) string {
	if strings.Contains(dsn, "_pragma=") {
		return dsn // caller already set pragmas
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// sanitizeDSN removes any password from a DSN for permission patterns and logs.
func sanitizeDSN(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.User != nil {
		u.User = url.User(u.User.Username())
		return u.String()
	}
	return dsn
}

// poolOpts holds the parsed pool kwargs.
type poolOpts struct{ kwargs []starlark.Tuple }

func (p poolOpts) intOr(key string, def int) int {
	for _, kv := range p.kwargs {
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

// splitKwargs separates pool kwargs (handled here) from the rest (parsed by
// startype into the open() struct).
func splitKwargs(kwargs []starlark.Tuple) ([]starlark.Tuple, poolOpts) {
	var simple []starlark.Tuple
	var pool []starlark.Tuple
	for _, kv := range kwargs {
		if name, ok := kv[0].(starlark.String); ok && poolKeys[string(name)] {
			pool = append(pool, kv)
			continue
		}
		simple = append(simple, kv)
	}
	return simple, poolOpts{kwargs: pool}
}
