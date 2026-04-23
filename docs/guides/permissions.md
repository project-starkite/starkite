---
title: "Permissions"
description: "Trust and sandbox modes"
weight: 2
---

Starkite provides two permission modes to control what operations scripts can perform.

## Trust Mode (Default)

Trust mode allows all operations. This is the default.

```bash
kite script.star          # trust mode (default)
kite script.star --trust  # explicit trust mode
```

## Sandbox Mode

Sandbox mode restricts scripts to safe operations only. Dangerous operations like `exec()`, file writes, and network access are blocked.

```bash
kite script.star --sandbox
```

### Allowed in Sandbox

- String manipulation (`strings.*`)
- JSON/YAML encoding/decoding (in-memory)
- Math and logic operations
- Time functions
- UUID generation
- Template rendering (in-memory)

### Blocked in Sandbox

- Command execution (`exec()`, `os.exec()`)
- File I/O (`path().write_text()`, `path().remove()`)
- Network access (`http.*`, `ssh.*`)
- Process operations (`os.exit()`)

### Permission Errors

When a sandbox-blocked operation is attempted:

```python
# In sandbox mode:
exec("echo hello")
# Error: permission denied: os.exec is not allowed in sandbox mode
```
