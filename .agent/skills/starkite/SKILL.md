---
name: starkite
description: >-
  Automate cloud-native infrastructure, systems operations, Kubernetes
  workflows, and local process tasks using the Starkite Starlark DSL. Use when
  writing, reviewing, or debugging Starkite scripts (.star), executing
  cloud/system tasks with kite, or automating workflows across files, HTTP,
  CLI commands, and Kubernetes.
---

# Starkite Automation DSL Skill

This skill teaches autonomous agents how to author idiomatic, sandboxed, and robust Starkite automation scripts.

## Tier 1: Core Language Invariants & Execution Rules

Autonomous agents generating Starkite scripts MUST adhere to four load-bearing execution rules:

1. **Automatic Entrypoint Execution (`main()`)**:
   - Starkite scripts define entry logic inside `def main():` or `def main(ctx):`.
   - **NEVER** call `main()` at the top-level. The runtime automatically discovers and invokes `main()`.
   - Calling `main()` manually causes duplicate execution and corrupts exit status codes.

2. **Error Handling via `try_` Result Pattern**:
   - Built-in functions have safe `try_` variants (e.g. `fs.try_read_text`, `os.try_exec`, `http.try_get`).
   - Safe functions return a `Result` (or `ExecResult`) with status fields: `res.ok` (bool), `res.value` (any), and `res.error` / `res.err` (string).
   - **ALWAYS** check `res.ok` before reading `res.value`. If `res.ok` is False, handle `res.error` (or `res.err`) gracefully.
   - Direct functions (e.g. `fs.read_text`) raise uncatchable runtime failures on error.

