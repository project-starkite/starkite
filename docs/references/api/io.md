---
title: "io"
description: "Interactive user input: prompts and confirmations"
weight: 22
---

The `io` module provides interactive user input functions for CLI scripts.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `io.confirm(msg, default=False)` | `bool` | Prompt the user for a yes/no confirmation |
| `io.prompt(msg, default="", secret=False)` | `string` | Prompt the user for text input. Set `secret=True` to hide input (e.g. passwords) |

## Examples

### Confirmation prompt

```python
if io.confirm("Deploy to production?"):
    exec("kubectl apply -f prod.yaml")
else:
    print("Aborted.")
```

### Confirmation with default

```python
# Default to True — pressing Enter confirms
if io.confirm("Continue?", default=True):
    print("Continuing...")
```

### Text prompt

```python
name = io.prompt("Enter cluster name")
print("Deploying to:", name)
```

### Prompt with default value

```python
region = io.prompt("AWS region", default="us-east-1")
print("Using region:", region)
```

### Secret input

```python
token = io.prompt("API token", secret=True)
os.setenv("API_TOKEN", token)
```

## io.reader

An `io.reader` represents a readable stream of data, exposing the following methods:

| Method | Returns | Description |
|--------|---------|-------------|
| `read(count)` | `bytes` | Reads up to `count` bytes from the stream, returning them as a `bytes` object (empty on EOF) |
| `bytes()` | `bytes` | Buffers all remaining content of the stream into host memory, returns it as a single `bytes` object, and automatically closes the stream |
| `lines()` | `iterator` | Returns a Starlark iterator yielding lines of text sequentially (trailing newlines are kept) |
| `close()` | `None` | Closes the underlying stream, releasing the OS resource |

### Example: Consuming a Stream Line-by-Line

```python
# Assuming reader is obtained from a file path or HTTP response
reader = fs.path("app.log").get_reader()

for line in reader.lines():
    if "ERROR" in line:
        print("Found error:", line.strip())

reader.close() # Explicitly release the resource
```

---

## io.writer

An `io.writer` represents a writable stream of data, exposing the following methods:

| Method | Returns | Description |
|--------|---------|-------------|
| `write(data)` | `int` | Writes data to the stream. Accepts a `string`, `bytes`, or another `io.reader` (which automatically pipes the reader's content into the writer and closes the reader on completion) |
| `close()` | `None` | Flushes any remaining buffered bytes and closes the stream |

### Example: Streaming a File

```python
# Stream content from one file to another using zero-copy piping
reader = fs.path("source.dat").get_reader()
writer = fs.path("destination.dat").get_writer()

# Writes all content and automatically closes the reader
writer.write(reader)

writer.close() # Close the writer to flush and sync to disk
```

---

> **Note:**
All `io` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

