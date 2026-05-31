---
title: "Files"
description: "Filesystem operations through the Path object"
weight: 40
---

# Files

The `fs` module provides filesystem operations through the `Path` object. Create a path with `fs.path()` or the `path()` global alias:

```python
p = fs.path("/etc/hosts")
p = path("/etc/hosts")        # equivalent global alias
```

## Path properties and building

A `Path` exposes its components and composes new paths with the `/` operator, like Python's `pathlib`:

```python
p = path("/home/alice/report.md")
p.name       # "report.md"
p.stem       # "report"
p.suffix     # ".md"
p.parent     # Path("/home/alice")

config = path("/home/alice") / "config" / "app.yaml"
print(config.string)   # /home/alice/config/app.yaml
```

## Reading and writing

```python
# Read
text = path("config.yaml").read_text()

# Write
path("/tmp/out.txt").write_text("hello")

# Existence and type checks
if path("/etc/hosts").exists():
    ...
```

## Listing and globbing

```python
for entry in path("./logs").glob("*.log"):
    print(entry.name)
```

## Error handling

Path methods have `try_` variants returning a [`Result`](../fundamentals/language.md#error-handling):

```python
result = path("/etc/missing").try_read_text()
if result.ok:
    print(result.value)
else:
    print("error:", result.error)
```

See the [fs API reference](../references/api/fs.md) for the complete `Path` surface — path building, metadata, directory operations, and globbing.
