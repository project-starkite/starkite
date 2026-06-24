---
title: "Files"
description: "Filesystem operations through the Path object"
weight: 40
---

# Files

Starkite scripts use the `fs` module to manage files, directories, and path resolutions. The module centers on the `Path` type, allowing scripts to execute filesystem reads, writes, deletions, and traversals directly through path-anchored methods.

## Path instantiation and composition

To perform filesystem operations, instantiate a `Path` object using the `fs.path()` factory or the global alias `path()`:

```python
p = fs.path("/etc/hosts")
p = path("/etc/hosts")        # equivalent global alias
```

Once instantiated, you can query its properties to inspect path components:

| Property | Type | Description | Example (for `path("/home/alice/report.md")`) |
|----------|------|-------------|-----------------------------------------------|
| `.name` | `string` | The filename including its extension. | `"report.md"` |
| `.stem` | `string` | The filename excluding the extension. | `"report"` |
| `.suffix` | `string` | The file extension including the leading dot. | `".md"` |
| `.parent` | `Path` | The parent directory as a new `Path` object. | `path("/home/alice")` |
| `.parts` | `list[string]` | A list of individual path segments. | `["/", "home", "alice", "report.md"]` |
| `.string` | `string` | The clean string representation of the path. | `"/home/alice/report.md"` |

To compose and join new path segments, use the path slash operator (`/`) or the `.join()` method:

```python
# Build a nested path using the path slash operator
config = path("/home/alice") / "config" / "app.yaml"

# Alternatively, join segments using the join() method
config = path("/home/alice").join("config", "app.yaml")

# Resolves to the absolute path: /home/alice/config/app.yaml
print(config.string)
```

## File I/O operations

To read, write, or append data to a file, execute the corresponding methods directly on a `Path` object:

```python
p = path("config.yaml")

# Read the entire file as a string
content = p.read_text()

# Overwrite or create a file with text (default mode is 0644)
path("/tmp/output.txt").write_text("hello world")

# Append text to the end of a file
path("/tmp/output.txt").append_text("\nsecond line")
```

Binary operations are supported via `.read_bytes()`, `.write_bytes()`, and `.append_bytes()`:

```python
# Read binary data from a file (returns a bytes object)
binary_data = path("image.png").read_bytes()

# Write raw bytes to a file
path("copy.png").write_bytes(binary_data)

# Append binary data to a file using a bytes literal
path("data.bin").append_bytes(b"\x00\xff\x00\xff")
```

## Metadata and checks

You can query file types, check existence, and inspect system metadata:

* **`.exists()`**: Returns `True` if the path exists on disk.
* **`.is_file()` / `.is_dir()` / `.is_symlink()`**: Type-checking predicates.
* **`.stat()`**: Returns a dictionary containing file details (`name`, `size`, `mode`, `is_dir`, `mod_time`).
* **`.disk_usage()`**: Returns a dictionary containing storage space info (`total`, `used`, `free` in bytes) for the partition hosting the path.

```python
def main():
    target = path("/var/log/syslog")

    if target.exists() and target.is_file():
        info = target.stat()
        print("File size in bytes: " + str(info["size"]))
```

## Directory traversal and globbing

To locate, list, or recursively traverse files in a directory, use the following methods:

* **`.listdir()`**: Returns a list of immediate child `Path` objects in the directory.
* **`.glob(pattern)`**: Searches the directory and returns a list of child `Path` objects matching a wildcard glob pattern.
* **`.walk()`**: Recursively traverses the directory tree, returning a list of `tuple(dir_path, subdirs, files)` identical to Python's `os.walk`.

### Traversal examples

```python
def main():
    # List all immediate logs in a directory
    for entry in path("./logs").glob("*.log"):
        print("Log file: " + entry.name)

    # Recursively traverse and list all nested files
    for dir_path, subdirs, files in path("./src").walk():
        for f in files:
            # Re-assemble the path using the path slash operator
            full_path = dir_path / f
            print("Found source file: " + full_path.string)
```

## Common file operations

The `Path` object provides standard methods to manage files and directories:

| Method | Purpose |
|--------|---------|
| `.touch([exist_ok=True])` | Creates an empty file or updates its modification time. |
| `.mkdir([parents=False, mode=0755])` | Creates a new directory. Set `parents=True` to create intermediate parent folders. |
| `.remove()` | Deletes the file or empty directory. |
| `.rename(target_path)` | Renames or moves the file to a new path. |
| `.copy_to(target_path)` | Copies the file to a new destination. |
| `.move_to(target_path)` | Moves the file to a new destination. |
| `.expanduser()` | Resolves leading `~` or `~user` home directory prefixes. |

```python
# Create nested directories
path("./data/archive").mkdir(parents=True)

# Copy a file to a backup destination
path("config.json").copy_to("config.json.bak")
```

## Permissions

All filesystem interactions are gated by Starkite's security engine. Reading from the filesystem requires the `fs.read` capability, and modifying or deleting files requires `fs.write` or `fs.delete`. 

To run a script that accesses the filesystem, execute it with a permission profile like `--allow-fs`:

```bash
kite run ./backup.star --allow-fs
```

For more details, see the [Permission Guide](../fundamentals/security/permission.md).

## Failure handling

By default, executing an I/O operation on a missing file or path with insufficient permissions raises a Starlark-level execution error and terminates the script. 

To handle failures programmatically without halting the script, append `try_` to any I/O method name. These variants return a `Result` object:

```python
def main():
    # Safely attempt to read an optional configuration file
    result = path("optional-settings.json").try_read_text()

    if result.ok:
        settings = result.value
    else:
        print("Skipping optional settings: " + result.error)
```

## See also

* [`fs` API reference](../references/api/fs.md) — Detailed function signatures and properties.

