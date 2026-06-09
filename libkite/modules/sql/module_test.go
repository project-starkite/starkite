package sql

import (
	"fmt"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/project-starkite/starkite/libkite"
)

func threadWith(t *testing.T, cfg *libkite.PermissionConfig) *starlark.Thread {
	t.Helper()
	th := &starlark.Thread{Name: "sql-test"}
	checker, err := libkite.NewPermissionChecker(cfg)
	if err != nil {
		t.Fatalf("NewPermissionChecker: %v", err)
	}
	libkite.SetPermissions(th, checker)
	return th
}

func openMem(t *testing.T, th *starlark.Thread) *Connection {
	t.Helper()
	m := New()
	if _, err := m.Load(&libkite.ModuleConfig{}); err != nil {
		t.Fatalf("Load: %v", err)
	}
	v, err := m.open(th, nil, starlark.Tuple{starlark.String("sqlite"), starlark.String(":memory:")}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	c, ok := v.(*Connection)
	if !ok {
		t.Fatalf("open returned %T, want *Connection", v)
	}
	return c
}

// callMethod looks up a connection/transaction method and calls it.
func callMethod(t *testing.T, th *starlark.Thread, v starlark.HasAttrs, name string, args ...starlark.Value) starlark.Value {
	t.Helper()
	attr, err := v.Attr(name)
	if err != nil || attr == nil {
		t.Fatalf("Attr(%q): %v", name, err)
	}
	b, ok := attr.(*starlark.Builtin)
	if !ok {
		t.Fatalf("Attr(%q) is %T, want *starlark.Builtin", name, attr)
	}
	res, err := starlark.Call(th, b, starlark.Tuple(args), nil)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

// structAttr reads a named attribute from a starlarkstruct result.
func structAttr(t *testing.T, v starlark.Value, name string) starlark.Value {
	t.Helper()
	s, ok := v.(starlark.HasAttrs)
	if !ok {
		t.Fatalf("value %T is not a struct", v)
	}
	a, err := s.Attr(name)
	if err != nil || a == nil {
		t.Fatalf("Attr(%q): %v", name, err)
	}
	return a
}

func TestConnectionCRUD(t *testing.T) {
	th := threadWith(t, libkite.AllowAllPermissions())
	c := openMem(t, th)
	defer c.db.Close()

	callMethod(t, th, c, "exec", starlark.String(
		"CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, active BOOLEAN)"))

	res := callMethod(t, th, c, "exec",
		starlark.String("INSERT INTO users (name, active) VALUES (?, ?)"),
		starlark.String("alice"), starlark.Bool(true))
	if got := structAttr(t, res, "last_insert_id"); got != starlark.MakeInt(1) {
		t.Errorf("last_insert_id = %v, want 1", got)
	}
	if got := structAttr(t, res, "rows_affected"); got != starlark.MakeInt(1) {
		t.Errorf("rows_affected = %v, want 1", got)
	}

	callMethod(t, th, c, "exec",
		starlark.String("INSERT INTO users (name, active) VALUES (?, ?)"),
		starlark.String("bob"), starlark.Bool(false))

	// query → list of dicts
	rows := callMethod(t, th, c, "query", starlark.String("SELECT id, name FROM users ORDER BY id")).(*starlark.List)
	if rows.Len() != 2 {
		t.Fatalf("query returned %d rows, want 2", rows.Len())
	}
	first := rows.Index(0).(*starlark.Dict)
	if name, _, _ := first.Get(starlark.String("name")); name != starlark.String("alice") {
		t.Errorf("row 0 name = %v, want alice", name)
	}

	// query_row → dict / None
	row := callMethod(t, th, c, "query_row", starlark.String("SELECT name FROM users WHERE id = ?"), starlark.MakeInt(2)).(*starlark.Dict)
	if name, _, _ := row.Get(starlark.String("name")); name != starlark.String("bob") {
		t.Errorf("query_row name = %v, want bob", name)
	}
	if none := callMethod(t, th, c, "query_row", starlark.String("SELECT name FROM users WHERE id = ?"), starlark.MakeInt(99)); none != starlark.None {
		t.Errorf("query_row for missing id = %v, want None", none)
	}
}

func TestTransactionCommitRollback(t *testing.T) {
	th := threadWith(t, libkite.AllowAllPermissions())
	c := openMem(t, th)
	defer c.db.Close()
	callMethod(t, th, c, "exec", starlark.String("CREATE TABLE t (n INTEGER)"))

	// commit persists
	tx := callMethod(t, th, c, "begin").(*Transaction)
	callMethod(t, th, tx, "exec", starlark.String("INSERT INTO t (n) VALUES (?)"), starlark.MakeInt(1))
	callMethod(t, th, tx, "commit")

	// rollback discards
	tx2 := callMethod(t, th, c, "begin").(*Transaction)
	callMethod(t, th, tx2, "exec", starlark.String("INSERT INTO t (n) VALUES (?)"), starlark.MakeInt(2))
	callMethod(t, th, tx2, "rollback")
	// rollback again is a no-op
	callMethod(t, th, tx2, "rollback")

	row := callMethod(t, th, c, "query_row", starlark.String("SELECT count(*) AS c FROM t")).(*starlark.Dict)
	if cnt, _, _ := row.Get(starlark.String("c")); cnt.(starlark.Int) != starlark.MakeInt(1) {
		t.Errorf("row count = %v, want 1 (only the committed insert)", cnt)
	}
}

func TestPermissionLadder(t *testing.T) {
	m := New()
	m.Load(&libkite.ModuleConfig{})

	t.Run("deny-all blocks sqlite", func(t *testing.T) {
		th := threadWith(t, libkite.DenyAllPermissions())
		if _, err := m.open(th, nil, starlark.Tuple{starlark.String("sqlite"), starlark.String(":memory:")}, nil); err == nil {
			t.Error("expected permission denial under deny-all")
		}
	})

	t.Run("allow-fs permits sqlite", func(t *testing.T) {
		th := threadWith(t, libkite.AllowFSPermissions())
		v, err := m.open(th, nil, starlark.Tuple{starlark.String("sqlite"), starlark.String(":memory:")}, nil)
		if err != nil {
			t.Fatalf("allow-fs should permit sqlite: %v", err)
		}
		v.(*Connection).db.Close()
	})

	t.Run("allow-fs denies postgres", func(t *testing.T) {
		th := threadWith(t, libkite.AllowFSPermissions())
		if _, err := m.open(th, nil, starlark.Tuple{starlark.String("postgres"), starlark.String("postgres://u:p@h/db")}, nil); err == nil {
			t.Error("expected permission denial for postgres under allow-fs")
		}
	})
}

func TestUnavailableDriver(t *testing.T) {
	m := New()
	m.Load(&libkite.ModuleConfig{})
	th := threadWith(t, libkite.AllowAllPermissions())
	// postgres is permitted under allow-all but not registered in libkite.
	if _, err := m.open(th, nil, starlark.Tuple{starlark.String("postgres"), starlark.String("postgres://u@h/db")}, nil); err == nil {
		t.Error("expected 'not available' error for postgres in libkite build")
	}
}

func TestSanitizeDSN(t *testing.T) {
	got := sanitizeDSN("postgres://user:secret@host:5432/db")
	if got != "postgres://user@host:5432/db" {
		t.Errorf("sanitizeDSN leaked password: %q", got)
	}
}

func TestBatchResultShapes(t *testing.T) {
	th := threadWith(t, libkite.AllowAllPermissions())
	c := openMem(t, th)
	defer c.db.Close()
	callMethod(t, th, c, "exec", starlark.String("CREATE TABLE t (id INTEGER PRIMARY KEY AUTOINCREMENT, n INTEGER)"))

	m := New()
	m.Load(&libkite.ModuleConfig{})
	mk := func(name, q string, args ...starlark.Value) starlark.Value {
		tup := append(starlark.Tuple{starlark.String(q)}, args...)
		var kw []starlark.Tuple
		if name != "" {
			kw = []starlark.Tuple{{starlark.String("name"), starlark.String(name)}}
		}
		v, err := m.stmt(th, nil, tup, kw)
		if err != nil {
			t.Fatalf("stmt: %v", err)
		}
		return v
	}

	t.Run("named → dict", func(t *testing.T) {
		res := callMethod(t, th, c, "batch", starlark.NewList([]starlark.Value{
			mk("a", "INSERT INTO t (n) VALUES (?)", starlark.MakeInt(1)),
			mk("b", "INSERT INTO t (n) VALUES (?)", starlark.MakeInt(2)),
		}))
		d, ok := res.(*starlark.Dict)
		if !ok {
			t.Fatalf("named batch returned %T, want dict", res)
		}
		av, _, _ := d.Get(starlark.String("a"))
		if structAttr(t, av, "rows_affected") != starlark.MakeInt(1) {
			t.Errorf("a.rows_affected != 1")
		}
	})

	t.Run("unnamed → list", func(t *testing.T) {
		res := callMethod(t, th, c, "batch", starlark.NewList([]starlark.Value{
			mk("", "INSERT INTO t (n) VALUES (?)", starlark.MakeInt(3)),
			mk("", "INSERT INTO t (n) VALUES (?)", starlark.MakeInt(4)),
		}))
		if _, ok := res.(*starlark.List); !ok {
			t.Fatalf("unnamed batch returned %T, want list", res)
		}
	})

	t.Run("query_value / query_column / exec_many", func(t *testing.T) {
		if v := callMethod(t, th, c, "query_value", starlark.String("SELECT count(*) FROM t")); v != starlark.MakeInt(4) {
			t.Errorf("query_value count = %v, want 4", v)
		}
		col := callMethod(t, th, c, "query_column", starlark.String("SELECT n FROM t ORDER BY n")).(*starlark.List)
		if col.Len() != 4 {
			t.Errorf("query_column len = %d, want 4", col.Len())
		}
		em := callMethod(t, th, c, "exec_many", starlark.String("INSERT INTO t (n) VALUES (?)"),
			starlark.NewList([]starlark.Value{
				starlark.NewList([]starlark.Value{starlark.MakeInt(10)}),
				starlark.NewList([]starlark.Value{starlark.MakeInt(11)}),
			}))
		if structAttr(t, em, "rows_affected") != starlark.MakeInt(2) {
			t.Errorf("exec_many rows_affected != 2")
		}
	})
}

func TestBatchRollback(t *testing.T) {
	th := threadWith(t, libkite.AllowAllPermissions())
	c := openMem(t, th)
	defer c.db.Close()
	callMethod(t, th, c, "exec", starlark.String("CREATE TABLE t (n INTEGER)"))

	m := New()
	m.Load(&libkite.ModuleConfig{})
	s1, _ := m.stmt(th, nil, starlark.Tuple{starlark.String("INSERT INTO t (n) VALUES (?)"), starlark.MakeInt(1)}, nil)
	s2, _ := m.stmt(th, nil, starlark.Tuple{starlark.String("INSERT INTO no_such_table VALUES (?)"), starlark.MakeInt(2)}, nil)

	attr, _ := c.Attr("batch")
	if _, err := starlark.Call(th, attr.(*starlark.Builtin), starlark.Tuple{starlark.NewList([]starlark.Value{s1, s2})}, nil); err == nil {
		t.Fatal("expected batch failure")
	}
	// first insert must have been rolled back
	v := callMethod(t, th, c, "query_value", starlark.String("SELECT count(*) FROM t"))
	if v != starlark.MakeInt(0) {
		t.Errorf("after failed batch, count = %v, want 0 (rolled back)", v)
	}
}

func TestIsRetryable(t *testing.T) {
	if !isRetryable("sqlite", fmt.Errorf("database is locked")) {
		t.Error("sqlite 'database is locked' should be retryable")
	}
	if isRetryable("sqlite", fmt.Errorf("no such column: x")) {
		t.Error("a syntax error should not be retryable")
	}
	if !isRetryable("postgres", fmt.Errorf("pq: could not serialize (SQLSTATE 40001)")) {
		t.Error("postgres 40001 should be retryable")
	}
}

func TestAutoCloseOnRunEnd(t *testing.T) {
	rt, err := libkite.NewTrusted(nil) // trusted → allow-all permissions
	if err != nil {
		t.Fatalf("NewTrusted: %v", err)
	}
	th := rt.NewThread("sql-autoclose")

	m := New()
	m.Load(&libkite.ModuleConfig{})
	v, err := m.open(th, nil, starlark.Tuple{starlark.String("sqlite"), starlark.String(":memory:")}, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	conn := v.(*Connection)
	if conn.closed {
		t.Fatal("connection should be open before run end")
	}

	rt.Close() // simulates run end → registered cleanups run
	if !conn.closed {
		t.Error("connection was not auto-closed at run end")
	}
}

func TestBuildInsert(t *testing.T) {
	mk := func(pairs ...string) *starlark.Dict {
		d := starlark.NewDict(len(pairs) / 2)
		for i := 0; i < len(pairs); i += 2 {
			d.SetKey(starlark.String(pairs[i]), starlark.String(pairs[i+1]))
		}
		return d
	}
	tests := []struct {
		name      string
		rows      []*starlark.Dict
		style     string
		wantSQL   string
		wantCount int
	}{
		{"single qmark", []*starlark.Dict{mk("name", "a", "email", "e")}, "?",
			"INSERT INTO users (name, email) VALUES (?, ?)", 2},
		{"single numbered", []*starlark.Dict{mk("name", "a", "email", "e")}, "$",
			"INSERT INTO users (name, email) VALUES ($1, $2)", 2},
		{"multi numbered", []*starlark.Dict{mk("n", "1"), mk("n", "2"), mk("n", "3")}, "$",
			"INSERT INTO users (n) VALUES ($1), ($2), ($3)", 3},
		{"multi qmark", []*starlark.Dict{mk("n", "1"), mk("n", "2")}, "?",
			"INSERT INTO users (n) VALUES (?), (?)", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, params, err := buildInsert("users", tt.rows, tt.style)
			if err != nil {
				t.Fatalf("buildInsert: %v", err)
			}
			if q != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", q, tt.wantSQL)
			}
			if len(params) != tt.wantCount {
				t.Errorf("params = %d, want %d", len(params), tt.wantCount)
			}
		})
	}

	t.Run("column mismatch", func(t *testing.T) {
		if _, _, err := buildInsert("t", []*starlark.Dict{mk("a", "1"), mk("b", "2")}, "?"); err == nil {
			t.Error("expected error for differing columns across rows")
		}
	})
}

func TestResolveStyle(t *testing.T) {
	c := &Connection{driver: "sqlite"}
	if s, _ := c.resolveStyle(""); s != "?" {
		t.Errorf("sqlite default = %q, want ?", s)
	}
	if s, _ := c.resolveStyle("$"); s != "$" {
		t.Errorf("override = %q, want $", s)
	}
	if _, err := c.resolveStyle("%s"); err == nil {
		t.Error("expected error for invalid placeholder style")
	}
	pg := &Connection{driver: "postgres"}
	if s, _ := pg.resolveStyle(""); s != "$" {
		t.Errorf("postgres default = %q, want $", s)
	}
}

func TestInsertAndMigrate(t *testing.T) {
	th := threadWith(t, libkite.AllowAllPermissions())
	c := openMem(t, th)
	defer c.db.Close()
	callMethod(t, th, c, "exec", starlark.String("CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, email TEXT)"))

	row := func(pairs ...string) *starlark.Dict {
		d := starlark.NewDict(len(pairs) / 2)
		for i := 0; i < len(pairs); i += 2 {
			d.SetKey(starlark.String(pairs[i]), starlark.String(pairs[i+1]))
		}
		return d
	}

	t.Run("insert single", func(t *testing.T) {
		res := callMethod(t, th, c, "insert", starlark.String("users"), row("name", "alice", "email", "a@x"))
		if structAttr(t, res, "rows_affected") != starlark.MakeInt(1) {
			t.Error("rows_affected != 1")
		}
		if structAttr(t, res, "last_insert_id") != starlark.MakeInt(1) {
			t.Error("last_insert_id != 1")
		}
	})

	t.Run("insert batch", func(t *testing.T) {
		res := callMethod(t, th, c, "insert", starlark.String("users"), starlark.NewList([]starlark.Value{
			row("name", "bob", "email", "b@x"),
			row("name", "carol", "email", "c@x"),
		}))
		if structAttr(t, res, "rows_affected") != starlark.MakeInt(2) {
			t.Error("batch rows_affected != 2")
		}
		v := callMethod(t, th, c, "query_value", starlark.String("SELECT count(*) FROM users"))
		if v != starlark.MakeInt(3) {
			t.Errorf("total users = %v, want 3", v)
		}
	})

	m := New()
	m.Load(&libkite.ModuleConfig{})
	mig := func(name, ddl string) starlark.Value {
		v, _ := m.stmt(th, nil, starlark.Tuple{starlark.String(ddl)}, []starlark.Tuple{{starlark.String("name"), starlark.String(name)}})
		return v
	}

	t.Run("migrate applies then skips", func(t *testing.T) {
		list := starlark.NewList([]starlark.Value{
			mig("001_widgets", "CREATE TABLE widgets (id INTEGER PRIMARY KEY)"),
			mig("002_color", "ALTER TABLE widgets ADD COLUMN color TEXT"),
		})
		res := callMethod(t, th, c, "migrate", list)
		applied := structAttr(t, res, "applied").(*starlark.List)
		if applied.Len() != 2 {
			t.Fatalf("first migrate applied %d, want 2", applied.Len())
		}
		// re-run: both skipped
		res2 := callMethod(t, th, c, "migrate", list)
		skipped := structAttr(t, res2, "skipped").(*starlark.List)
		appl2 := structAttr(t, res2, "applied").(*starlark.List)
		if skipped.Len() != 2 || appl2.Len() != 0 {
			t.Errorf("re-run: applied=%d skipped=%d, want 0/2", appl2.Len(), skipped.Len())
		}
	})

	t.Run("migrate requires names", func(t *testing.T) {
		attr, _ := c.Attr("migrate")
		unnamed, _ := m.stmt(th, nil, starlark.Tuple{starlark.String("CREATE TABLE z (id INT)")}, nil)
		if _, err := starlark.Call(th, attr.(*starlark.Builtin), starlark.Tuple{starlark.NewList([]starlark.Value{unnamed})}, nil); err == nil {
			t.Error("expected error for an unnamed migration")
		}
	})
}

func TestSqliteFileDSN(t *testing.T) {
	got := sqliteFileDSN("app.db")
	if !strings.Contains(got, "busy_timeout") || !strings.Contains(got, "WAL") {
		t.Errorf("sqliteFileDSN missing pragmas: %q", got)
	}
	if again := sqliteFileDSN(got); again != got {
		t.Errorf("sqliteFileDSN not idempotent: %q", again)
	}
}
