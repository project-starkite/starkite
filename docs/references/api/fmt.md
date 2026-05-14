---
title: "fmt"
description: "Formatted printing and string formatting"
weight: 17
---

The `fmt` module provides Go-style formatted printing and string formatting.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `fmt.printf(format, *args)` | `None` | Print formatted output to stdout |
| `fmt.println(*args)` | `None` | Print arguments to stdout followed by a newline |
| `fmt.sprintf(format, *args)` | `string` | Return a formatted string |

`printf`, `println`, and `sprintf` are also available as top-level global aliases (no `fmt.` prefix required).

Format strings use Go `fmt` verbs: `%s` (string), `%d` (integer), `%f` (float), `%v` (default), `%q` (quoted string), `%%` (literal percent), etc.

## Examples

### Formatted printing

```python
fmt.printf("Hello, %s! You have %d messages.\n", "alice", 42)
```

### String formatting

```python
msg = fmt.sprintf("deploy %s to %s (replicas=%d)", "myapp", "prod", 3)
print(msg)
```

### Format verbs

```python
fmt.printf("string: %s\n", "hello")
fmt.printf("quoted: %q\n", "hello world")
fmt.printf("int:    %d\n", 42)
fmt.printf("float:  %.2f\n", 3.14159)
fmt.printf("value:  %v\n", [1, 2, 3])
fmt.printf("percent: 100%%\n")
```
