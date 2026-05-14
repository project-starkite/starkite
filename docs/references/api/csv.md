---
title: "csv"
description: "CSV reading, writing, and file I/O"
weight: 7
---

The `csv` module provides CSV file reading and writing with configurable delimiters, comment characters, and header support.

## Factory Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `csv.file(path)` | `csv.file` | Create a file object for reading CSV |
| `csv.source(data)` | `csv.writer` | Create a writer object from data |

## csv.file

Read and parse CSV files.

```python
f = csv.file("data.csv")
```

### Methods and Properties

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `f.read(sep=",", comment="", skip=0, header=False)` | `list` | Read and parse the CSV file |
| `f.try_read(sep=",", comment="", skip=0, header=False)` | `Result` | Read the file, returning a Result |
| `f.path` | `string` | The file path |

### read() Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `sep` | `string` | `","` | Field separator character |
| `comment` | `string` | `""` | Comment line prefix (lines starting with this are skipped) |
| `skip` | `int` | `0` | Number of leading rows to skip |
| `header` | `bool` | `False` | If `True`, first row is treated as headers and rows are returned as dicts |

When `header=False`, `read()` returns a list of lists (each row is a list of strings). When `header=True`, `read()` returns a list of dicts keyed by header names.

## csv.writer

Write data to CSV files.

```python
w = csv.source([
    ["name", "age", "city"],
    ["Alice", "30", "NYC"],
    ["Bob", "25", "LA"],
])
```

### Methods and Properties

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `w.write_file(path, sep=",", headers=None)` | `None` | Write CSV to a file |
| `w.try_write_file(path, sep=",", headers=None)` | `Result` | Write CSV to a file, returning a Result |
| `w.data` | `any` | The underlying data |

### write_file() Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `path` | `string` | required | Output file path |
| `sep` | `string` | `","` | Field separator character |
| `headers` | `list[string]` | `None` | Column headers to write as the first row |

## Examples

### Reading CSV Files

```python
# Basic reading — returns list of lists
rows = csv.file("data.csv").read()
for row in rows:
    print(row[0], row[1])

# With headers — returns list of dicts
records = csv.file("users.csv").read(header=True)
for r in records:
    print(r["name"], r["email"])

# TSV file with comments
rows = csv.file("data.tsv").read(sep="\t", comment="#", skip=1)
```

### Writing CSV Files

```python
# Write from a list of lists
data = [
    ["Alice", "30", "NYC"],
    ["Bob", "25", "LA"],
    ["Carol", "35", "Chicago"],
]
csv.source(data).write_file("people.csv", headers=["name", "age", "city"])

# Write from a list of dicts
records = [
    {"host": "web-1", "status": "healthy", "uptime": "45d"},
    {"host": "web-2", "status": "degraded", "uptime": "12d"},
]
csv.source(records).write_file("report.csv", headers=["host", "status", "uptime"])
```

### Error Handling

```python
result = csv.file("maybe-missing.csv").try_read(header=True)
if result.ok:
    for record in result.value:
        print(record)
else:
    print("Could not read CSV:", result.error)
```

### Processing Pipeline

```python
# Read a CSV, filter, and write results
rows = csv.file("servers.csv").read(header=True)
unhealthy = [r for r in rows if r["status"] != "healthy"]

if unhealthy:
    csv.source(unhealthy).write_file(
        "unhealthy-servers.csv",
        headers=["host", "status", "last_check"],
    )
    print(len(unhealthy), "unhealthy servers written to report")
```

> **Note:**
All `csv` functions and methods that can fail support `try_` variants. For example, `csv.try_file(path)`, `f.try_read()`, and `w.try_write_file(path)` return a `Result` instead of raising an error.

