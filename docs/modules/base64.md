---
title: "base64"
description: "Base64 encoding and decoding for text, bytes, and files"
weight: 10
---

The `base64` module provides standard and URL-safe Base64 encoding and decoding.

## Factories

| Factory | Returns | Description |
|---------|---------|-------------|
| `base64.file(path)` | `base64.file` | Create a Base64 handle for a file |
| `base64.text(s)` | `base64.source` | Create a Base64 source from a string |
| `base64.bytes(data)` | `base64.source` | Create a Base64 source from raw bytes |

## base64.source

| Method | Returns | Description |
|--------|---------|-------------|
| `encode()` | `string` | Encode data to standard Base64 |
| `decode()` | `bytes` | Decode Base64 string to bytes |
| `encode_url()` | `string` | Encode data to URL-safe Base64 |
| `decode_url()` | `bytes` | Decode URL-safe Base64 to bytes |

### try_ variants

| Method | Returns | Description |
|--------|---------|-------------|
| `try_encode()` | `Result` | Like `encode`, returns `Result` instead of raising |
| `try_decode()` | `Result` | Like `decode`, returns `Result` instead of raising |
| `try_encode_url()` | `Result` | Like `encode_url`, returns `Result` instead of raising |
| `try_decode_url()` | `Result` | Like `decode_url`, returns `Result` instead of raising |

## base64.file

The file type has the same methods as `base64.source`, plus a `path` property.

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `encode()` | `string` | Encode file contents to standard Base64 |
| `decode()` | `bytes` | Decode Base64 file contents to bytes |
| `encode_url()` | `string` | Encode file contents to URL-safe Base64 |
| `decode_url()` | `bytes` | Decode URL-safe Base64 file contents to bytes |
| `path` | property | The file path |

All file methods also have `try_` variants.

## Examples

### Encode a string

```python
encoded = base64.text("Hello, world!").encode()
print(encoded)  # SGVsbG8sIHdvcmxkIQ==
```

### Decode a Base64 string

```python
decoded = base64.text("SGVsbG8sIHdvcmxkIQ==").decode()
print(decoded)  # b"Hello, world!"
```

### URL-safe encoding

```python
encoded = base64.bytes(b"\xff\xfe\xfd").encode_url()
print(encoded)  # URL-safe Base64 string
```

### Encode a file

```python
b64 = base64.file("/etc/hosts")
encoded = b64.encode()
print(b64.path)  # /etc/hosts
```

### Safe decoding with try_

```python
result = base64.text("not-valid-base64!!!").try_decode()
if not result.ok:
    print("Decode error:", result.error)
```

> **Note:**
All `base64` methods that can fail support `try_` variants that return a `Result` instead of raising an error.

