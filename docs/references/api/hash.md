---
title: "hash"
description: "Cryptographic hash functions for text, bytes, and files"
weight: 11
---

The `hash` module provides cryptographic hash functions (MD5, SHA-1, SHA-256, SHA-512) for text, bytes, and files.

## Factories

| Factory | Returns | Description |
|---------|---------|-------------|
| `hash.file(path)` | `hash.file` | Create a hash source from a file |
| `hash.text(s)` | `hash.source` | Create a hash source from a string |
| `hash.bytes(data)` | `hash.source` | Create a hash source from raw bytes |

## Methods

Both `hash.source` and `hash.file` share the same methods:

| Method | Returns | Description |
|--------|---------|-------------|
| `md5()` | `string` | Compute MD5 hex digest |
| `sha1()` | `string` | Compute SHA-1 hex digest |
| `sha256()` | `string` | Compute SHA-256 hex digest |
| `sha512()` | `string` | Compute SHA-512 hex digest |

### try_ variants

| Method | Returns | Description |
|--------|---------|-------------|
| `try_md5()` | `Result` | Like `md5`, returns `Result` instead of raising |
| `try_sha1()` | `Result` | Like `sha1`, returns `Result` instead of raising |
| `try_sha256()` | `Result` | Like `sha256`, returns `Result` instead of raising |
| `try_sha512()` | `Result` | Like `sha512`, returns `Result` instead of raising |

## Examples

### Hash a string

```python
digest = hash.text("hello").sha256()
print(digest)  # 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
```

### Hash a file

```python
digest = hash.file("/usr/bin/kite").sha256()
print("SHA-256:", digest)
```

### Compare checksums

```python
expected = "d2d2d2..."
actual = hash.file("download.tar.gz").sha256()
if actual != expected:
    fail("checksum mismatch!")
```

### Multiple algorithms

```python
src = hash.text("sensitive data")
print("MD5:   ", src.md5())
print("SHA-1: ", src.sha1())
print("SHA-256:", src.sha256())
print("SHA-512:", src.sha512())
```

### Safe hashing with try_

```python
result = hash.file("/nonexistent").try_sha256()
if not result.ok:
    print("Hash error:", result.error)
```

> **Note:**
All `hash` methods that can fail support `try_` variants that return a `Result` instead of raising an error.

