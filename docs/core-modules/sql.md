---
title: "Databases (SQL)"
description: "Persist and query relational data using the sql module and embedded SQLite"
weight: 45
---

# Databases (SQL)

The `sql` module provides persistent, queryable data storage for Starkite scripts. When a script needs to maintain state between executions—such as tracking processed items, seeding database fixtures, or managing schema migrations—it uses this module to read and write relational data. SQLite is embedded directly into the Starkite runtime, enabling serverless database operations against a local file. The module also connects to external PostgreSQL and MySQL databases using the same API.

The module provides two levels of control: a low-level interface for direct SQL execution, and a high-level interface that automates common scripting tasks such as managing transactions, streaming large datasets, and running idempotent schema migrations.

Because database operations access the host system, Starkite enforces permission checks. Interacting with local SQLite files requires the `--allow-fs` permission, while connecting to external PostgreSQL or MySQL servers requires `--allow-net`:

```bash
kite run ./store.star --allow-fs
```

## Connecting to a database

Initialize a database connection using `sql.open(driver, dsn)`. The driver name and connection string (DSN) format determine the target database and the required permission profile:

| Driver | DSN Format / Example | Required Permission | Description |
| :--- | :--- | :--- | :--- |
| `sqlite` | `app.db` or `:memory:` | `--allow-fs` | Local file-based database or in-memory ephemeral database. |
| `postgres` | `postgres://user:pass@host:port/db?sslmode=disable` | `--allow-net` | External PostgreSQL server connection string. |
| `mysql` | `user:pass@tcp(host:port)/db` | `--allow-net` | External MySQL or MariaDB server connection string. |

```python
# Open a persistent SQLite database file
db = sql.open("sqlite", "app.db")

# Open an in-memory SQLite database
db = sql.open("sqlite", ":memory:")
```

Database connections close automatically when the script finishes execution. Calling `db.close()` is optional and is only necessary to release connection pool resources early.

## Executing write operations

To modify database schema or data, use `db.exec(query, *args)`. This method executes statements that do not return rows, such as schema modifications or data insertions.

Pass query parameters as arguments using driver-native placeholders to prevent SQL injection:

```python
# Create a table
db.exec("""
    CREATE TABLE IF NOT EXISTS notes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT NOT NULL,
        done BOOLEAN DEFAULT 0
    )
""")

# Insert a row with bound parameters
res = db.exec("INSERT INTO notes (title) VALUES (?)", "Write documentation")
```

The `db.exec()` method returns a result object containing two properties:
* `last_insert_id`: The auto-generated primary key ID of the inserted row (typically supported by SQLite and MySQL).
* `rows_affected`: The number of rows modified by the statement.

## Inserting structured data

To insert dictionaries or lists of dictionaries directly without writing manual `INSERT` statements, use `db.insert(table, data)`. The module automatically generates the column names and placeholders from the keys of the dictionary.

To insert a single row, pass a dictionary:

```python
db.insert("notes", {"title": "Write documentation", "done": False})
```

To perform a bulk insertion in a single statement, pass a list of dictionaries:

```python
db.insert("notes", [
    {"title": "First task"},
    {"title": "Second task"},
])
```

All dictionaries in a bulk insertion list must contain the identical set of keys. The module generates the SQL structure using the keys of the first dictionary.

### Bulk execution with prepared statements

To execute the same SQL statement repeatedly with different parameters, use `db.exec_many(query, param_sets)`. This method prepares the statement once and runs it for each set of parameters within a single transaction, improving performance:

```python
# Parameter sets must be a list of lists or tuples
tasks = [
    ("Task A",),
    ("Task B",),
    ("Task C",),
]

res = db.exec_many("INSERT INTO notes (title) VALUES (?)", tasks)
```

The returned result contains the cumulative `rows_affected` across all executions. The `last_insert_id` is set to `None`.

## Querying data

The `sql` module provides several query methods tailored to the expected structure of the results.

### Querying multiple rows

Use `db.query(query, *args)` to retrieve multiple rows. This method returns a list of dictionaries, where each dictionary represents a row keyed by column names.

Because Starlark restricts control-flow statements like `for` loops to function bodies, wrap the iteration in a function:

```python
def print_open_notes():
    notes = db.query("SELECT id, title FROM notes WHERE done = ?", False)
    for n in notes:
        printf("[%d] %s\n", n["id"], n["title"])
```

### Targeted query methods

To retrieve specific result shapes without manual parsing, use these specialized query methods:

* **`db.query_row(query, *args)`**: Returns a single row as a dictionary, or `None` if no row matches.
* **`db.query_value(query, *args)`**: Returns a single scalar value from the first column of the first row.
* **`db.query_column(query, *args)`**: Returns a flat list of scalar values from the first column of all matching rows.

```python
# Retrieve a single row
note = db.query_row("SELECT * FROM notes WHERE id = ?", 1)

# Retrieve an aggregate scalar value
count = db.query_value("SELECT count(*) FROM notes")

# Retrieve a list of values for a single column
titles = db.query_column("SELECT title FROM notes")
```

### Streaming large datasets

