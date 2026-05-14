---
title: "gzip"
description: "Gzip compression and decompression for files, text, and bytes"
weight: 8
---

The `gzip` module provides gzip compression and decompression for files, text, and raw bytes.

## Factories

| Factory | Returns | Description |
|---------|---------|-------------|
| `gzip.file(path)` | `gzip.file` | Create a gzip file handle for compress/decompress operations |
| `gzip.text(s)` | `gzip.source` | Create a gzip source from a string |
| `gzip.bytes(data)` | `gzip.source` | Create a gzip source from raw bytes |

## gzip.source

A source wraps in-memory data (text or bytes) for compression and decompression.

| Method | Returns | Description |
|--------|---------|-------------|
| `compress(dest="", level=-1)` | `bytes` | Compress the data; optionally write to `dest` file. `level` sets compression level (-1 = default) |
| `decompress(dest="")` | `bytes` | Decompress the data; optionally write to `dest` file |
| `data` | property | The underlying data |

### try_ variants

| Method | Returns | Description |
|--------|---------|-------------|
| `try_compress(dest="", level=-1)` | `Result` | Like `compress`, returns `Result` instead of raising |
| `try_decompress(dest="")` | `Result` | Like `decompress`, returns `Result` instead of raising |

## gzip.file

A file handle for compressing or decompressing gzip files on disk.

| Method | Returns | Description |
|--------|---------|-------------|
| `compress(source, level=-1)` | `None` | Compress `source` file into this gzip file |
| `decompress(dest="")` | `None` | Decompress this gzip file; optionally specify output `dest` |

### try_ variants

| Method | Returns | Description |
|--------|---------|-------------|
| `try_compress(source, level=-1)` | `Result` | Like `compress`, returns `Result` instead of raising |
| `try_decompress(dest="")` | `Result` | Like `decompress`, returns `Result` instead of raising |

## Examples

### Compress text to bytes

```python
src = gzip.text("Hello, world!")
compressed = src.compress()
print(len(compressed))  # compressed byte count
```

### Compress and write to file

```python
src = gzip.text("large content here")
src.compress(dest="/tmp/output.gz")
```

### Decompress a gzip file

```python
gz = gzip.file("/tmp/output.gz")
gz.decompress(dest="/tmp/output.txt")
```

### Round-trip with bytes

```python
original = b"binary data payload"
src = gzip.bytes(original)
compressed = src.compress(level=9)

restored = gzip.bytes(compressed).decompress()
assert_equal(original, restored)
```

### Using try_ for safe decompression

```python
result = gzip.file("/tmp/maybe.gz").try_decompress()
if result.ok:
    print("Decompressed successfully")
else:
    print("Error:", result.error)
```

> **Note:**
All `gzip` methods that can fail support `try_` variants that return a `Result` instead of raising an error.

