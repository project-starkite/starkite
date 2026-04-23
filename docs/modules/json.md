---
title: "json"
description: "JSON encoding, decoding, and file I/O"
weight: 5
---

The `json` module provides JSON encoding and decoding, both for strings and files.

## Module-Level Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `json.encode(value)` | `string` | Encode a value to a JSON string |
| `json.decode(string)` | `any` | Decode a JSON string to a value |

## Factory Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `json.file(path)` | `json.file` | Create a file object for reading JSON |
| `json.source(data)` | `json.writer` | Create a writer object from data |

## json.file

Read and query JSON files.

```python
f = json.file("config.json")
```

### Methods and Properties

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `f.decode()` | `any` | Decode the entire file |
| `f.try_decode()` | `Result` | Decode the file, returning a Result |
| `f.path` | `string` | The file path |

The `path` property on `json.file` refers to the filesystem path of the file, not a JSON path query.

## json.writer

Encode data to JSON strings or write to files.

```python
w = json.source({"name": "kite", "version": "1.0"})
```

### Methods and Properties

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `w.encode(indent="", prefix="")` | `string` | Encode data to JSON string |
| `w.write_file(path, indent="", prefix="")` | `None` | Write JSON to a file |
| `w.try_write_file(path, indent="", prefix="")` | `Result` | Write JSON to a file, returning a Result |
| `w.data` | `any` | The underlying data |

## Examples

### Encoding and Decoding

```python
# Encode a dict to JSON
text = json.encode({"host": "localhost", "port": 8080})
print(text)  # {"host":"localhost","port":8080}

# Decode a JSON string
data = json.decode('{"host":"localhost","port":8080}')
print(data["host"])  # localhost
```

### Reading JSON Files

```python
# Read and decode a JSON file
f = json.file("package.json")
pkg = f.decode()
print(pkg["name"], pkg["version"])

# With error handling
result = json.file("config.json").try_decode()
if result.ok:
    config = result.value
else:
    print("Failed to read config:", result.error)
```

### Writing JSON Files

```python
config = {
    "database": {"host": "db.example.com", "port": 5432},
    "cache": {"host": "redis.example.com", "port": 6379},
}

# Write with indentation
w = json.source(config)
w.write_file("config.json", indent="  ")

# Get as string
text = w.encode(indent="  ")
print(text)
```

### Round-Trip Editing

```python
# Read, modify, write
data = json.file("settings.json").decode()
data["debug"] = False
data["log_level"] = "warn"
json.source(data).write_file("settings.json", indent="  ")
```

> **Note:**
All `json` functions and methods that can fail support `try_` variants. For example, `json.try_decode(s)`, `json.try_file(path)`, `f.try_decode()`, and `w.try_write_file(path)` return a `Result` instead of raising an error.

