---
title: "Language"
description: "The Starlark language, the main() entry point, and the try_ error pattern"
weight: 40
---

# Language

Starkite scripts are written in Starlark, a deterministic, Python-derived language. While inheriting the core syntax of Starlark, Starkite introduces two primary execution conventions: an automatic `main()` entry point for structured execution and the `try_` error-handling pattern to handle script failures safely without raising runtime exceptions.

For details on the core syntax and semantics of the base Starlark language, refer to the [Starlark Specification](https://github.com/bazelbuild/starlark/blob/master/spec.md).

## Entry point

By convention, if you define a function named `main()`, the `kite` runtime automatically executes it after the script's top-level statements run:

```python
def main():
    print("Hello from main!")
```

Running `kite run ./hello.star` executes `main()` automatically. 

Defining `main()` is optional. Short scripts can execute statements sequentially at the top level without a `main()` definition.

### Syntactic detection of manual calls

If a script defines `main()` and explicitly invokes it at the top level (e.g., `main()`), the runtime detects the call and skips automatic execution to prevent duplicate runs. It outputs an information message to standard error:

```
level=INFO msg="skipping automatic entry-point invocation: script calls it at top level" entrypoint=main script=hello.star
```

This detection looks specifically for direct, top-level `main()` call expressions. Indirect invocations (such as calling an alias of `main` or invoking it within custom control flow) are not detected and may lead to duplicate executions.

### Modules and inputs

*   **Scope**: Automatic invocation only applies to the entry script. A `main()` function defined inside a module loaded via `load()` is never executed automatically.
*   **Arguments**: The automatic entry-point execution passes no arguments. Inputs must be injected via the [variable-injection system](configuration.md) instead of function parameters.

## Error handling

By default, file, database, or network errors raise runtime exceptions and halt script execution. To inspect errors and perform conditional handling without crashing, Starkite provides a `try_` variant for every built-in function and method that can fail.

Instead of raising exceptions, `try_` functions return a `Result` object:

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | `bool` | `True` if the operation succeeded |
| `value` | `any` | The function's return value on success |
| `error` | `string` | The error message on failure |

### Basic usage

```python
def main():
    # Plain function call: raises an exception and aborts on failure
    content = read_text("/etc/hosts")

    # try_ variant: returns a Result object instead of raising an exception
    result = fs.path("/etc/missing").try_read_text()
    if result.ok:
        print(result.value)
    else:
        print("Error:", result.error)
```

> [!IMPORTANT]
> Starlark restricts loop (`for`) and conditional (`if`) statements to function bodies. All error handling and branching logic must reside inside a defined function (like `main()`).

### Object method variants

The `try_` naming prefix applies to object methods as well:

```python
# File objects
config = json.file("config.json").try_decode()

# Path objects
data = fs.path("/tmp/data.txt").try_read_text()

# HTTP requests
page = http.url("https://api.example.com/data").try_get()
```

### Custom Result construction

For modules like `retry` that evaluate script states, you can construct custom `Result` instances using the `Result()` constructor:

```python
def check_service():
    resp = http.url("http://localhost:8080/health").try_get()
    if resp.ok and resp.value.status_code == 200:
        return Result(ok=True, value="healthy")
    return Result(ok=False, error="unhealthy")

# retry.do evaluates the 'ok' attribute of the returned Result
result = retry.do(check_service, max_attempts=5, delay="2s")
```

### Factory functions vs. execution methods

Built-in factory functions (like `json.file()` or `http.url()`) only initialize configuration metadata on the host—they do not perform file I/O or network calls directly. Consequently, these factory initializers always succeed, and any failures (such as missing files or DNS resolution errors) will only surface when executing the subsequent methods:

```python
def main():
    # json.file() configures the path, while try_decode() handles the actual parse I/O
    result = json.file("maybe-missing.json").try_decode()
    if result.ok:
        data = result.value
    else:
        print("Error:", result.error)
```
