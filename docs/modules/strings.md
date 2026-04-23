---
title: "strings"
description: "String utility functions beyond Starlark builtins"
weight: 12
---

The `strings` module provides string utility functions that complement Starlark's built-in string methods. Functions that duplicate Starlark builtins have been removed in favor of the native equivalents.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `strings.ljust(s, width, fillchar=" ")` | `string` | Left-justify `s` in a field of `width`, padding with `fillchar` |
| `strings.rjust(s, width, fillchar=" ")` | `string` | Right-justify `s` in a field of `width`, padding with `fillchar` |
| `strings.center(s, width, fillchar=" ")` | `string` | Center `s` in a field of `width`, padding with `fillchar` |
| `strings.cut(s, sep)` | `tuple` | Cut `s` at the first occurrence of `sep`, returning `(before, after, found)` |
| `strings.equal(s, t)` | `bool` | Case-insensitive string equality |
| `strings.has_any(s, chars)` | `bool` | Return `True` if `s` contains any character in `chars` |
| `strings.quote(s)` | `string` | Add quoting to `s` (Go-style quoted string) |
| `strings.unquote(s)` | `string` | Remove quoting from `s` |

## Use Starlark Builtins Instead

The following operations are handled directly by Starlark's built-in string methods. There is no need to use a module function:

| Operation | Starlark Builtin |
|-----------|-----------------|
| Contains | `sub in s` |
| Upper/Lower | `s.upper()` / `s.lower()` |
| Split | `s.split(sep)` |
| Join | `sep.join(parts)` |
| Replace | `s.replace(old, new)` |
| Strip | `s.strip()` / `s.lstrip()` / `s.rstrip()` |
| Starts/Ends with | `s.startswith(prefix)` / `s.endswith(suffix)` |

## Examples

### Padding and alignment

```python
name = "kite"
print(strings.ljust(name, 20))        # "kite                "
print(strings.rjust(name, 20))        # "                kite"
print(strings.center(name, 20, "-"))   # "--------kite--------"
```

### Cutting a string

```python
before, after, found = strings.cut("host:8080", ":")
print(before)  # "host"
print(after)   # "8080"
print(found)   # True

_, _, found = strings.cut("no-colon", ":")
print(found)  # False
```

### Case-insensitive comparison

```python
if strings.equal("Hello", "hello"):
    print("match!")  # prints
```

### Character membership

```python
if strings.has_any("hello!", "!@#$"):
    print("has special characters")
```

### Quoting

```python
quoted = strings.quote('hello "world"')
print(quoted)  # "hello \"world\""

original = strings.unquote(quoted)
print(original)  # hello "world"
```

> **Note:**
All `strings` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

