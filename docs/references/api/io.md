---
title: "io"
description: "Interactive user input: prompts and confirmations"
weight: 22
---

The `io` module provides interactive user input functions for CLI scripts.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `io.confirm(msg, default=False)` | `bool` | Prompt the user for a yes/no confirmation |
| `io.prompt(msg, default="", secret=False)` | `string` | Prompt the user for text input. Set `secret=True` to hide input (e.g. passwords) |

## Examples

### Confirmation prompt

```python
if io.confirm("Deploy to production?"):
    exec("kubectl apply -f prod.yaml")
else:
    print("Aborted.")
```

### Confirmation with default

```python
# Default to True — pressing Enter confirms
if io.confirm("Continue?", default=True):
    print("Continuing...")
```

### Text prompt

```python
name = io.prompt("Enter cluster name")
print("Deploying to:", name)
```

### Prompt with default value

```python
region = io.prompt("AWS region", default="us-east-1")
print("Using region:", region)
```

### Secret input

```python
token = io.prompt("API token", secret=True)
os.setenv("API_TOKEN", token)
```

> **Note:**
All `io` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

