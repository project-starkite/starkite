package sql

import (
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

func TestConnectionCRUD(t *testing.T) {
	th := threadWith(t, libkite.AllowAllPermissions())
	c := openMem(t, th)
	defer c.db.Close()

	callMethod(t, th, c, "exec", starlark.String(
		"CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, active BOOLEAN)"))

	res := callMethod(t, th, c, "exec",
		starlark.String("INSERT INTO users (name, active) VALUES (?, ?)"),
		starlark.String("alice"), starlark.Bool(true))
	d := res.(*starlark.Dict)
	if id, _, _ := d.Get(starlark.String("last_insert_id")); id.(starlark.Int) != starlark.MakeInt(1) {
		t.Errorf("last_insert_id = %v, want 1", id)
	}
	if n, _, _ := d.Get(starlark.String("rows_affected")); n.(starlark.Int) != starlark.MakeInt(1) {
		t.Errorf("rows_affected = %v, want 1", n)
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
