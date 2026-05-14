---
title: "zip"
description: "ZIP archive reading and writing"
weight: 9
---

The `zip` module provides reading and writing of ZIP archives.

## Factory

| Factory | Returns | Description |
|---------|---------|-------------|
| `zip.file(path)` | `zip.archive` | Open or create a ZIP archive at the given path |

## zip.archive

| Method | Returns | Description |
|--------|---------|-------------|
| `namelist(match="")` | `list` | List entry names in the archive; optionally filter with a glob `match` pattern |
| `read(name)` | `bytes` | Read a single entry by name |
| `read_all(match="", files=[])` | `dict` | Read multiple entries; filter by glob `match` or explicit `files` list. Returns `{name: bytes}` |
| `write(source, name="")` | `None` | Add a file to the archive. Uses `source` filename if `name` is omitted |
| `write_all(match="", files=[], base_dir="", level=-1)` | `None` | Add multiple files; filter by glob `match` or explicit `files` list. `base_dir` sets the root for relative paths. `level` sets compression level |

### try_ variants

| Method | Returns | Description |
|--------|---------|-------------|
| `try_namelist(match="")` | `Result` | Like `namelist`, returns `Result` instead of raising |
| `try_read(name)` | `Result` | Like `read`, returns `Result` instead of raising |
| `try_read_all(match="", files=[])` | `Result` | Like `read_all`, returns `Result` instead of raising |
| `try_write(source, name="")` | `Result` | Like `write`, returns `Result` instead of raising |
| `try_write_all(match="", files=[], base_dir="", level=-1)` | `Result` | Like `write_all`, returns `Result` instead of raising |

## Examples

### List archive contents

```python
archive = zip.file("release.zip")
for name in archive.namelist():
    print(name)

# Filter by pattern
configs = archive.namelist(match="*.yaml")
```

### Read a single file

```python
archive = zip.file("release.zip")
data = archive.read("config.yaml")
print(data)
```

### Read all matching files

```python
archive = zip.file("release.zip")
yamls = archive.read_all(match="*.yaml")
for name, content in yamls.items():
    print(name, "->", len(content), "bytes")
```

### Create a ZIP archive

```python
archive = zip.file("/tmp/backup.zip")
archive.write("app.conf")
archive.write("/var/log/app.log", name="logs/app.log")
```

### Bulk write with glob

```python
archive = zip.file("/tmp/configs.zip")
archive.write_all(match="configs/*.yaml", base_dir="configs/")
```

### Safe reading with try_

```python
archive = zip.file("data.zip")
result = archive.try_read("missing.txt")
if not result.ok:
    print("Not found:", result.error)
```

> **Note:**
All `zip.archive` methods that can fail support `try_` variants that return a `Result` instead of raising an error.

