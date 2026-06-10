---
title: "Configuration"
description: "Variable injection and the config.yaml file"
weight: 30
---

# Configuration

Starkite scripts read inputs through a layered variable-injection system and an optional `config.yaml`. The same `var_*` functions resolve values from every source, so a script reads a variable the same way whether it came from a CLI flag, a file, the environment, or a default.

## Variable injection

Starkite resolves variables from five sources, highest priority first:

1. **CLI flags** — `--var key=value`
2. **Variable files** — `--var-file=values.yaml`
3. **Default config** — `~/.starkite/config.yaml` or `./config.yaml`
4. **Environment** — `STARKITE_VAR_key=value`
5. **Script default** — `var_str("key", "default")`

### Variable functions

| Function | Returns | Description |
|----------|---------|-------------|
| `var_str(name, default="")` | string | String variable |
| `var_int(name, default=0)` | int | Integer variable |
| `var_bool(name, default=False)` | bool | Boolean variable |
| `var_float(name, default=0.0)` | float | Float variable |
| `var_list(name, default=[])` | list | List variable (auto-detects JSON from CLI) |
| `var_dict(name, default={})` | dict | Dict variable (auto-detects JSON from CLI) |
| `var_names()` | list | Sorted list of all variable names |

### Access patterns

```python
# Top-level variables
env = var_str("environment", "dev")
count = var_int("replicas", 3)

# Nested user variables (dot notation flattens automatically)
app = var_str("labels.app")               # "myapp"
labels = var_dict("labels", {})           # {"app": "myapp", "team": "platform"}

# Lists
regions = var_list("regions", ["us-east-1"])

# Enumerate every defined variable
for name in var_names():
    print(name, "=", var_str(name))
```

Pass values on the command line with `--var` (repeatable) or a `--var-file`:

```bash
kite run ./deploy.star --var environment=prod --var replicas=5
kite run ./deploy.star --var-file=prod.yaml
```

## Config file format

`~/.starkite/config.yaml` (and `./config.yaml` in the working directory) holds defaults for the starkite runtime. Four top-level keys are **reserved** — parsed specially and **not** accessible via `var_*`:

| Reserved key | Purpose |
|---|---|
| `project` | Project metadata (name, version). Read by tooling; not user variables. |
| `defaults` | Runtime defaults (log_level, timeout). Read by the runtime; not user variables. |
| `providers` | Provider-specific defaults (`ssh`, etc.). Read by the relevant module at construction time; not user variables. |
| `active_edition` | The active edition for `kite edition use`. |

Every **other** top-level key becomes a user variable accessible via `var_*`. Nested maps flatten into dot-notation:

```yaml
# ~/.starkite/config.yaml

# Reserved sections (not accessible via var_*)
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

# User variables (accessible via var_*)
environment: dev
replicas: 3
labels:
  app: myapp
  team: platform
```

Reserved keys (`project.name`, `providers.ssh.user`, etc.) do not appear in `var_names()`, and `var_str("providers.ssh.user")` returns the default. Provider config is read by the relevant module's factory (`ssh.config(...)`), not via `var_*`.

## Environment variables

Environment variables prefixed with `STARKITE_VAR_` are picked up automatically. Underscores in the name become dots:

```bash
export STARKITE_VAR_DATABASE_HOST=pg.local
export STARKITE_VAR_DATABASE_PORT=5432
```

```python
host = var_str("database.host")   # "pg.local"
port = var_int("database.port")   # 5432
```
