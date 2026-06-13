---
title: "Files"
description: "Filesystem operations through the Path object"
weight: 40
---

# Files

Almost every automation eventually touches the filesystem — reading a config, writing a report, walking a directory tree. Starkite gives you one object for all of it. The `fs` module is **Path-first**: instead of a grab-bag of free functions that each take a string, you build a `Path` and call methods on it, the way Python's `pathlib` works. You create a path with `fs.path()`, or with the `path()` global alias that the module injects so you never need a prefix for the common case:

```python
p = fs.path("/etc/hosts")
p = path("/etc/hosts")        # equivalent global alias
```

Both calls produce the same `Path` object. Everything that follows hangs off it.

## Path properties and building

Before you read or write anything, you usually need to inspect a path or derive a new one from it, and a `Path` answers both needs without string surgery. Its components are exposed as properties, and the `/` operator composes new paths by joining segments — again mirroring `pathlib`, so the path is always a real object rather than a string you splice together:

```python
p = path("/home/alice/report.md")
p.name       # "report.md"
p.stem       # "report"
p.suffix     # ".md"
p.parent     # Path("/home/alice")

config = path("/home/alice") / "config" / "app.yaml"
print(config.string)   # /home/alice/config/app.yaml
```

The properties read off the parts you care about — `name`, `stem`, `suffix`, `parent` — while `/` builds `config` up segment by segment. Note `.string` at the end: a `Path` is an object, so when you want the plain text form for printing or passing onward, you ask for it explicitly.

## Reading and writing

Once you have a path, moving data in and out of it is a single method call. `read_text()` pulls the whole file into a string, `write_text()` replaces its contents, and predicate methods like `exists()` let you branch before you act:

```python
# Read
text = path("config.yaml").read_text()

# Write
path("/tmp/out.txt").write_text("hello")

# Existence and type checks
if path("/etc/hosts").exists():
    ...
```

These calls are the ones gated by the [permission](../fundamentals/security/permission.md) ladder. A read needs `fs.read`, and a write or delete needs `fs.write` / `fs.delete` — and under the default `deny-all` profile a script gets neither, so a script that touches the filesystem must run under `allow-fs` or higher. That gate is the cost of safety: the write above will not reach disk until you grant the authority for it.

## Listing and globbing

Reading one file at a time only takes you so far; most jobs operate over a set of files you discover at runtime. `glob()` on a directory path matches a pattern and yields each result as its own `Path`, so you can iterate the matches and keep calling `Path` methods on them without re-wrapping strings:

```python
for entry in path("./logs").glob("*.log"):
    print(entry.name)
```

Each `entry` here is a full `Path`, which is why `entry.name` works directly inside the loop — the same object model carries through from the directory to every file it contains.

## Error handling

A missing file or a permission denial normally raises and stops the script, which is the right default when you cannot continue without the data. When you can — a probe that may legitimately fail, an optional file — every I/O method offers a `try_` variant that returns a [`Result`](../fundamentals/language.md#error-handling) instead of raising, handing you control over the failure:

```python
result = path("/etc/missing").try_read_text()
if result.ok:
    print(result.value)
else:
    print("error:", result.error)
```

You inspect `result.ok` to see whether the read succeeded, then reach for `result.value` on success or `result.error` on failure. The trade-off is explicitness: `try_read_text()` never interrupts your script, but you carry the obligation to check the result yourself.

See the [fs API reference](../references/api/fs.md) for the complete `Path` surface — path building, metadata, directory operations, and globbing.
