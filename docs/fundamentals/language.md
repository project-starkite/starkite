---
title: "Language"
description: "The Starlark language, the main() entry point, and the try_ error pattern"
weight: 40
---

# Language

Starkite scripts are Starlark — a deterministic, Python-derived language — extended with two conventions used across every module: an automatic `main()` entry point and the `try_` prefix for error handling. Script inputs come from the variable-injection system — see [Configuration](configuration.md). For Starlark's syntax and semantics, see the upstream [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md).

## Entry point

A script may define a `main` function as its entry point. After the script's top-level code runs, the runtime calls `main()` automatically:

```python
def main():
    print("hello")
```

`kite run ./hello.star` prints `hello` — no explicit call needed.

Defining `main` is optional. A script that does not define it runs entirely at the top level.

If a script both defines `main` and calls it at the top level, the runtime detects the explicit call and does not call `main` a second time. It records a notice on stderr so the skipped invocation is visible:

```
level=INFO msg="skipping automatic entry-point invocation: script calls it at top level" entrypoint=main script=hello.star
```

The detection is syntactic — it recognizes a direct top-level call such as `main()`. A call reached through an alias or nested inside control flow is not detected and would run `main` twice.

Automatic invocation applies only to the entry script. A `main` defined in a module loaded with `load()` is never called automatically. `main` must be callable with no arguments; script inputs come from the [variable-injection system](configuration.md).

## Error handling

Every starkite function that can fail has a `try_` variant that returns a `Result` instead of raising. The `Result` type has three attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | bool | `True` if the operation succeeded |
| `value` | any | Return value on success |
| `error` | string | Error message on failure |

### The try_ pattern

```python
def main():
    # Without try_ — raises on failure
    content = read_text("/etc/hosts")

    # With try_ — returns a Result instead of raising
    result = fs.path("/etc/missing").try_read_text()
    if result.ok:
        print(result.value)
    else:
        print("Error:", result.error)
```

Starlark allows `if`/`for` only inside a function, so error-handling logic lives in `main()` (or any `def`), not at the top level.

### Constructing results

The `Result()` built-in constructs Result values for use with `retry`:

```python
def check_service():
    resp = http.url("http://localhost:8080/health").try_get()
    if resp.ok and resp.value.status_code == 200:
        return Result(ok=True, value="healthy")
    return Result(ok=False, error="unhealthy")

result = retry.do(check_service, max_attempts=5, delay="2s")
```

### Object method variants

Objects support `try_` on their methods:

```python
# File objects
config = json.file("config.json").try_decode()

# Path objects
data = fs.path("/tmp/data.txt").try_read_text()

# HTTP
page = http.url("https://api.example.com/data").try_get()
```

Each call returns a `Result` with `ok` / `value` / `error`.

### Module-level factories

Factory functions have `try_` variants, but a factory only builds the object — it does not touch the file, so a missing path is not reported until the read. Guard the read, not the factory:

```python
def main():
    result = json.file("maybe-missing.json").try_decode()
    if result.ok:
        data = result.value
    else:
        print("Error:", result.error)
```
