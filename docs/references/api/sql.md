---
title: "sql"
description: "SQL database connectivity for SQLite, PostgreSQL, and MySQL"
weight: 8
---

The `sql` module provides database connectivity over Go's `database/sql`. It is a factory module: `sql.open()` returns a connection used for queries, executes, and transactions.

The API is uniform across drivers, but the SQL itself is driver-native — placeholders and grammar are not rewritten. SQLite uses `?`, PostgreSQL uses `$1`, MySQL uses `?`.

## Editions

| Driver | Availability |
|--------|--------------|
| `sqlite` | every edition (bundled in `libkite`) |
| `postgres` | the all-in-one `kite` binary |
| `mysql` | the all-in-one `kite` binary |

A driver not built into the running edition fails at `sql.open` with an error naming the driver.

## Opening a connection

```python
db = sql.open("sqlite", "app.db")
db = sql.open("sqlite", ":memory:")
db = sql.open("postgres", "postgres://user:pass@host:5432/mydb?sslmode=disable")
db = sql.open("mysql", "user:pass@tcp(host:3306)/mydb")

# pool tuning (all optional)
db = sql.open("postgres", var_str("DATABASE_URL"),
    max_open=25, max_idle=5, max_lifetime=300, max_idle_time=60)
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `driver` | `string` | required | `"sqlite"`, `"postgres"`, or `"mysql"` |
| `dsn` | `string` | required | Driver-native connection string |
| `max_open` | `int` | `25` | Max open connections (forced to `1` for `:memory:`) |
| `max_idle` | `int` | `5` | Max idle connections |
| `max_lifetime` | `int` | `300` | Max connection lifetime (seconds) |
| `max_idle_time` | `int` | `60` | Max idle time (seconds) |

A connection is **auto-closed at run end**; an explicit `db.close()` is optional. For a SQLite file database, `busy_timeout` and WAL are enabled automatically. For `:memory:`, the pool is pinned to one connection (each pooled connection is otherwise a separate in-memory database).

## Reading

```python
rows = db.query("SELECT id, name FROM users WHERE active = ?", True)   # list of dicts
row  = db.query_row("SELECT * FROM users WHERE id = ?", 42)            # dict or None
n    = db.query_value("SELECT count(*) FROM users")                    # scalar or None
ids  = db.query_column("SELECT id FROM users ORDER BY id")             # flat list

db.query_each("SELECT * FROM events", lambda r: process(r))            # streaming, no full materialization
```

| Method | Returns |
|--------|---------|
| `query(sql, *params)` | list of `{column: value}` dicts |
| `query_row(sql, *params)` | one dict, or `None` |
| `query_value(sql, *params)` | first column of the first row, or `None` |
| `query_column(sql, *params)` | flat list of the first column |
| `query_each(sql, fn, *params)` | `None`; calls `fn(row_dict)` per row without materializing the full result |

Rows are dicts (column names are dynamic). Use `query_each` for large result sets to avoid loading every row into memory.

## Writing

```python
res = db.exec("INSERT INTO users (name) VALUES (?)", "alice")
res.rows_affected    # → 1
res.last_insert_id   # → 1  (None when the driver does not report it, e.g. postgres)

db.exec_many("INSERT INTO users (name, email) VALUES (?, ?)",
    [["alice", "a@x"], ["bob", "b@x"]])    # bulk, atomic
```

`exec` returns a result with `rows_affected` and `last_insert_id` (the latter is `None` when the driver does not report a generated id — PostgreSQL uses `INSERT … RETURNING`). `exec_many` runs the same statement over many parameter sets in one transaction and returns the total `rows_affected`.

## Transactions

### Managed callback — `tx`

The recommended form. The callback receives a transaction; the module commits on a clean return and rolls back on error. It returns the callback's value.

```python
def place_order(tx):
    tx.exec("INSERT INTO orders (user_id, total) VALUES (?, ?)", 1, 99.99)
    oid = tx.query_value("SELECT last_insert_rowid()")
    tx.exec("INSERT INTO order_items (order_id, qty) VALUES (?, ?)", oid, 3)

