---
title: "vars"
description: "Typed variable access for script parameters"
weight: 23
---

The `vars` module provides typed access to script variables (parameters passed via the CLI or environment). All functions are also available as **global builtins** without the `vars.` prefix.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `vars.var_str(name, default="")` | `string` | Get a string variable |
| `vars.var_int(name, default=0)` | `int` | Get an integer variable |
| `vars.var_bool(name, default=False)` | `bool` | Get a boolean variable |
| `vars.var_float(name, default=0.0)` | `float` | Get a float variable |
| `vars.var_list(name, default=[])` | `list` | Get a list variable |
| `vars.var_dict(name, default={})` | `dict` | Get a dict variable |
| `vars.var_names()` | `list` | List all defined variable names |

## Global Builtins

All `vars` functions are also registered as top-level globals. These two calls are equivalent:

```python
# With module prefix
env = vars.var_str("environment", "dev")

# As global built-in (no prefix needed)
env = var_str("environment", "dev")
```

## Examples

### Script parameters

```python
# kite run deploy.star -- --env=prod --replicas=3 --dry-run=true

env = var_str("env", "dev")
replicas = var_int("replicas", 1)
dry_run = var_bool("dry-run", False)

print("Deploying to", env, "with", replicas, "replicas")
if dry_run:
    print("(dry run mode)")
```

### List and dict variables

```python
# kite run script.star -- --tags='["web","api"]' --labels='{"team":"platform"}'

tags = var_list("tags")
labels = var_dict("labels")

for tag in tags:
    print("Tag:", tag)
for k, v in labels.items():
    print(k, "=", v)
```

### Listing variables

```python
print("Available variables:")
for name in var_names():
    print(" ", name)
```

### Default values

```python
# All functions return the default if the variable is not set
region = var_str("region", "us-east-1")
workers = var_int("workers", 4)
verbose = var_bool("verbose", False)
threshold = var_float("threshold", 0.95)
```
