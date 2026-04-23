---
title: "table"
description: "ASCII table rendering"
weight: 19
---

The `table` module creates formatted ASCII tables for terminal output.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `table.new(headers)` | `TableValue` | Create a new table with the given column headers |
| `table.print(tbl)` | `None` | Print a table to stdout |

## TableValue

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `add_row(*values)` | `None` | Add a row of values to the table |
| `render()` | `string` | Render the table as a string |
| `row_count` | `int` | Number of rows in the table |

## Examples

### Basic table

```python
tbl = table.new(["Name", "Status", "Replicas"])
tbl.add_row("frontend", "Running", 3)
tbl.add_row("backend", "Running", 2)
tbl.add_row("worker", "Pending", 0)
table.print(tbl)
```

Output:
```text
+-----------+---------+----------+
| Name      | Status  | Replicas |
+-----------+---------+----------+
| frontend  | Running | 3        |
| backend   | Running | 2        |
| worker    | Pending | 0        |
+-----------+---------+----------+
```

### Render to string

```python
tbl = table.new(["Key", "Value"])
tbl.add_row("host", os.hostname())
tbl.add_row("user", os.username())
tbl.add_row("pid", os.pid())

output = tbl.render()
write_text("/tmp/info.txt", output)
```

### Dynamic rows

```python
tbl = table.new(["File", "Size"])
for entry in glob("/etc/*.conf"):
    info = path(entry).stat()
    tbl.add_row(entry, info.size)

print("Found", tbl.row_count, "config files")
table.print(tbl)
```
