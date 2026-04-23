---
title: "Error Handling"
description: "The try_ pattern and Result type"
weight: 3
---

Starkite uses the `try_` prefix pattern for error handling. Every function that can fail has a `try_` variant that returns a `Result` instead of raising an error.

`path()` is available as a global alias for `fs.path()`; the examples below use `fs.path()` consistently, but `path()` works identically.

## The try_ Pattern

```python
# Without try_ — raises error on failure
content = read_text("/etc/hosts")

# With try_ — returns Result
result = fs.path("/etc/missing").try_read_text()
if result.ok:
    print(result.value)
else:
    print("Error:", result.error)
```

## Result Type

The `Result` type has these attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | bool | `True` if the operation succeeded |
| `value` | any | Return value on success |
| `error` | string | Error message on failure |

## Constructing Results

The `Result()` built-in can construct Result values:

```python
Result(ok=True, value="data")
Result(ok=False, error="something failed")
```

This is useful with the `retry` module:

```python
def check_service():
    resp = http.url("http://localhost:8080/health").try_get()
    if resp.ok and resp.value.status_code == 200:
        return Result(ok=True, value="healthy")
    return Result(ok=False, error="unhealthy")

result = retry.do(check_service, max_attempts=5, delay="2s")
```

## Object Method try_ Variants

Objects also support `try_` on their methods:

```python
# File objects
f = json.file("config.json")
result = f.try_decode()

# Path objects
p = fs.path("/tmp/data.txt")
result = p.try_read_text()

# HTTP
url = http.url("https://api.example.com/data")
result = url.try_get()
```

## Module-Level try_ Factories

Factory functions also have `try_` variants:

```python
# Returns Result wrapping the file object itself
result = json.try_file("maybe-missing.json")
if result.ok:
    data = result.value.decode()
```
