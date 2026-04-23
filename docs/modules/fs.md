---
title: "fs"
description: "Filesystem operations through the Path object"
weight: 2
---

## Overview

The `fs` module provides filesystem operations through the `Path` object. All functionality is auto-loaded — no imports are needed.

The primary API is the `Path` object, created with `fs.path()` or the `path()` global alias. A set of global alias functions provide convenient shorthand for common Path operations.

## Path Object

The `Path` object is the primary API for all filesystem operations. Create one with `fs.path()` or the `path()` global alias:

```python
p = fs.path("/etc/hosts")
p = path("/etc/hosts")       # equivalent, using global alias
```

### Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | `string` | Filename with extension (`"report.md"`) |
| `parent` | `Path` | Parent directory as a Path |
| `stem` | `string` | Filename without extension (`"report"`) |
| `suffix` | `string` | File extension (`".md"`) |
| `string` | `string` | Full path as string |
| `parts` | `tuple` | Path components as a tuple |

### Path Building

| Method | Returns | Description |
|--------|---------|-------------|
| `p.join(other)` | `Path` | Join with another path component |
| `p.with_name(name)` | `Path` | Replace the filename |
| `p.with_suffix(suffix)` | `Path` | Replace the extension |
| `p.resolve()` | `Path` | Resolve to absolute path |
| `p.clean()` | `Path` | Return cleaned path (resolves `.` and `..`) |
| `p.is_absolute()` | `bool` | Check if path is absolute |
| `p.is_relative_to(base)` | `bool` | Check if path is relative to base |
| `p.relative_to(base)` | `Path` | Get relative path from base |
| `p.match(pattern)` | `bool` | Test against glob pattern |
| `p.expanduser()` | `Path` | Expand `~` to home directory |

### Path Separator

The `/` operator joins path components, similar to Python's `pathlib`:

```python
p = path("/home/user")
config = p / "config" / "app.yaml"
print(config.string)  # /home/user/config/app.yaml
```

### File Info

| Method | Returns | Description |
|--------|---------|-------------|
| `p.exists()` | `bool` | Check if path exists on disk |
| `p.is_file()` | `bool` | Check if path is a regular file |
| `p.is_dir()` | `bool` | Check if path is a directory |
| `p.is_symlink()` | `bool` | Check if path is a symbolic link |
| `p.stat()` | `dict` | Get file metadata |
| `p.owner()` | `string` | Get file owner name |
| `p.group()` | `string` | Get file group name |

### File I/O

| Method | Returns | Description |
|--------|---------|-------------|
| `p.read_text()` | `string` | Read file as text |
| `p.read_bytes()` | `bytes` | Read file as bytes |
| `p.write_text(content)` | `None` | Write text to file |
| `p.write_bytes(data)` | `None` | Write bytes to file |
| `p.append_text(content)` | `None` | Append text to file |
| `p.append_bytes(data)` | `None` | Append bytes to file |

### File Operations

| Method | Returns | Description |
|--------|---------|-------------|
| `p.touch()` | `None` | Create file or update mtime |
| `p.mkdir()` | `None` | Create directory (and parents) |
| `p.remove()` | `None` | Remove file or directory |
| `p.rename(target)` | `Path` | Rename/move to target |
| `p.copy_to(target)` | `Path` | Copy file to target path |
| `p.move_to(target)` | `Path` | Move file/directory to target |
| `p.truncate(size)` | `None` | Truncate file to given size |
| `p.chmod(mode)` | `None` | Change permissions |
| `p.chown(uid=-1, gid=-1)` | `None` | Change file ownership |
| `p.symlink_to(target)` | `None` | Create symlink pointing to target |
| `p.readlink()` | `Path` | Read symlink target |
| `p.hardlink_to(target)` | `None` | Create hard link at this path pointing to target |

### Directory

| Method | Returns | Description |
|--------|---------|-------------|
| `p.listdir()` | `list[Path]` | List directory entries as Paths |
| `p.glob(pattern)` | `list[Path]` | Glob within this directory |
| `p.walk()` | `list[tuple]` | Recursive directory traversal (Path, dirs, files) |
| `p.disk_usage()` | `dict` | Disk space info with total, used, free |

### Examples

```python
p = path("/var/log/app")

# Navigate
parent = p.parent                  # Path(/var/log)
child = p / "errors" / "today.log" # Path(/var/log/app/errors/today.log)

# Query properties
print(child.name)    # today.log
print(child.stem)    # today
print(child.suffix)  # .log
print(child.parts)   # ("/", "var", "log", "app", "errors", "today.log")

# Modify path
renamed = child.with_name("yesterday.log")
different_ext = child.with_suffix(".txt")

# Read and iterate
if p.is_dir():
    for entry in p.listdir():
        if entry.is_file() and entry.match("*.log"):
            content = entry.read_text()
            print(entry.name, ":", len(content), "bytes")

# Write and append
output = path("/tmp/report.txt")
output.write_text("Summary\n")
output.append_text("Details\n")

# File metadata
info = path("data.bin").stat()
print("Size:", info["size"])

# Directory operations
tmp = path("/tmp/myapp/logs")
tmp.mkdir()
for entry in tmp.parent.listdir():
    print(entry.name)

# Glob for files
for f in path("src").glob("**/*.go"):
    print(f.string)
```

## Global Aliases

These top-level functions are convenient shorthand for common Path methods, available without any module prefix:

| Function | Returns | Description |
|----------|---------|-------------|
| `path(p)` | `Path` | Create a Path object |
| `read_text(p)` | `string` | Read file as text. Alias for `path(p).read_text()` |
| `read_bytes(p)` | `bytes` | Read file as bytes. Alias for `path(p).read_bytes()` |
| `write_text(p, content)` | `None` | Write text to file. Alias for `path(p).write_text(content)` |
| `write_bytes(p, data)` | `None` | Write bytes to file. Alias for `path(p).write_bytes(data)` |
| `exists(p)` | `bool` | Check if path exists. Alias for `path(p).exists()` |
| `glob(pattern)` | `list[string]` | Find paths matching a glob pattern |

```python
# Quick file I/O with global aliases
config = read_text("/etc/myapp/config.yaml")
write_text("/tmp/output.txt", "Hello, World!\n")

if exists("/tmp/output.txt"):
    print("File written successfully")

for f in glob("src/**/*.go"):
    print(f)
```

## The try_ Pattern

All I/O methods on `Path` support `try_` variants that return a `Result` instead of raising an error:

```python
result = path("/etc/missing").try_read_text()
if result.ok:
    print(result.value)
else:
    print("Error:", result.error)
```
