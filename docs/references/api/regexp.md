---
title: "regexp"
description: "Regular expression matching, searching, and replacement"
weight: 13
---

The `regexp` module provides regular expression support using Go's `regexp` engine (RE2 syntax).

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `regexp.match(pattern, s)` | `bool` | Test if `pattern` matches anywhere in `s` |
| `regexp.find(pattern, s)` | `Match` / `None` | Find the first match, or `None` |
| `regexp.find_all(pattern, s, n=-1)` | `list[Match]` | Find all matches; `n` limits the count (-1 = all) |
| `regexp.replace(pattern, s, repl)` | `string` | Replace all matches of `pattern` in `s` with `repl` |
| `regexp.split(pattern, s, n=-1)` | `list` | Split `s` by `pattern`; `n` limits the number of splits (-1 = all) |
| `regexp.compile(pattern, flags="")` | `Pattern` | Compile a pattern for repeated use |

All functions accept an optional `flags` keyword argument.

### Flags

| Flag | Description |
|------|-------------|
| `i` | Case-insensitive matching |
| `m` | Multi-line mode: `^` and `$` match line boundaries |
| `s` | Let `.` match `\n` |
| `U` | Ungreedy: swap meaning of `x*` and `x*?` |

Flags can be combined: `flags="im"`.

## Pattern

A compiled pattern object with the same methods as the module-level functions (without the `pattern` parameter):

| Method | Returns | Description |
|--------|---------|-------------|
| `match(s)` | `bool` | Test if this pattern matches anywhere in `s` |
| `find(s)` | `Match` / `None` | Find the first match |
| `find_all(s, n=-1)` | `list[Match]` | Find all matches |
| `replace(s, repl)` | `string` | Replace all matches in `s` |
| `split(s, n=-1)` | `list` | Split `s` by this pattern |

## Match

| Attribute / Method | Type | Description |
|--------------------|------|-------------|
| `text` | `string` | The matched text |
| `start` | `int` | Start index of the match |
| `end` | `int` | End index of the match |
| `groups` | `tuple` | Captured groups |
| `group(n=0, name="")` | `string` | Get a group by index or name |

## Examples

### Simple matching

```python
if regexp.match(r"\d+", "order-42"):
    print("contains a number")
```

### Find and extract

```python
m = regexp.find(r"(\w+)@(\w+\.\w+)", "user@example.com")
if m:
    print(m.text)       # user@example.com
    print(m.group(1))   # user
    print(m.group(2))   # example.com
```

### Find all matches

```python
matches = regexp.find_all(r"\b\w{4}\b", "the quick brown fox")
for m in matches:
    print(m.text)  # quick, brown (4-letter words... well, check your regex)
```

### Replace

```python
result = regexp.replace(r"\d+", "item-42-v3", "X")
print(result)  # item-X-vX
```

### Split

```python
parts = regexp.split(r"[,;\s]+", "a, b; c  d")
print(parts)  # ["a", "b", "c", "d"]
```

### Compiled pattern with flags

```python
pat = regexp.compile(r"error:\s*(.+)", flags="i")
m = pat.find("ERROR: disk full")
if m:
    print(m.group(1))  # disk full
```

### Case-insensitive matching

```python
if regexp.match(r"hello", "HELLO WORLD", flags="i"):
    print("matched!")
```

> **Note:**
All `regexp` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

