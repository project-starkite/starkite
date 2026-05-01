---
title: "Permissions"
description: "Trust mode and permission profiles"
weight: 2
---

Starkite provides permission profiles to control what operations scripts can perform.

## Trust Mode (Default)

When `--permissions` is not set, scripts run in trust mode and may perform any operation.

```bash
kite script.star          # trust mode (default)
```

## Strict Profile

The `strict` profile restricts scripts to safe operations only. Dangerous operations like `exec()`, file writes, and network access are blocked.

```bash
kite script.star --permissions=strict
```

### Allowed under `strict`

- String manipulation (`strings.*`)
- JSON/YAML encoding/decoding (in-memory)
- Math and logic operations
- Time functions
- UUID generation
- Template rendering (in-memory)

### Blocked under `strict`

- Command execution (`exec()`, `os.exec()`)
- File I/O (`path().write_text()`, `path().remove()`)
- Network access (`http.*`, `ssh.*`)
- Process operations (`os.exit()`)

### Permission Errors

When a blocked operation is attempted under `--permissions=strict`:

```python
# Under --permissions=strict:
exec("echo hello")
# Error: permission denied: os.exec is not allowed
```