3. **Kernel Sandbox Isolation Constraints**:
   - Scripts execute within OS-level sandboxes (Landlock LSM on Linux, Seatbelt on macOS, AppContainers on Windows).
   - By default, file writes and network access outside the script directory are restricted.
   - Always use `fs.path` for path normalization across operating systems (automatically converts `/` and `\`).
   - Scripts requiring network or write access must use appropriate CLI flags (`--sandbox-net-allow`, `--sandbox-path-write`).

4. **Auto-Loaded Standard Library Modules**:
   - Standard modules (`fs`, `http`, `os`, `json`, `yaml`, `concur`, `time`, `log`, `csv`, `zip`, `gzip`, `base64`, `hash`, `table`, `template`, `strings`, `args`, `runtime`, `regexp`, `uuid`) are predeclared globals.
   - **DO NOT** write `import fs` or `load("fs", ...)` for built-in modules.
   - Use `load("//path/to/script.star", "symbol")` strictly for relative or installed user modules.

## Tier 2: Scripting Idioms & Production Recipes

### Robust File I/O & Path Traversal
```python
def main():
    p = fs.path("data/config.json")
    if p.exists():
        # Safe read with Result handling
        res = p.try_read_text()
        if res.ok:
            log.info("Read %d bytes from %s" % (len(res.value), p.name))
        else:
            log.error("Read failed: %s" % res.error)
```

### Subprocess Execution & Command Piping
```python
def main():
    # Execute system command with argument vector (prevents shell injection)
    res = os.try_exec("git", ["status", "--porcelain"])
    if res.ok:
        stdout = res.stdout.strip()
        if stdout:
            log.warn("Uncommitted changes:\n%s" % stdout)
    else:
        log.error("Command failed with exit code %d: %s" % (res.code, res.error))
```

### Structured Data Transformation (JSON to YAML)
```python
def main():
    raw_json = '{"service": "api", "replicas": 3, "enabled": true}'
    data = json.decode(raw_json)
    data["environment"] = "production"

    # Encode transformed data directly into YAML
    yaml_out = yaml.encode(data)
    fs.write_text("deploy.yaml", yaml_out)
```

### Concurrent Fan-Out & Rate-Limited Pipelines
```python
def check_endpoint(target):
    res = http.try_get("https://" + target + "/healthz")
    return {"target": target, "alive": res.ok and res.value.status_code == 200}

def main():
    endpoints = ["node1.internal", "node2.internal", "node3.internal"]
    # Concurrently check endpoints with worker pool limit of 2
    results = concur.map(endpoints, check_endpoint, workers=2)
    for r in results:
        log.info("Endpoint %s alive=%s" % (r["target"], r["alive"]))
```

### Kubernetes Cluster Triage & Resource Inspection
```python
def main():
    pods_res = k8s.try_list("pods", namespace="default")
    if pods_res.ok:
        for pod in pods_res.value:
            phase = pod.status.phase
            if phase != "Running":
                log.warn("Pod %s is in state: %s" % (pod.metadata.name, phase))
    else:
        log.error("Failed to query cluster: %s" % pods_res.error)
```

## Tier 3: Complete Module & Member Reference Tables

Reference documentation for all functions, methods, and return types within this skill's scope.

### Module `args`

args provides declarative command-line argument and flag parsing: string, int, bool, float, list, positional, parse

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `bool` | `args.bool(name, ...)` | `None` | Declare a boolean flag | `args.try_bool` |
| `float` | `args.float(name, ...)` | `None` | Declare a floating-point flag with optional range limits | `args.try_float` |
| `int` | `args.int(name, ...)` | `None` | Declare an integer flag with optional range limits | `args.try_int` |
| `list` | `args.list(name, ...)` | `None` | Declare a multi-value flag (repeatable or comma-separated) | `args.try_list` |
| `parse` | `args.parse()` | `ArgsResult` | Parse forwarded script arguments against declared schema | `args.try_parse` |
| `positional` | `args.positional(name, ...)` | `None` | Declare a positional argument | `args.try_positional` |
| `string` | `args.string(name, ...)` | `None` | Declare a string flag | `args.try_string` |

### Module `base64`

base64 provides base64 encoding/decoding: file, text, bytes

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `bytes` | `base64.bytes(data)` | `base64.source` | Create a Base64 source from raw bytes | `base64.try_bytes` |
| `file` | `base64.file(path)` | `base64.file` | Create a Base64 handle for a file | `base64.try_file` |
| `text` | `base64.text(s)` | `base64.source` | Create a Base64 source from a string | `base64.try_text` |

### Module `concur`

concur provides concurrent execution: map, each, exec

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `each` | `concur.each(items, func, workers=0, timeout="", on_error="abort")` | `None` | Apply `func` to each item concurrently (no return values) | `concur.try_each` |
| `exec` | `concur.exec(*fns, timeout="", on_error="abort")` | `tuple` | Execute multiple functions concurrently, returning their results | `concur.try_exec` |
| `map` | `concur.map(items, func, workers=0, timeout="", on_error="abort")` | `list` | Apply `func` to each item concurrently, returning results in order | `concur.try_map` |

### Module `containers`

containers provides container engine management: config() returns a client for daemon operations

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `config` | `containers.config()` | `None` | - | `containers.try_config` |
| `delete` | `containers.delete()` | `None` | - | `containers.try_delete` |
| `exec` | `containers.exec()` | `None` | - | `containers.try_exec` |
| `image_build` | `containers.image_build()` | `None` | - | `containers.try_image_build` |
| `image_inspect` | `containers.image_inspect()` | `None` | - | `containers.try_image_inspect` |
| `image_list` | `containers.image_list()` | `None` | - | `containers.try_image_list` |
| `image_pull` | `containers.image_pull()` | `None` | - | `containers.try_image_pull` |
| `image_remove` | `containers.image_remove()` | `None` | - | `containers.try_image_remove` |
| `inspect` | `containers.inspect()` | `None` | - | `containers.try_inspect` |
| `list` | `containers.list()` | `None` | - | `containers.try_list` |
| `remove` | `containers.remove()` | `None` | - | `containers.try_remove` |
| `run` | `containers.run()` | `None` | - | `containers.try_run` |
| `stop` | `containers.stop()` | `None` | - | `containers.try_stop` |

### Module `csv`

csv provides CSV file reading/writing: file, from

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `file` | `csv.file(path)` | `csv.file` | Create a file object for reading CSV | `csv.try_file` |
| `source` | `csv.source(data)` | `csv.writer` | Create a writer object from data | `csv.try_source` |

#### Exported Type `csv.file` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `path` | `f.path` | `string` | The file path |
| `read` | `f.read(sep=",", comment="", skip=0, header=False)` | `list` | Read and parse the CSV file |
| `try_read` | `f.try_read(sep=",", comment="", skip=0, header=False)` | `Result` | Read the file, returning a Result |

### Module `fleet`

fleet provides compute resource fleet management: new, file, hosts_file

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `file` | `fleet.file(path)` | `Fleet` | Load a fleet from a static YAML or JSON file. Alias for `fleet.new(file=path)`. | `fleet.try_file` |
| `host_file` | `fleet.host_file(path="/etc/hosts", loopback=False)` | `Fleet` | Alias for `fleet.hosts_file()`. | `fleet.try_host_file` |
| `hosts_file` | `fleet.hosts_file(path="/etc/hosts", loopback=False)` | `Fleet` | Ingest compute resources from a standard POSIX hosts file. | `fleet.try_hosts_file` |
| `new` | `fleet.new()` | `None` | - | `fleet.try_new` |

### Module `fmt`

fmt provides formatting functions: printf, println, sprintf

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `printf` | `fmt.printf(format, *args)` | `None` | Print formatted output to stdout | `fmt.try_printf` |
| `println` | `fmt.println(*args)` | `None` | Print arguments to stdout followed by a newline | `fmt.try_println` |
| `sprintf` | `fmt.sprintf(format, *args)` | `string` | Return a formatted string | `fmt.try_sprintf` |

### Module `fs`

fs provides file system operations: read, write, copy, move, path manipulation

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `exists` | `p.exists()` | `bool` | Check if path exists on disk | `fs.try_exists` |
| `glob` | `p.glob(pattern)` | `list[Path]` | Glob within this directory | `fs.try_glob` |
| `path` | `path(p)` | `Path` | Create a Path object | `fs.try_path` |
| `read_bytes` | `p.read_bytes()` | `bytes` | Read file as bytes | `fs.try_read_bytes` |
| `read_text` | `p.read_text()` | `string` | Read file as text | `fs.try_read_text` |
| `write_bytes` | `p.write_bytes(data)` | `None` | Write bytes to file | `fs.try_write_bytes` |
| `write_text` | `p.write_text(content)` | `None` | Write text to file | `fs.try_write_text` |

#### Exported Type `fs.path` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `append_bytes` | `p.append_bytes(data)` | `None` | Append bytes to file |
| `append_from` | `p.append_from(reader)` | `int` | Read from `io.reader` and append to file |
| `append_text` | `p.append_text(content)` | `None` | Append text to file |
| `chmod` | `p.chmod(mode)` | `None` | Change permissions |
| `chown` | `p.chown(uid=-1, gid=-1)` | `None` | Change file ownership |
| `clean` | `p.clean()` | `Path` | Return cleaned path (resolves `.` and `..`) |
| `copy_to` | `p.copy_to(target)` | `Path` | Copy file to target path |
| `disk_usage` | `p.disk_usage()` | `dict` | Disk space info with total, used, free |
| `exists` | `p.exists()` | `bool` | Check if path exists on disk |
| `expanduser` | `p.expanduser()` | `Path` | Expand `~` to home directory |
| `get_reader` | `p.get_reader()` | `io.reader` | Open file for streaming reads |
| `get_writer` | `p.get_writer()` | `io.writer` | Open file for streaming writes (truncates file) |
| `glob` | `p.glob(pattern)` | `list[Path]` | Glob within this directory |
| `group` | `p.group()` | `string` | Get file group name |
| `hardlink_to` | `p.hardlink_to(target)` | `None` | Create hard link at this path pointing to target |
| `is_absolute` | `p.is_absolute()` | `bool` | Check if path is absolute |
| `is_dir` | `p.is_dir()` | `bool` | Check if path is a directory |
| `is_file` | `p.is_file()` | `bool` | Check if path is a regular file |
| `is_relative_to` | `p.is_relative_to(base)` | `bool` | Check if path is relative to base |
| `is_symlink` | `p.is_symlink()` | `bool` | Check if path is a symbolic link |
| `join` | `p.join(other)` | `Path` | Join with another path component |
| `listdir` | `p.listdir()` | `list[Path]` | List directory entries as Paths |
| `match` | `p.match(pattern)` | `bool` | Test against glob pattern |
| `mkdir` | `p.mkdir()` | `None` | Create directory (and parents) |
| `move_to` | `p.move_to(target)` | `Path` | Move file/directory to target |
| `name` | `name` | `string` | Filename with extension (`"report.md"`) |
| `owner` | `p.owner()` | `string` | Get file owner name |
| `parent` | `parent` | `Path` | Parent directory as a Path |
| `parts` | `parts` | `tuple` | Path components as a tuple |
| `read_bytes` | `p.read_bytes()` | `bytes` | Read file as bytes |
| `read_from` | `p.read_from(reader)` | `int` | Read from `io.reader` and write to file (truncates file) |
| `read_text` | `p.read_text()` | `string` | Read file as text |
| `readlink` | `p.readlink()` | `Path` | Read symlink target |
| `relative_to` | `p.relative_to(base)` | `Path` | Get relative path from base |
| `remove` | `p.remove()` | `None` | Remove file or directory |
| `rename` | `p.rename(target)` | `Path` | Rename/move to target |
| `resolve` | `p.resolve()` | `Path` | Resolve to absolute path |
| `stat` | `p.stat()` | `dict` | Get file metadata |
| `stem` | `stem` | `string` | Filename without extension (`"report"`) |
| `string` | `string` | `string` | Full path as string |
| `suffix` | `suffix` | `string` | File extension (`".md"`) |
| `symlink_to` | `p.symlink_to(target)` | `None` | Create symlink pointing to target |
| `touch` | `p.touch()` | `None` | Create file or update mtime |
| `truncate` | `p.truncate(size)` | `None` | Truncate file to given size |
| `try_append_bytes` | `p.try_append_bytes(data)` | `Result` | Append bytes to file Returns a Result instead of raising. |
| `try_append_from` | `p.try_append_from(reader)` | `Result` | Read from `io.reader` and append to file Returns a Result instead of raising. |
| `try_append_text` | `p.try_append_text(content)` | `Result` | Append text to file Returns a Result instead of raising. |
| `try_chmod` | `p.try_chmod(mode)` | `Result` | Change permissions Returns a Result instead of raising. |
| `try_chown` | `p.try_chown(uid=-1, gid=-1)` | `Result` | Change file ownership Returns a Result instead of raising. |
| `try_copy_to` | `p.try_copy_to(target)` | `Result` | Copy file to target path Returns a Result instead of raising. |
| `try_disk_usage` | `p.try_disk_usage()` | `Result` | Disk space info with total, used, free Returns a Result instead of raising. |
| `try_exists` | `p.try_exists()` | `Result` | Check if path exists on disk Returns a Result instead of raising. |
| `try_get_reader` | `p.try_get_reader()` | `Result` | Open file for streaming reads Returns a Result instead of raising. |
| `try_get_writer` | `p.try_get_writer()` | `Result` | Open file for streaming writes (truncates file) Returns a Result instead of raising. |
| `try_glob` | `p.try_glob(pattern)` | `Result` | Glob within this directory Returns a Result instead of raising. |
| `try_group` | `p.try_group()` | `Result` | Get file group name Returns a Result instead of raising. |
| `try_hardlink_to` | `p.try_hardlink_to(target)` | `Result` | Create hard link at this path pointing to target Returns a Result instead of raising. |
| `try_is_dir` | `p.try_is_dir()` | `Result` | Check if path is a directory Returns a Result instead of raising. |
| `try_is_file` | `p.try_is_file()` | `Result` | Check if path is a regular file Returns a Result instead of raising. |
| `try_is_symlink` | `p.try_is_symlink()` | `Result` | Check if path is a symbolic link Returns a Result instead of raising. |
| `try_listdir` | `p.try_listdir()` | `Result` | List directory entries as Paths Returns a Result instead of raising. |
| `try_mkdir` | `p.try_mkdir()` | `Result` | Create directory (and parents) Returns a Result instead of raising. |
| `try_move_to` | `p.try_move_to(target)` | `Result` | Move file/directory to target Returns a Result instead of raising. |
| `try_owner` | `p.try_owner()` | `Result` | Get file owner name Returns a Result instead of raising. |
| `try_read_bytes` | `p.try_read_bytes()` | `Result` | Read file as bytes Returns a Result instead of raising. |
| `try_read_from` | `p.try_read_from(reader)` | `Result` | Read from `io.reader` and write to file (truncates file) Returns a Result instead of raising. |
| `try_read_text` | `p.try_read_text()` | `Result` | Read file as text Returns a Result instead of raising. |
| `try_readlink` | `p.try_readlink()` | `Result` | Read symlink target Returns a Result instead of raising. |
| `try_remove` | `p.try_remove()` | `Result` | Remove file or directory Returns a Result instead of raising. |
| `try_rename` | `p.try_rename(target)` | `Result` | Rename/move to target Returns a Result instead of raising. |
| `try_stat` | `p.try_stat()` | `Result` | Get file metadata Returns a Result instead of raising. |
| `try_symlink_to` | `p.try_symlink_to(target)` | `Result` | Create symlink pointing to target Returns a Result instead of raising. |
| `try_touch` | `p.try_touch()` | `Result` | Create file or update mtime Returns a Result instead of raising. |
| `try_truncate` | `p.try_truncate(size)` | `Result` | Truncate file to given size Returns a Result instead of raising. |
| `try_walk` | `p.try_walk()` | `Result` | Recursive directory traversal (Path, dirs, files) Returns a Result instead of raising. |
| `try_write_bytes` | `p.try_write_bytes(data)` | `Result` | Write bytes to file Returns a Result instead of raising. |
| `try_write_text` | `p.try_write_text(content)` | `Result` | Write text to file Returns a Result instead of raising. |
| `try_write_to` | `p.try_write_to(writer)` | `Result` | Read file and write to another `io.writer` Returns a Result instead of raising. |
| `walk` | `p.walk()` | `list[tuple]` | Recursive directory traversal (Path, dirs, files) |
| `with_name` | `p.with_name(name)` | `Path` | Replace the filename |
| `with_suffix` | `p.with_suffix(suffix)` | `Path` | Replace the extension |
| `write_bytes` | `p.write_bytes(data)` | `None` | Write bytes to file |
| `write_text` | `p.write_text(content)` | `None` | Write text to file |
| `write_to` | `p.write_to(writer)` | `int` | Read file and write to another `io.writer` |

### Module `gzip`

gzip provides compression/decompression: file, text, bytes

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `bytes` | `gzip.bytes(data)` | `gzip.source` | Create a gzip source from raw bytes | `gzip.try_bytes` |
| `file` | `gzip.file(path)` | `gzip.file` | Create a gzip file handle for compress/decompress operations | `gzip.try_file` |
| `text` | `gzip.text(s)` | `gzip.source` | Create a gzip source from a string | `gzip.try_text` |

### Module `hash`

hash provides cryptographic hash functions: file, text, bytes

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `bytes` | `hash.bytes(data)` | `hash.source` | Create a hash source from raw bytes | `hash.try_bytes` |
| `file` | `hash.file(path)` | `hash.file` | Create a hash source from a file | `hash.try_file` |
| `text` | `hash.text(s)` | `hash.source` | Create a hash source from a string | `hash.try_text` |

### Module `http`

http provides HTTP client and server: get, post, put, patch, delete, url, config, server, serve

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `config` | `http.config(timeout="30s", headers={})` | `None` | Set default client configuration | `http.try_config` |
| `delete` | `http.delete(url, headers={}, timeout="")` | `http.response` | Send DELETE request directly to a URL | `http.try_delete` |
| `get` | `http.get(url, headers={}, timeout="")` | `http.response` | Send GET request directly to a URL | `http.try_get` |
| `patch` | `http.patch(url, body=None, headers={}, timeout="")` | `http.response` | Send PATCH request directly to a URL | `http.try_patch` |
| `post` | `http.post(url, body=None, headers={}, timeout="")` | `http.response` | Send POST request directly to a URL | `http.try_post` |
| `put` | `http.put(url, body=None, headers={}, timeout="")` | `http.response` | Send PUT request directly to a URL | `http.try_put` |
| `serve` | `http.serve(handler_or_routes, port=0, host="", tls_cert="", tls_key="")` | `None` | Quick-start a server with a handler or route dict | `http.try_serve` |
| `server` | `http.server(port=0, host="", tls_cert="", tls_key="", read_timeout="", write_timeout="", idle_timeout="", shutdown_timeout="", max_header_bytes=0, max_body_bytes=0)` | `http.server` | Create an HTTP server. Timeout kwargs take duration strings (e.g. `"30s"`); `max_*_bytes` take integer byte sizes | `http.try_server` |
| `url` | `http.url(url_string)` | `http.url` | Create a URL object for making requests | `http.try_url` |

#### Exported Type `http.server` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `handle` | `srv.handle(pattern, handler)` | `None` | Register a route handler |
| `port` | `srv.port()` | `int` | Get the port the server is listening on |
| `serve` | `srv.serve(port=0, host="", tls_cert="", tls_key="")` | `None` | Start server and block |
| `shutdown` | `srv.shutdown()` | `None` | Gracefully shut down the server |
| `start` | `srv.start(port=0, host="", tls_cert="", tls_key="")` | `None` | Start server in background |
| `try_handle` | `srv.try_handle(pattern, handler)` | `Result` | Register a route handler Returns a Result instead of raising. |
| `try_port` | `srv.try_port()` | `Result` | Get the port the server is listening on Returns a Result instead of raising. |
| `try_serve` | `srv.try_serve(port=0, host="", tls_cert="", tls_key="")` | `Result` | Start server and block Returns a Result instead of raising. |
| `try_shutdown` | `srv.try_shutdown()` | `Result` | Gracefully shut down the server Returns a Result instead of raising. |
| `try_start` | `srv.try_start(port=0, host="", tls_cert="", tls_key="")` | `Result` | Start server in background Returns a Result instead of raising. |
| `try_use` | `srv.try_use(middleware)` | `Result` | Add middleware function Returns a Result instead of raising. |
| `use` | `srv.use(middleware)` | `None` | Add middleware function |

#### Exported Type `http.url` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `delete` | `url.delete(headers={}, timeout="")` | `http.response` | Send DELETE request |
| `get` | `url.get(headers={}, timeout="")` | `http.response` | Send GET request |
| `patch` | `url.patch(body, headers={}, timeout="")` | `http.response` | Send PATCH request |
| `post` | `url.post(body, headers={}, timeout="")` | `http.response` | Send POST request |
| `put` | `url.put(body, headers={}, timeout="")` | `http.response` | Send PUT request |
| `try_delete` | `url.try_delete(headers={}, timeout="")` | `Result` | Send DELETE request Returns a Result instead of raising. |
| `try_get` | `url.try_get(headers={}, timeout="")` | `Result` | Send GET request Returns a Result instead of raising. |
| `try_patch` | `url.try_patch(body, headers={}, timeout="")` | `Result` | Send PATCH request Returns a Result instead of raising. |
| `try_post` | `url.try_post(body, headers={}, timeout="")` | `Result` | Send POST request Returns a Result instead of raising. |
| `try_put` | `url.try_put(body, headers={}, timeout="")` | `Result` | Send PUT request Returns a Result instead of raising. |

### Module `inventory`

inventory provides resource discovery: file, list, filter, group_by, merge, addresses

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `addresses` | `inventory.addresses(inventory, key="address")` | `list` | Extract a list of addresses from inventory items | `inventory.try_addresses` |
| `file` | `inventory.file(path)` | `InventoryValue` | Load an inventory from a YAML or JSON file | `inventory.try_file` |
| `filter` | `inventory.filter(inventory, filter_fn=None, **kwargs)` | `InventoryValue` | Filter items by a positional predicate function or keyword matches | `inventory.try_filter` |
| `group_by` | `inventory.group_by(inventory, key)` | `dict` | Group items by a key, returning `{value: InventoryValue}` | `inventory.try_group_by` |
| `list` | `inventory.list(inventory)` | `list[dict]` | Return all items as a list of dictionaries | `inventory.try_list` |
| `merge` | `inventory.merge(*inventories)` | `InventoryValue` | Merge multiple inventories into one | `inventory.try_merge` |

### Module `io`

io provides interactive I/O: confirm, prompt

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `confirm` | `io.confirm(msg, default=False)` | `bool` | Prompt the user for a yes/no confirmation | `io.try_confirm` |
| `prompt` | `io.prompt(msg, default="", secret=False)` | `string` | Prompt the user for text input. Set `secret=True` to hide input (e.g. passwords) | `io.try_prompt` |

### Module `json`

json provides JSON file reading/writing: file, from

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `decode` | `json.decode(string)` | `any` | Decode a JSON string to a value | `json.try_decode` |
| `encode` | `json.encode(value)` | `string` | Encode a value to a JSON string | `json.try_encode` |
| `file` | `json.file(path)` | `json.file` | Create a file object for reading JSON | `json.try_file` |
| `source` | `json.source(data)` | `json.writer` | Create a writer object from data | `json.try_source` |

#### Exported Type `json.file` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `decode` | `f.decode()` | `any` | Decode the entire file |
| `path` | `f.path` | `string` | The file path |
| `try_decode` | `f.try_decode()` | `Result` | Decode the file, returning a Result |

### Module `k8s`

k8s provides 3-tier Kubernetes operations: CRUD (get, list, apply, delete, patch, label, annotate), watch (watch, wait_for), I/O (logs, exec, port_forward), high-level (deploy, scale, rollout, drain), and k8s.obj constructors

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `annotate` | `k8s.annotate(kind, name, annotations, namespace="", timeout="")` | `AttrDict` | Set annotations on a resource | `k8s.try_annotate` |
| `api_resources` | `k8s.api_resources(timeout="")` | `list[AttrDict]` | Available API resources | `k8s.try_api_resources` |
| `apply` | `k8s.apply(manifest, namespace="", field_manager="starkite", dry_run=False, force=False, prune=False, prune_labels={}, timeout="")` | `AttrDict` | Apply a resource (server-side apply). When `prune=True`, prunes omitted resources managed by `field_manager` matching `prune_labels` | `k8s.try_apply` |
| `autoscale` | `k8s.autoscale(kind, name, min=1, max=10, cpu_percent=80, namespace="", timeout="")` | `AttrDict` | Create a HorizontalPodAutoscaler | `k8s.try_autoscale` |
| `claims` | `k8s.claims(namespace="", labels="", timeout="")` | `list[AttrDict]` | List `resource.k8s.io/v1` `ResourceClaim` objects for Dynamic Resource Allocation (DRA) | `k8s.try_claims` |
| `condition` | `k8s.condition()` | `None` | - | - |
| `config` | `k8s.config(kubeconfig="", context="", namespace="")` | `Client` | Return a configured Kubernetes client bound to the given kubeconfig path, context, and namespace | `k8s.try_config` |
| `context` | `k8s.context()` | `string` | Current kubeconfig context name | `k8s.try_context` |
| `control` | `k8s.control()` | `None` | - | `k8s.try_control` |
| `cordon` | `k8s.cordon(node, timeout="")` | `AttrDict` | Mark a node unschedulable | `k8s.try_cordon` |
| `cp` | `k8s.cp(pod, src, dst, namespace="", container="", timeout="")` | `AttrDict` | Copy files to/from a pod. Use `pod:path` as `src` to download, `pod:path` as `dst` to upload | `k8s.try_cp` |
| `create` | `k8s.create(manifest, namespace="", dry_run=False, timeout="")` | `AttrDict` | Create a resource from a manifest (dict, AttrDict, or YAML string) | `k8s.try_create` |
| `debug` | `k8s.debug(name, image="nicolaka/netshoot", target_container="", command=None, namespace="", timeout="3m")` | `AttrDict` | Attach an ephemeral container to a running pod for zero-shell diagnostics. Returns `{"stdout", "stderr", "code", "container", "pod"}` | `k8s.try_debug` |
| `delete` | `k8s.delete(kind, name, namespace="", propagation="Background", timeout="")` | `None` | Delete a resource | `k8s.try_delete` |
| `deploy` | `k8s.deploy(name, image, replicas=1, port=0, namespace="", labels=None, env=None, timeout="")` | `AttrDict` | Create a Deployment (returns `{"deployment": str, "service": str}`) | `k8s.try_deploy` |
| `describe` | `k8s.describe(kind, name, namespace="", timeout="")` | `AttrDict` | Return detailed description of a resource, with `.resource`, `.conditions`, and `.events` | `k8s.try_describe` |
| `diff` | `k8s.diff(manifest, namespace="", field_manager="starkite", timeout="")` | `AttrDict` | Server-side apply dry-run diff against live cluster state (`has_drift`, `drifted_fields`, `conflicts`, `diff`, `live`, `applied`) | `k8s.try_diff` |
| `drain` | `k8s.drain(node, force=False, ignore_daemonsets=False, timeout="")` | `AttrDict` | Drain a node (evict pods). `force` continues past pods not backed by a controller; `ignore_daemonsets` leaves DaemonSet pods in place | `k8s.try_drain` |
| `event` | `k8s.event(obj, reason, message, type="Normal", namespace="", timeout="")` | `AttrDict` | Emit a Kubernetes event attached to `obj`. `type` can be `"Normal"` or `"Warning"` | `k8s.try_event` |
| `events` | `k8s.events(target=None, kind="", name="", namespace="", type="", reason="", since="", limit=0, timeout="")` | `list[AttrDict]` | Query diagnostic event streams with dual-API normalization (`events.k8s.io/v1` and `core/v1`). Filter by target object, kind, name, type (`"Warning"`/`"Normal"`), reason substring, or relative duration (`since="30m"`). Returns items sorted newest first with relative `age` | `k8s.try_events` |
| `evict` | `k8s.evict(name, namespace="", dry_run=False, timeout="")` | `AttrDict` | Evict a Pod via `/eviction` subresource, honoring `PodDisruptionBudgets` | `k8s.try_evict` |
| `exec` | `k8s.exec(name, command, namespace="", container="", timeout="")` | `AttrDict` | Run a command in a pod. `command` may be a string (executed via `/bin/sh -c`) or a list (argv). Returns `{"stdout", "stderr", "code"}` | `k8s.try_exec` |
| `expose` | `k8s.expose(kind, name, port, target_port=0, type="ClusterIP", namespace="", timeout="")` | `AttrDict` | Expose a resource as a Service | `k8s.try_expose` |
| `finalizer` | `finalizer` | `string` | Custom finalizer string. Injected on creation and removed after `finalize()` completes cleanly | `k8s.try_finalizer` |
| `get` | `k8s.get(kind, name, namespace="", timeout="")` | `AttrDict` | Get a single resource | `k8s.try_get` |
| `is_deleting` | `k8s.is_deleting(obj)` | `bool` | Check if `obj.metadata.deletionTimestamp` is set | `k8s.try_is_deleting` |
| `label` | `k8s.label(kind, name, labels, namespace="", timeout="")` | `AttrDict` | Set labels on a resource | `k8s.try_label` |
| `list` | `k8s.list(kind, namespace="", labels="", fields="", timeout="")` | `list[AttrDict]` | List resources with optional label/field selectors | `k8s.try_list` |
| `logs` | `k8s.logs(name, namespace="", container="", tail=0, since="", previous=False, timeout="")` | `string` | Fetch pod logs. `tail` caps line count; `since` is a duration string (e.g., `"10m"`); `previous=True` reads the previous container instance | `k8s.try_logs` |
| `logs_follow` | `k8s.logs_follow(name, handler, namespace="", container="", tail=0, timeout="")` | `None` | Stream logs, calling `handler(line)` per line. Blocks until the pod ends or `timeout` elapses | `k8s.try_logs_follow` |
| `namespace_name` | `k8s.namespace_name()` | `string` | Default namespace for the current context | `k8s.try_namespace_name` |
| `obj` | `k8s.obj()` | `None` | - | - |
| `patch` | `k8s.patch(kind, name, patch, namespace="", type="merge", timeout="")` | `AttrDict` | Patch a resource. `type`: `"merge"`, `"strategic"`, or `"json"` | `k8s.try_patch` |
| `port_forward` | `k8s.port_forward(name, port, local_port=0, namespace="")` | `PortForwardHandle` | Forward a local port to a pod port. Blocks until interrupted. `local_port=0` picks a free port | `k8s.try_port_forward` |
| `pvcs` | `k8s.pvcs(namespace="", labels="", timeout="")` | `list[AttrDict]` | List `PersistentVolumeClaim` objects | `k8s.try_pvcs` |
| `pvs` | `k8s.pvs(labels="", timeout="")` | `list[AttrDict]` | List cluster-scoped `PersistentVolume` objects | `k8s.try_pvs` |
| `resize` | `k8s.resize(name, container, cpu=None, memory=None, requests=None, limits=None, namespace="", timeout="30s")` | `AttrDict` | Resize CPU and memory allocations of a running container in-place without pod restart | `k8s.try_resize` |
| `rollout` | `k8s.rollout(kind, name, action="status", namespace="", timeout="")` | `AttrDict` | Manage rollouts. `action`: `"status"`, `"restart"`, `"pause"`, `"resume"` | `k8s.try_rollout` |
| `route` | `k8s.route(name, gateway, service, port, prefix="/", host="", namespace="", timeout="")` | `AttrDict` | Create or update an HTTPRoute binding a Gateway to a Service backend (returns `{"route": str}`) | `k8s.try_route` |
| `run` | `k8s.run(name, image, command=None, namespace="", restart="Never", rm=False, timeout="3m")` | `AttrDict` | Run a one-off Pod (like `kubectl run`) | `k8s.try_run` |
| `scale` | `k8s.scale(kind, name, replicas, namespace="", timeout="")` | `AttrDict` | Scale a resource to the given replica count | `k8s.try_scale` |
| `set_env` | `k8s.set_env(kind, name, env, namespace="", container="", timeout="")` | `AttrDict` | Set environment variables on a resource | `k8s.try_set_env` |
| `set_image` | `k8s.set_image(kind, name, container, image, namespace="", timeout="")` | `AttrDict` | Update the container image of a resource | `k8s.try_set_image` |
| `set_resources` | `k8s.set_resources(kind, name, requests=None, limits=None, namespace="", container="", timeout="")` | `AttrDict` | Set resource requests and limits | `k8s.try_set_resources` |
| `status` | `k8s.status(obj, status, namespace="", timeout="")` | `AttrDict` | Update the status subresource of a resource. Pass the resource as `obj` and the new status dict as `status` | `k8s.try_status` |
| `storage_classes` | `k8s.storage_classes(labels="", timeout="")` | `list[AttrDict]` | List cluster-scoped `StorageClass` objects | `k8s.try_storage_classes` |
| `taint` | `k8s.taint(node, key, value="", effect="", timeout="")` | `AttrDict` | Add a taint to a node. `effect`: `"NoSchedule"`, `"PreferNoSchedule"`, or `"NoExecute"` | `k8s.try_taint` |
| `top_nodes` | `k8s.top_nodes(timeout="")` | `list[AttrDict]` | CPU/memory capacity and allocatable per node | `k8s.try_top_nodes` |
| `top_pods` | `k8s.top_pods(namespace="", sort_by="", timeout="")` | `list[AttrDict]` | Resource requests and status per pod | `k8s.try_top_pods` |
| `uncordon` | `k8s.uncordon(node, timeout="")` | `AttrDict` | Re-enable scheduling on a node | `k8s.try_uncordon` |
| `untaint` | `k8s.untaint(node, key, timeout="")` | `AttrDict` | Remove a taint from a node by key | `k8s.try_untaint` |
| `validate` | `k8s.validate(manifest, policy=None, expression="", strict=False)` | `AttrDict` | Evaluate CEL validation policies or expressions against a manifest client-side prior to cluster submission. Returns `{"valid": bool, "violations": list[str], "policy": str}` | `k8s.try_validate` |
| `version` | `k8s.version(timeout="")` | `AttrDict` | Kubernetes server version info (`.major`, `.minor`, `.git_version`, `.platform`) | `k8s.try_version` |
| `wait_for` | `k8s.wait_for(kind, name, condition="", namespace="", timeout="")` | `AttrDict` | Block until the named resource meets the given condition (e.g., `"Available"`, `"Ready"`, `"Complete"`) or the timeout expires. Returns `{"ready": bool, "resource": AttrDict, "message": str}` | `k8s.try_wait_for` |
| `watch` | `k8s.watch(kind, namespace="", labels="", timeout="", handler=None)` | `list[AttrDict]` or `None` | Watch a resource kind. If `handler` is supplied, call it per event (`handler(event_type, obj)`) and return `None`; otherwise collect events and return a list of `{"type": ..., "object": ...}` AttrDicts. `timeout` caps wall-clock duration | `k8s.try_watch` |
| `webhook` | `k8s.webhook()` | `None` | - | `k8s.try_webhook` |
| `yaml` | `k8s.yaml(manifest)` | `string` | Render a manifest dict or KubeResource as YAML | `k8s.try_yaml` |

### Module `log`

log provides structured logging: debug, info, warn, error, set_level, set_format, set_output, logger

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `debug` | `log.debug(msg, attrs={})` | `None` | Log a debug message with optional attributes | `log.try_debug` |
| `error` | `log.error(msg, attrs={})` | `None` | Log an error message with optional attributes | `log.try_error` |
| `info` | `log.info(msg, attrs={})` | `None` | Log an info message with optional attributes | `log.try_info` |
| `logger` | `log.logger(level="info", format="text", output="stderr")` | `Logger` | Create a new independent logger | `log.try_logger` |
| `set_format` | `log.set_format(format)` | `None` | Set the module-level log format (`"text"` or `"json"`) | `log.try_set_format` |
| `set_level` | `log.set_level(level)` | `None` | Set the module-level log level (`"debug"`, `"info"`, `"warn"`, `"error"`) | `log.try_set_level` |
| `set_output` | `log.set_output(output)` | `None` | Set the module-level output (`"stderr"` or `"stdout"`) | `log.try_set_output` |
| `warn` | `log.warn(msg, attrs={})` | `None` | Log a warning message with optional attributes | `log.try_warn` |

### Module `mcp`

mcp provides Model Context Protocol server + client

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `connect` | `mcp.connect(transport, timeout=)` | `MCPClient` | Connect to a remote MCP server | `mcp.try_connect` |
| `serve` | `mcp.serve(name=, tools=, resources=, prompts=, port=, host=, path=, tls_cert=, tls_key=, version=)` | `None` | Start a blocking MCP server (stdio or HTTP) | `mcp.try_serve` |

### Module `os`

os provides operating system operations: env, cwd, exec, which, user info

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `bash` | `os.bash(...)` | `Shell` | Construct `/bin/bash` shell (`flag="-c"`) | `os.try_bash` |
| `chdir` | `os.chdir(path)` | `None` | Change current working directory | `os.try_chdir` |
| `cmdexe` | `os.cmdexe(...)` | `Shell` | Construct Windows `cmd.exe` shell (`flag="/c"`) | `os.try_cmdexe` |
| `cwd` | `os.cwd()` | `string` | Get current working directory | `os.try_cwd` |
| `env` | `os.env(name, default="")` | `string` | Get environment variable, with optional default | `os.try_env` |
| `exec` | `os.exec(cmd, args=[], env=None, cwd=None, timeout="60s", userid=None, groupid=None, input=None, output=None)` | `string` | Execute a binary directly (returns stdout) | `os.try_exec` |
| `exit` | `os.exit(code=0)` | `None` | Exit the process with the given code | `os.try_exit` |
| `groupid` | `os.groupid()` | `string` | Get current group ID | `os.try_groupid` |
| `home` | `os.home()` | `string` | Get home directory path | `os.try_home` |
| `hostname` | `os.hostname()` | `string` | Get system hostname | `os.try_hostname` |
| `pid` | `os.pid()` | `int` | Get current process ID | `os.try_pid` |
| `powershell` | `os.powershell(...)` | `Shell` | Construct PowerShell shell (`flag="-Command"`, resolves `pwsh` or `powershell.exe`) | `os.try_powershell` |
| `ppid` | `os.ppid()` | `int` | Get parent process ID | `os.try_ppid` |
| `setenv` | `os.setenv(name, value)` | `None` | Set environment variable | `os.try_setenv` |
| `sh` | `os.sh(...)` | `Shell` | Construct `/bin/sh` shell (`flag="-c"`) | `os.try_sh` |
| `shell` | `os.shell(command=None, flag=None, cwd=None, env=None, timeout="60s", userid=None, groupid=None)` | `Shell` | Construct a configured shell execution object | `os.try_shell` |
| `temp_dir` | `os.temp_dir()` | `string` | Get default temporary directory path (`/tmp` on POSIX, `%TEMP%` on Windows) | `os.try_temp_dir` |
| `userid` | `os.userid()` | `string` | Get current user ID | `os.try_userid` |
| `username` | `os.username()` | `string` | Get current username | `os.try_username` |
| `which` | `os.which(name)` | `string`/`None` | Find executable on PATH | `os.try_which` |
| `zsh` | `os.zsh(...)` | `Shell` | Construct `/bin/zsh` shell (`flag="-c"`) | `os.try_zsh` |

### Module `regexp`

regexp provides regular expression functions: match, find, find_all, replace, split, compile

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `compile` | `regexp.compile(pattern, flags="")` | `Pattern` | Compile a pattern for repeated use | `regexp.try_compile` |
| `find` | `regexp.find(pattern, s)` | `Match` / `None` | Find the first match, or `None` | `regexp.try_find` |
| `find_all` | `regexp.find_all(pattern, s, n=-1)` | `list[Match]` | Find all matches; `n` limits the count (-1 = all) | `regexp.try_find_all` |
| `match` | `regexp.match(pattern, s)` | `bool` | Test if `pattern` matches anywhere in `s` | `regexp.try_match` |
| `replace` | `regexp.replace(pattern, s, repl)` | `string` | Replace all matches of `pattern` in `s` with `repl` | `regexp.try_replace` |
| `split` | `regexp.split(pattern, s, n=-1)` | `list` | Split `s` by `pattern`; `n` limits the number of splits (-1 = all) | `regexp.try_split` |

### Module `retry`

retry provides retry functionality: do, with_backoff

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `do` | `retry.do(func, max_attempts=3, delay="1s", retry_on=None, on_retry=None, timeout="", jitter=False)` | `RetryResult` | Retry `func` with fixed delay between attempts | `retry.try_do` |
| `with_backoff` | `retry.with_backoff(func, max_attempts=5, delay="500ms", max_delay="30s", retry_on=None, on_retry=None, timeout="", jitter=True)` | `RetryResult` | Retry `func` with exponential backoff | `retry.try_with_backoff` |

### Module `runtime`

runtime provides platform and runtime information: platform, arch, cpu_count, uname

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `arch` | `runtime.arch()` | `string` | CPU architecture (e.g. `"amd64"`, `"arm64"`) | `runtime.try_arch` |
| `cpu_count` | `runtime.cpu_count()` | `int` | Number of logical CPUs | `runtime.try_cpu_count` |
| `go_version` | `runtime.go_version()` | `string` | Go runtime version | `runtime.try_go_version` |
| `platform` | `runtime.platform()` | `string` | Operating system name (e.g. `"linux"`, `"darwin"`, `"windows"`) | `runtime.try_platform` |
| `uname` | `runtime.uname()` | `dict` | System information dictionary with keys: `system`, `node`, `release`, `version`, `machine` (Unix only; returns an error on Windows) | `runtime.try_uname` |
| `version` | `runtime.version()` | `string` | Starkite version string | `runtime.try_version` |

### Module `sandbox`

sandbox provides programmatic execution isolation and container/native sandboxing

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `config` | `sandbox.config()` | `None` | - | `sandbox.try_config` |
| `default_driver` | `sandbox.default_driver()` | `None` | - | `sandbox.try_default_driver` |
| `list_drivers` | `sandbox.list_drivers()` | `None` | - | `sandbox.try_list_drivers` |
| `run_script` | `sandbox.run_script()` | `None` | - | `sandbox.try_run_script` |

### Module `sql`

sql provides database connectivity: open() returns a connection for queries, execs, and transactions

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `open` | `sql.open()` | `None` | - | `sql.try_open` |
| `stmt` | `sql.stmt()` | `None` | - | `sql.try_stmt` |

### Module `ssh`

ssh provides SSH remote execution: config() returns a client for remote commands

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `add_known_host` | `client.add_known_host(key, host=None, ...)` | `None` | Append host key entry for target host to known_hosts | `ssh.try_add_known_host` |
| `check_authorized_key` | `client.check_authorized_key(key, ...)` | `None` | Probe remote hosts for public key authorization without logging in (alias: `client.key_check`) | `ssh.try_check_authorized_key` |
| `config` | `ssh.config()` | `None` | - | `ssh.try_config` |
| `copy_id` | `client.copy_id(key, ...)` | `None` | Install public key into target hosts' `authorized_keys` | `ssh.try_copy_id` |
| `exec` | `client.exec(cmd, ...)` | `None` | Execute single command or pipeline across target hosts | `ssh.try_exec` |
| `find_known_hosts` | `client.find_known_hosts(host=None, ...)` | `None` | Query entries matching target hosts in known_hosts file | `ssh.try_find_known_hosts` |
| `key_check` | `ssh.key_check()` | `None` | - | `ssh.try_key_check` |
| `keygen` | `ssh.keygen()` | `None` | - | `ssh.try_keygen` |
| `keyscan` | `ssh.keyscan()` | `None` | - | `ssh.try_keyscan` |
| `remove_known_host` | `client.remove_known_host(host=None, ...)` | `None` | Evict stale host key entries matching target hosts | `ssh.try_remove_known_host` |
| `scan_host_keys` | `client.scan_host_keys(...)` | `None` | Discover and inspect public host keys across target hosts (alias: `client.keyscan`) | `ssh.try_scan_host_keys` |

### Module `strings`

strings provides unique string functions: ljust, rjust, center, cut, equal, has_any, quote, unquote

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `center` | `strings.center(s, width, fillchar=" ")` | `string` | Center `s` in a field of `width`, padding with `fillchar` | `strings.try_center` |
| `cut` | `strings.cut(s, sep)` | `tuple` | Cut `s` at the first occurrence of `sep`, returning `(before, after, found)` | `strings.try_cut` |
| `equal` | `strings.equal(s, t)` | `bool` | Case-insensitive string equality | `strings.try_equal` |
| `has_any` | `strings.has_any(s, chars)` | `bool` | Return `True` if `s` contains any character in `chars` | `strings.try_has_any` |
| `ljust` | `strings.ljust(s, width, fillchar=" ")` | `string` | Left-justify `s` in a field of `width`, padding with `fillchar` | `strings.try_ljust` |
| `quote` | `strings.quote(s)` | `string` | Add quoting to `s` (Go-style quoted string) | `strings.try_quote` |
| `rjust` | `strings.rjust(s, width, fillchar=" ")` | `string` | Right-justify `s` in a field of `width`, padding with `fillchar` | `strings.try_rjust` |
| `unquote` | `strings.unquote(s)` | `string` | Remove quoting from `s` | `strings.try_unquote` |

### Module `table`

table provides table formatting: new, print

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `new` | `table.new(headers)` | `TableValue` | Create a new table with the given column headers | `table.try_new` |
| `print` | `table.print(tbl)` | `None` | Print a table to stdout | `table.try_print` |

### Module `template`

template provides text/template rendering: text, file, bytes, render

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `bytes` | `template.bytes(data, delims=())` | `template` | Create a template from raw bytes | `template.try_bytes` |
| `file` | `template.file(path, delims=())` | `template` | Create a template from a file | `template.try_file` |
| `render` | `template.render(template_str, data)` | `string` | Render a template string with `data` in one call | `template.try_render` |
| `text` | `template.text(template_str, delims=())` | `template` | Create a template from a string | `template.try_text` |

### Module `test`

test provides test utilities: skip, fail, assert helpers

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `assert` | `test.assert(condition, msg?, *args)` | `None` | Assert that `condition` is truthy | `test.try_assert` |
| `assert_contains` | `test.assert_contains(haystack, needle, msg?)` | `None` | Assert that `haystack` contains `needle` | `test.try_assert_contains` |
| `assert_equal` | `test.assert_equal(actual, expected, msg?)` | `None` | Assert that `actual == expected` | `test.try_assert_equal` |
| `assert_false` | `test.assert_false(value, msg?)` | `None` | Assert that `value` is `False` | `test.try_assert_false` |
| `assert_not_equal` | `test.assert_not_equal(actual, unexpected, msg?)` | `None` | Assert that `actual != unexpected` | `test.try_assert_not_equal` |
| `assert_true` | `test.assert_true(value, msg?)` | `None` | Assert that `value` is `True` | `test.try_assert_true` |
| `fail` | `test.fail(msg)` | `None` | Fail the current test with a message | `test.try_fail` |
| `skip` | `test.skip(reason?)` | `None` | Skip the current test with an optional reason | `test.try_skip` |

### Module `time`

time provides time functions: now, sleep, parse, format, duration

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `DateOnly` | `time.DateOnly` | `None` | Date-only layout | `time.try_DateOnly` |
| `DateTime` | `time.DateTime` | `None` | Date and time layout | `time.try_DateTime` |
| `Kitchen` | `time.Kitchen` | `None` | Kitchen time layout | `time.try_Kitchen` |
| `RFC1123` | `time.RFC1123` | `None` | RFC 1123 layout | `time.try_RFC1123` |
| `RFC1123Z` | `time.RFC1123Z()` | `None` | - | - |
| `RFC3339` | `time.RFC3339` | `None` | RFC 3339 layout | `time.try_RFC3339` |
| `RFC3339Nano` | `time.RFC3339Nano` | `None` | RFC 3339 with nanoseconds | `time.try_RFC3339Nano` |
| `RFC822` | `time.RFC822` | `None` | RFC 822 layout | `time.try_RFC822` |
| `RFC822Z` | `time.RFC822Z()` | `None` | - | - |
| `RFC850` | `time.RFC850()` | `None` | - | - |
| `Stamp` | `time.Stamp()` | `None` | - | - |
| `TimeOnly` | `time.TimeOnly` | `None` | Time-only layout | `time.try_TimeOnly` |
| `duration` | `time.duration(value)` | `DurationValue` | Parse a duration string (e.g. `"1h30m"`, `"500ms"`) | `time.try_duration` |
| `format` | `time.format(time, layout)` | `string` | Format a `TimeValue` using the given layout | `time.try_format` |
| `now` | `time.now()` | `TimeValue` | Get the current time | `time.try_now` |
| `parse` | `time.parse(layout, value)` | `TimeValue` | Parse a time string using the given layout | `time.try_parse` |
| `since` | `time.since(time)` | `DurationValue` | Duration elapsed since `time` | `time.try_since` |
| `sleep` | `time.sleep(duration)` | `None` | Sleep for the given duration (e.g. `"5s"`, `"100ms"`) | `time.try_sleep` |
| `until` | `time.until(time)` | `DurationValue` | Duration remaining until `time` | `time.try_until` |

### Module `uuid`

uuid provides UUID generation: v4

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `v4` | `uuid.v4()` | `string` | Generate a random UUID v4 | `uuid.try_v4` |

### Module `vars`

vars provides variable access: var_str(name, default) reads from the VarStore

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `var_bool` | `vars.var_bool(name, default=False)` | `bool` | Get a boolean variable | `vars.try_var_bool` |
| `var_dict` | `vars.var_dict(name, default={})` | `dict` | Get a dict variable | `vars.try_var_dict` |
| `var_float` | `vars.var_float(name, default=0.0)` | `float` | Get a float variable | `vars.try_var_float` |
| `var_int` | `vars.var_int(name, default=0)` | `int` | Get an integer variable | `vars.try_var_int` |
| `var_list` | `vars.var_list(name, default=[])` | `list` | Get a list variable | `vars.try_var_list` |
| `var_names` | `vars.var_names()` | `list` | List all defined variable names | `vars.try_var_names` |
| `var_str` | `vars.var_str(name, default="")` | `string` | Get a string variable | `vars.try_var_str` |

### Module `yaml`

yaml provides YAML file reading/writing: file, from

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `decode` | `yaml.decode(string)` | `any` | Decode a YAML string to a value | `yaml.try_decode` |
| `decode_all` | `yaml.decode_all(string)` | `list` | Decode a multi-document YAML string to a list of values | `yaml.try_decode_all` |
| `encode` | `yaml.encode(value)` | `string` | Encode a value to a YAML string | `yaml.try_encode` |
| `encode_all` | `yaml.encode_all(list)` | `string` | Encode a list of values to a multi-document YAML string | `yaml.try_encode_all` |
| `file` | `yaml.file(path)` | `yaml.file` | Create a file object for reading YAML | `yaml.try_file` |
| `source` | `yaml.source(data)` | `yaml.writer` | Create a writer object from data | `yaml.try_source` |

#### Exported Type `yaml.file` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `decode` | `f.decode()` | `any` | Decode the first (or only) document |
| `decode_all` | `f.decode_all()` | `list` | Decode all documents in the file |
| `path` | `f.path` | `string` | The file path |
| `try_decode` | `f.try_decode()` | `Result` | Decode the file, returning a Result |
| `try_decode_all` | `f.try_decode_all()` | `Result` | Decode all documents, returning a Result |

### Module `zip`

zip provides ZIP archive operations: file

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `file` | `zip.file(path)` | `zip.archive` | Open or create a ZIP archive at the given path | `zip.try_file` |

