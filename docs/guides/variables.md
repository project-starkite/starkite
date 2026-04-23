---
title: "Variable System"
description: "Variable injection and priority resolution"
weight: 1
---

Starkite provides a 5-tier variable injection system for configuring scripts without modifying them.

## Priority Order

Variables are resolved in this order (highest priority wins):

1. **CLI flags** — `--var key=value`
2. **Variable files** — `--var-file=values.yaml`
3. **Default config** — `~/.starkite/config.yaml` or `./config.yaml`
4. **Environment** — `STARKITE_VAR_key=value`
5. **Script default** — `var_str("key", "default")`

## Variable Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `var_str(name, default="")` | string | String variable |
| `var_int(name, default=0)` | int | Integer variable |
| `var_bool(name, default=False)` | bool | Boolean variable |
| `var_float(name, default=0.0)` | float | Float variable |
| `var_list(name, default=[])` | list | List variable (auto-detects JSON from CLI) |
| `var_dict(name, default={})` | dict | Dict variable (auto-detects JSON from CLI) |
| `var_names()` | list | Sorted list of all variable names |

## Config File Format

```yaml
# ~/.starkite/config.yaml or ./config.yaml

project:
  name: my-project
  version: 0.1.0

defaults:
  log_level: info
  timeout: 300

providers:
  ssh:
    user: deploy
    private_key_file: ~/.ssh/id_rsa

# Top-level keys become variables
environment: dev
replicas: 3
labels:
  app: myapp
  team: platform
```

## Access Patterns

```python
# Simple variables
env = var_str("environment", "dev")
count = var_int("replicas", 3)

# Nested variables (dot notation)
user = var_str("ssh.user", "deploy")

# Complex types
labels = var_dict("labels", {"app": "default"})
regions = var_list("regions", ["us-east-1"])

# List all available variables
for name in var_names():
    print(name, "=", var_str(name))
```

## Environment Variables

Environment variables with the `STARKITE_VAR_` prefix are automatically available:

```bash
export STARKITE_VAR_DATABASE_HOST=pg.local
export STARKITE_VAR_DATABASE_PORT=5432
```

Access in scripts:
```python
host = var_str("database.host")   # "pg.local"
port = var_int("database.port")   # 5432
```

Underscores in the env var name become dots: `STARKITE_VAR_DATABASE_HOST` → `database.host`.