db.tx(place_order)                 # commit on success; rollback + raise on error
db.tx(place_order, retry=3)        # re-run on a serialization failure / deadlock
res = db.try_tx(place_order)       # recoverable: returns a Result instead of raising
```

A transaction exposes `query`, `query_row`, `query_value`, `query_column`, `exec`, `commit`, and `rollback`.

### Declarative batch — `batch`

Run a fixed set of independent statements atomically. Build statements with `sql.stmt`.

```python
res = db.batch([
    sql.stmt("UPDATE accounts SET bal = bal - ? WHERE id = ?", 100, 1, name="debit"),
    sql.stmt("UPDATE accounts SET bal = bal + ? WHERE id = ?", 100, 2, name="credit"),
])
res["debit"].rows_affected         # named statements → result dict keyed by name
```

- `sql.stmt(sql, *params, name="…")` — a statement value; `name` is optional.
- Named statements return a **dict keyed by name**; unnamed statements return a **positional list**; mixing named and unnamed is an error.
- On failure, the whole batch rolls back and the error names the failed statement.
- `try_batch` returns a `Result`.
- Under `--dry-run`, `batch` prints the statements it would run without executing them.

### Low-level — `begin`

```python
tx = db.begin()
tx.exec("UPDATE accounts SET bal = bal - ? WHERE id = ?", 100, 1)
tx.commit()                        # or tx.rollback()
```

`begin` is the manual escape hatch; prefer `tx`/`batch`.

## Connection lifecycle

```python
db.ping()    # raises if the connection is dead
db.close()   # optional — connections auto-close at run end
db.stats()   # pool stats: .open .in_use .idle .max_open .wait_count
db.driver    # → "sqlite"
```

## Type mapping

| SQL type | Starlark | Notes |
|----------|----------|-------|
| INTEGER, BIGINT | `int` | |
| FLOAT, DOUBLE | `float` | |
| DECIMAL, NUMERIC | `string` | preserves precision; not a float |
| VARCHAR, TEXT, CHAR | `string` | |
| BOOLEAN | `bool` | see SQLite note below |
| NULL | `None` | a `None` parameter binds SQL NULL |
| TIMESTAMP, DATETIME | `string` (RFC 3339) | parse with `time.parse` |
| BLOB, BYTEA | `string` | |

**SQLite type fidelity:** SQLite has no native boolean — a `True` parameter is stored as `1` and read back as the integer `1`, not `True`. PostgreSQL and MySQL with a real `BOOLEAN` column return `bool`. This is driver-native behavior.

**Duplicate column names** in a result row collapse to the last value (a row is a dict). Alias columns to distinguish them.

## Permissions

`sql.open` is gated per driver in the [permission ladder](../../fundamentals/security/permission.md):

| Profile | Grants |
|---------|--------|
| `allow-fs` | `sql.open(sqlite:**)` — SQLite is file I/O |
| `allow-net` | adds `sql.open(postgres:**)`, `sql.open(mysql:**)` — networked databases |

Connection methods inherit the capability from `open`. DSN passwords are redacted in permission patterns and error messages. Under `--sandbox=strict`, network drivers are blocked by network isolation while a SQLite database in `$CWD` works.

## Error handling

Every data method has a `try_` variant returning a [`Result`](../../fundamentals/language.md) (`.ok` / `.value` / `.error`) instead of raising:

```python
res = db.try_query("SELECT * FROM maybe_missing")
if not res.ok:
    log.warn("query failed", attrs={"error": res.error})
```

A placeholder-dialect mismatch (e.g. `?` against PostgreSQL) produces a driver-aware hint.

## Example — data-backed HTTP API

```python
db = sql.open("sqlite", "users.db")
db.exec("""
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT NOT NULL,
        email TEXT UNIQUE
    )
""")

def create_user(req):
    body = json.decode(req.body)
    res = db.exec("INSERT INTO users (name, email) VALUES (?, ?)", body["name"], body["email"])
    user = db.query_row("SELECT * FROM users WHERE id = ?", res.last_insert_id)
    return {"status": 201, "body": json.encode(user)}

def list_users(req):
    return {"users": db.query("SELECT * FROM users")}

srv = http.server()
srv.handle("POST /api/users", create_user)
srv.handle("GET /api/users", list_users)
srv.serve(port=8080)
```

## Related

- [Modules](../../fundamentals/modules.md) — installing and loading modules
- [Permission](../../fundamentals/security/permission.md) — the profile ladder
