---
title: "Language"
description: "The Starlark language, the main() entry point, and the try_ error pattern"
weight: 40
---

# Language

Starkite scripts are written in Starlark — a deterministic, Python-derived language designed to run the same way every time, with no hidden state and no surprises. On top of that base, Starkite adds two conventions you will use in nearly every script: an automatic `main()` entry point that gives a script a clear place to start, and a `try_` prefix that turns failures you want to handle into ordinary values instead of crashes. This page covers those two conventions; for the syntax and semantics of Starlark itself, see the upstream [Starlark spec](https://github.com/bazelbuild/starlark/blob/master/spec.md), and for how a script receives its inputs, see [Configuration](configuration.md).

## Entry point

A script needs a place to begin, and Starkite gives it one without ceremony. Define a function named `main`, and after the script's top-level code finishes running, the runtime calls `main()` for you:

```python
def main():
    print("hello")
```

Running `kite run ./hello.star` prints `hello`. You never write the call yourself — defining `main` is enough to make it the entry point.

Defining `main` is optional. A script that does not define it simply runs top to bottom, with all of its work at the top level. Use `main` when you want a named starting point; skip it when a short script reads more clearly as a straight sequence of statements.

What happens if you both define `main` and call it yourself at the top level? The runtime notices the explicit call and declines to run `main` a second time, so your script does not execute twice. It records a notice on stderr so the skipped automatic invocation is never silent:

```
level=INFO msg="skipping automatic entry-point invocation: script calls it at top level" entrypoint=main script=hello.star
```

This detection is syntactic: the runtime recognizes a direct top-level call written as `main()`. A call hidden behind an alias or buried inside control flow is not detected, and `main` would then run twice. If you call the entry point yourself, call it plainly so the runtime can see it.

The automatic call applies only to the script you launch. A `main` defined in a module pulled in with `load()` is never invoked automatically — it is just another function for the importing script to use. Because the runtime supplies no arguments, `main` must be callable with none; a script's inputs arrive through the [variable-injection system](configuration.md) rather than through parameters.

## Error handling

Many operations a script performs can fail — a file may be missing, a host unreachable, an API down. By default those failures raise and stop the script, which is the right behavior when there is nothing to do but abort. When you want to inspect a failure and decide what to do next, reach for the `try_` variant: every Starkite function that can fail has one, and instead of raising it returns a `Result` you examine. The `Result` type carries three attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | bool | `True` if the operation succeeded |
| `value` | any | Return value on success |
| `error` | string | Error message on failure |

### The try_ pattern

The two styles sit side by side: call the plain function when a failure should halt the script, and the `try_` variant when you want to branch on the outcome. The example below reads one file each way:

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

The first call aborts the script if `/etc/hosts` cannot be read; the second never raises, leaving you to check `result.ok` and pull the value or the error message yourself. Note that the branching lives inside `main()`: Starlark allows `if` and `for` only inside a function, so any error-handling logic belongs in a `def`, never at the top level.

### Constructing results

You can also build a `Result` of your own, which is what you need when writing a check function for `retry`. The `Result()` built-in constructs one from explicit `ok`, `value`, and `error` fields:

```python
def check_service():
    resp = http.url("http://localhost:8080/health").try_get()
    if resp.ok and resp.value.status_code == 200:
        return Result(ok=True, value="healthy")
    return Result(ok=False, error="unhealthy")

result = retry.do(check_service, max_attempts=5, delay="2s")
```

Here `check_service` reports its own outcome as a `Result`, and `retry.do` reads that `ok` field to decide whether to try again — succeeding when the health check returns a 200, and retrying up to five times with a two-second delay otherwise.

### Object method variants

The `try_` prefix is not limited to top-level functions. Methods on the objects you build — files, paths, HTTP requests — carry the same variant, so you can guard a call at the exact point it might fail:

```python
# File objects
config = json.file("config.json").try_decode()

# Path objects
data = fs.path("/tmp/data.txt").try_read_text()

# HTTP
page = http.url("https://api.example.com/data").try_get()
```

Each of these returns a `Result` with the same `ok` / `value` / `error` shape, so you handle a failed decode, read, or request exactly as you would a failed function call.

### Module-level factories

One subtlety is worth knowing when you decide where to put the `try_`. Factory functions have `try_` variants too, but a factory only constructs the object — it does not touch the file, open the connection, or send the request. A missing path therefore goes unreported until the read that actually reaches for it. Guard the operation, not the factory:

```python
def main():
    result = json.file("maybe-missing.json").try_decode()
    if result.ok:
        data = result.value
    else:
        print("Error:", result.error)
```

`json.file(...)` here always succeeds because it merely names the file; the failure for a missing or malformed file surfaces at `try_decode()`, which is where the `Result` you check comes from.
