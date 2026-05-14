---
title: "uuid"
description: "UUID generation"
weight: 16
---

The `uuid` module generates universally unique identifiers.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `uuid.v4()` | `string` | Generate a random UUID v4 |

## Examples

### Generate a UUID

```python
id = uuid.v4()
print(id)  # e.g. "f47ac10b-58cc-4372-a567-0e02b2c3d479"
```

### Use as a unique identifier

```python
job_id = uuid.v4()
print("Starting job:", job_id)
exec("make build")
print("Completed job:", job_id)
```
