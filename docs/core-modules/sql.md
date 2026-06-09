---
title: "Databases (SQL)"
description: "Persisting and querying data with the sql module and embedded SQLite"
weight: 45
---

# Databases (SQL)

The `sql` module connects to SQL databases. **SQLite is built into every edition** — it is embedded and needs no server. PostgreSQL and MySQL ship in the all-in-one `kite` binary and use the same API.

These examples touch the filesystem (SQLite is file I/O), so run them with at least `--allow-fs`:

```bash
kite run store.star --allow-fs
```

## Open a database

`sql.open(driver, dsn)` returns a connection. For SQLite the DSN is a file path, or `:memory:` for an ephemeral database:

```python
db = sql.open("sqlite", "app.db")     # a file on disk
db = sql.open("sqlite", ":memory:")   # in-memory, gone when the script ends
```

The connection closes automatically when the script ends — an explicit `db.close()` is optional.

## Create a table and insert rows

`exec` runs a statement that changes data or schema. Pass parameters with `?` placeholders — never string formatting — so values are bound safely:

```python
db.exec("""
    CREATE TABLE IF NOT EXISTS notes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        done BOOLEAN DEFAULT 0
    )
""")

res = db.exec("INSERT INTO notes (title) VALUES (?)", "write the docs")
print(res.last_insert_id)   # 1
print(res.rows_affected)    # 1
```

## Query

`query` returns a list of rows, each a dict keyed by column name:

```python
notes = db.query("SELECT id, title FROM notes WHERE done = ?", False)
for n in notes:
    printf("[%d] %s\n", n["id"], n["title"])
```

For a single row, `query_row` returns one dict or `None`; for a single value, `query_value` returns a scalar:

```python
note  = db.query_row("SELECT * FROM notes WHERE id = ?", 1)   # dict or None
count = db.query_value("SELECT count(*) FROM notes")          # 1
titles = db.query_column("SELECT title FROM notes")           # ["write the docs"]
```

## Transactions

Wrap related changes in `db.tx()`. The function you pass receives a transaction; it commits when the function returns and rolls back if the function raises — no manual commit/rollback needed:

```python
def complete_all(tx):
    tx.exec("UPDATE notes SET done = 1 WHERE done = 0")
    tx.exec("INSERT INTO notes (title, done) VALUES (?, 1)", "archive sweep")

db.tx(complete_all)
```

To run a fixed set of independent statements atomically, use `db.batch()` with `sql.stmt()`:

```python
db.batch([
    sql.stmt("INSERT INTO notes (title) VALUES (?)", "first"),
    sql.stmt("INSERT INTO notes (title) VALUES (?)", "second"),
])
```

## A small end-to-end store

```python
# store.star — run: kite run store.star --allow-fs
db = sql.open("sqlite", "notes.db")
db.exec("CREATE TABLE IF NOT EXISTS notes (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT, done BOOLEAN DEFAULT 0)")

def add(title):
    return db.exec("INSERT INTO notes (title) VALUES (?)", title).last_insert_id

def finish(id):
    db.exec("UPDATE notes SET done = 1 WHERE id = ?", id)

def main():
    finish(add("draft proposal"))
    add("review PR")
    open_notes = db.query("SELECT id, title FROM notes WHERE done = 0")
    printf("%d open note(s)\n", len(open_notes))
    for n in open_notes:
        printf("  - %s\n", n["title"])
```

## Notes

- **Placeholders are driver-native.** SQLite and MySQL use `?`; PostgreSQL uses `$1`. The SQL is not rewritten — write it for the database you opened.
- **SQLite booleans** have no native type: a `True` stored is read back as the integer `1`. PostgreSQL/MySQL with a real `BOOLEAN` return `bool`.
- **Concurrent writers** to a file database are handled by an automatic busy-timeout and WAL mode.

For the complete method list, type mapping, permissions, and the other drivers, see the [`sql` API reference](../references/api/sql.md).
