---
title: "Databases (SQL)"
description: "Persisting and querying data with the sql module and embedded SQLite"
weight: 45
---

# Databases (SQL)

When a script needs to remember something between runs — a list of processed items, a set of seeded fixtures, the state of a long migration — it reaches for the `sql` module. The module gives a script durable, queryable storage without standing up infrastructure: **SQLite is built into every edition**, embedded and serverless, so a script that opens a file is a script with a database. PostgreSQL and MySQL ship in the all-in-one `kite` binary and present the same API, so code written against SQLite carries over to a networked database with only the connection string changing.

Under the hood the module is two layers over Go's `database/sql`. The lower layer is the raw machinery — `open`, `query`, `exec`, `begin` — a thin, faithful wrapping you can drop to when you want full control. The upper layer adds the conveniences a script actually wants: a managed `tx()` that commits and rolls back for you, scalar and column readers, streaming, and migrations. You will spend almost all of your time in the upper layer; the lower one is there when you need it.

Because SQLite is file I/O, these examples touch the filesystem, so run them with at least `--allow-fs`:

```bash
kite run ./store.star --allow-fs
```

## Open a database

Every interaction starts from a connection, and you obtain one with `sql.open(driver, dsn)`. This call is also the security seam: permission is checked here, per driver, so a SQLite open clears under `allow-fs` while a Postgres or MySQL open needs the network grant of `allow-net`. For SQLite the DSN is a file path, or `:memory:` for an ephemeral database that exists only for the life of the run:

```python
db = sql.open("sqlite", "app.db")     # a file on disk
db = sql.open("sqlite", ":memory:")   # in-memory, gone when the script ends
```

The connection that comes back closes itself when the script ends, so an explicit `db.close()` is optional — keep it only when you want to release the pool early.

## Create a table and insert rows

With a connection in hand, you change data and schema through `exec`, which runs any statement that does not return rows. The one rule to internalize: pass values as parameters with `?` placeholders, never by formatting them into the SQL string, so each value is bound safely and an apostrophe in a title can never become an injection. After an `exec`, the result tells you what happened:

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

The `INSERT` reports `last_insert_id` — the generated key for the new row — and `rows_affected`, the count of rows the statement touched. Use the former to thread a fresh row's id into the next statement, the latter to confirm an update or delete hit what you expected.

## Query

Reading is where the upper layer earns its keep, because the shape you want back is rarely "all rows, all columns." Start with the general case: `query` returns a list of rows, each a dict keyed by column name, which you iterate directly:

```python
notes = db.query("SELECT id, title FROM notes WHERE done = ?", False)
for n in notes:
    printf("[%d] %s\n", n["id"], n["title"])
```

Often you want less than a full table of dicts, and reaching for `query` and then indexing into it is clumsy. For a single row, `query_row` returns one dict or `None`; for a single value, `query_value` returns a bare scalar; and for one column across many rows, `query_column` returns a flat list:

```python
note  = db.query_row("SELECT * FROM notes WHERE id = ?", 1)   # dict or None
count = db.query_value("SELECT count(*) FROM notes")          # 1
titles = db.query_column("SELECT title FROM notes")           # ["write the docs"]
```

Each of these reads its whole result into memory before returning. That is fine until the result is large, at which point materializing every row is wasteful or impossible. For that case `query_each` streams: it calls a function once per row and never holds the full set:

```python
db.query_each("SELECT * FROM events", lambda r: process(r))
```

The trade-off is the usual one — `query_each` keeps memory flat for a million rows, but you process row by row rather than getting a list back to slice and reuse.

## Transactions

When several writes have to land together or not at all — debit one account, credit another — you need a transaction. The upper layer's `db.tx()` makes the common case safe by default: you hand it a function, it hands that function a transaction, and it commits when the function returns cleanly and rolls back if the function raises. There is no manual commit or rollback to forget:

```python
def complete_all(tx):
    tx.exec("UPDATE notes SET done = 1 WHERE done = 0")
    tx.exec("INSERT INTO notes (title, done) VALUES (?, 1)", "archive sweep")

db.tx(complete_all)
```

Sometimes the unit of work is not a function but a fixed, known list of independent statements. For that, `db.batch()` runs them atomically — all commit or all roll back — and you build each statement with `sql.stmt()`:

```python
db.batch([
    sql.stmt("INSERT INTO notes (title) VALUES (?)", "first"),
    sql.stmt("INSERT INTO notes (title) VALUES (?)", "second"),
])
```

Reach for `tx()` when the steps depend on each other or on intermediate reads, and for `batch()` when you simply have a list to apply as one unit.

## Insert from objects

Writing `INSERT` SQL by hand is tedious when the data is already a dict, so `insert` builds the statement for you, taking the column list straight from the data's keys. Pass one dict for a row, or a list of dicts to write a batch in a single statement:

```python
db.insert("notes", {"title": "write docs", "done": False})
db.insert("notes", [{"title": "first"}, {"title": "second"}])   # a batch in one statement
```

Every dict in a batch must carry the same columns, since `insert` derives the statement once from the first row and binds the rest to it.

## Migrations

A setup script that creates tables should be safe to re-run, but a bare `CREATE TABLE` fails the second time and a guarded one silently skips real schema changes. `migrate` solves this properly: it applies a list of named statements once each and records which have already run, so re-running the script applies only what is new:

```python
db.migrate([
    sql.stmt("CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT)", name="001_notes"),
    sql.stmt("ALTER TABLE notes ADD COLUMN done BOOLEAN DEFAULT 0", name="002_done"),
])
```

The `name` on each statement is its identity in that ledger — it is how `migrate` knows a step has run and skips it next time — so every migration needs one, and the names must stay stable once they have been applied.

## A small end-to-end store

Putting the pieces together, a self-contained store is a connection, a schema, a handful of functions over `exec` and `query`, and a `main()` that drives them:

```python
# store.star — run: kite run ./store.star --allow-fs
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

Running this creates `notes.db` in the working directory, adds two notes, marks one done, and prints the one that remains open — and because the file persists, a second run picks up where the first left off.

## Notes

A few behaviors are worth keeping in mind, because they follow from the module staying faithful to the underlying driver rather than papering over it:

- **Placeholders are driver-native.** SQLite and MySQL use `?`; PostgreSQL uses `$1`. The SQL is not rewritten — write it for the database you opened, or a mismatch surfaces as an error.
- **SQLite booleans** have no native type: a `True` stored is read back as the integer `1`. PostgreSQL and MySQL with a real `BOOLEAN` column return `bool`.
- **Concurrent writers** to a file database are handled for you by an automatic busy-timeout and WAL mode.

These are the conveniences of an OLTP module — short, transactional reads and writes against a live database — not a bulk analytics engine.

For the complete method list, type mapping, permissions, and the other drivers, see the [`sql` API reference](../references/api/sql.md).