Standard query methods load the entire result set into memory. For large datasets, use `db.query_each(query, callback, *args)` to stream rows. This method invokes the callback function for each row individually without loading the full set into memory:

```python
def process_row(row):
    print(row["title"])

db.query_each("SELECT * FROM notes", process_row)
```

## Transactions and batching

Transactions ensure that multiple database operations execute atomically. The `sql` module supports both callback-managed transactions and statement batching.

### Callback-managed transactions

Use `db.tx(callback, retry=0)` to execute multiple operations within a single transaction. Pass a function that accepts a transaction object (`tx`) as its argument.

The transaction commits automatically if the function returns cleanly, and rolls back if the function raises an error.

```python
def update_and_log(tx):
    tx.exec("UPDATE notes SET done = 1 WHERE done = 0")
    tx.exec("INSERT INTO notes (title, done) VALUES (?, 1)", "archive sweep")

db.tx(update_and_log)
```

To handle transient write contention (such as SQLite lock contention or database deadlocks), set the `retry` keyword argument to specify how many times the transaction should be retried before failing:

```python
# Retry up to 3 times on transient errors
db.tx(update_and_log, retry=3)
```

### Statement batching

For lists of independent statements that do not require intermediate reads, use `db.batch(statements)`. This method executes a list of statement objects, created using `sql.stmt(query, *args, name=None)`, inside a single transaction.

If all statements in the batch are named, `db.batch()` returns a dictionary keyed by statement names. If statements are unnamed, it returns a list of results in the order they were executed:

```python
# Naming statements returns a dictionary of results
results = db.batch([
    sql.stmt("INSERT INTO notes (title) VALUES (?)", "First task", name="task1"),
    sql.stmt("INSERT INTO notes (title) VALUES (?)", "Second task", name="task2"),
])

print(results["task1"].last_insert_id)
```

Use `db.tx()` when subsequent statements depend on intermediate query results. Use `db.batch()` to apply a list of static writes atomically.

## Schema migrations

To manage database schema changes safely, use `db.migrate(statements)`. This method tracks and applies schema updates incrementally. It records applied migrations in a `schema_migrations` tracking table to ensure each named migration step runs exactly once.

Define each migration step using `sql.stmt(query, *args, name)`:

```python
res = db.migrate([
    sql.stmt("CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT)", name="001_create_notes"),
    sql.stmt("ALTER TABLE notes ADD COLUMN done BOOLEAN DEFAULT 0", name="002_add_done_column"),
])

print("Applied migrations:", res.applied)
print("Skipped migrations:", res.skipped)
```

The `name` parameter uniquely identifies the migration step in the database ledger. Do not change migration names after they have been executed.

## Complete example

The following script demonstrates opening a database, initializing the schema, performing write operations, and querying the results:

```python
# store.star
# Run with: kite run ./store.star --allow-fs

db = sql.open("sqlite", "notes.db")

# Initialize the schema
db.exec("""
    CREATE TABLE IF NOT EXISTS notes (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        title TEXT,
        done BOOLEAN DEFAULT 0
    )
""")

def add_note(title):
    res = db.exec("INSERT INTO notes (title) VALUES (?)", title)
    return res.last_insert_id

def mark_done(note_id):
    db.exec("UPDATE notes SET done = 1 WHERE id = ?", note_id)

def main():
    # Insert notes and retrieve the ID
    proposal_id = add_note("Draft proposal")
    add_note("Review pull request")

    # Mark the first note as completed
    mark_done(proposal_id)

    # Query and print remaining active notes
    active_notes = db.query("SELECT id, title FROM notes WHERE done = 0")
    printf("%d active note(s):\n", len(active_notes))
    for note in active_notes:
        printf("  - [%d] %s\n", note["id"], note["title"])
```

Running this script creates a local SQLite database named `notes.db`, inserts two rows, updates one row, and outputs the remaining active row. The database file persists between runs.

## Driver-specific behaviors

Because the `sql` module wraps underlying database drivers directly, you must observe driver-specific behaviors:

* **Query placeholders**: Placeholders are driver-dependent and are not rewritten by Starkite. SQLite and MySQL use `?` placeholders, while PostgreSQL uses numbered `$1`, `$2` placeholders.
* **Boolean representation**: SQLite does not have a native boolean type. Boolean values stored in SQLite are returned as integers (`1` for true, `0` for false). PostgreSQL and MySQL return boolean values as Starlark `bool` values.
* **Concurrency in SQLite**: The embedded SQLite driver handles concurrent write operations using Write-Ahead Logging (WAL) and an automatic busy-timeout handler.

## Failure handling

By default, SQL syntax errors, connection failures, or constraint violations raise a Starlark-level execution error and halt the script. To inspect and handle errors programmatically, use the `try_` variants of the connection methods. These variants return a `Result` object containing `.ok`, `.value`, and `.error` attributes:

```python
def check_query():
    result = db.try_query("SELECT * FROM non_existent_table")

    if result.ok:
        rows = result.value
        print("Query succeeded")
    else:
        print("Query failed: " + result.error)
```

## See also

* [`sql` API reference](../references/api/sql.md) — Detailed function signatures and properties.
