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
3. **Default config** — `~/.starkite/config.yaml` or `./config.yaml` (loaded by every script-executing command: `run`, `exec`, `test`, `repl`, `watch`)
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

## Variable files (`--var-file`)

A var-file is a plain YAML map of variables — every top-level key is a variable. There are no sections and no reserved keys; the file is values only:

```yaml
# prod.yaml
environment: prod
replicas: 5
labels:
  app: myapp
  team: platform
regions:
  - us-east-1
  - eu-west-1
```

```python
var_str("environment")    # "prod"
var_int("replicas")       # 5
var_str("labels.app")     # "myapp"  — nested maps flatten to dot notation
var_dict("labels")        # {"app": "myapp", "team": "platform"} — and stay whole
var_list("regions")       # ["us-east-1", "eu-west-1"]
```

Values keep their YAML types: integers read with `var_int`, lists with `var_list`, maps with `var_dict`. A nested map is available both whole (`var_dict("labels")`) and flattened (`var_str("labels.app")`).

The flag repeats; later files override earlier ones on key collisions, and `--var` overrides them all:

```bash
kite run ./deploy.star --var-file=base.yaml --var-file=prod.yaml --var replicas=7
```

A var-file is not a config file: it does not use the three-section schema below. A `config:`, `permissions:`, or `sandbox:` key in a var-file is just a variable named `config.…`, `permissions.…`, or `sandbox.…` — permission and sandbox profiles are defined only in `config.yaml`.

## Config file format

`~/.starkite/config.yaml` (and `./config.yaml` in the working directory) is the single configuration file for the starkite runtime. It has exactly **three top-level sections** — any other top-level key is an error:

| Section | Purpose |
|---|---|
| `config` | Arbitrary configuration: runtime settings and user variables |
| `permissions` | Named permission profiles, selectable with `--permissions=<name>`; a profile named `default` becomes the implicit profile when no flag is given. See [Permission](security/permission.md#custom-permission-profiles). |
| `sandbox` | Named sandbox profiles, selectable with `--sandbox=<name>`. See [Sandbox](security/sandbox.md). |

Within `config:`, four keys are **reserved** — parsed into runtime state and **not** accessible via `var_*`:

| Reserved key | Purpose |
|---|---|
| `config.project` | Project metadata (name, version). Read by tooling; not user variables. |
| `config.defaults` | Runtime defaults (log_level, timeout). Read by the runtime; not user variables. |
| `config.providers` | Provider-specific defaults (`ssh`, etc.). Read by the relevant module at construction time; not user variables. |
| `config.active_edition` | The active edition for `kite edition use`. |

Every **other** key under `config:` becomes a user variable accessible via `var_*`. Nested maps flatten into dot-notation:

```yaml
# ~/.starkite/config.yaml
config:
  # Reserved keys (not accessible via var_*)
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

permissions:
  default: allow-fs        # alias — a built-in as the implicit profile
  ci:
    allow: ["fs.read", "os.exec($CWD/**)"]

sandbox:
  builder:
    network: host
    mounts:
      - destination: /tmp
        type: tmpfs
```

Reserved keys (`config.project.name`, `config.providers.ssh.user`, etc.) do not appear in `var_names()`, and `var_str("providers.ssh.user")` returns the default. Provider config is read by the relevant module's factory (`ssh.config(...)`), not via `var_*`.

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
