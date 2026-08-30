---
title: "Configuration"
description: "Variable injection and the config.yaml file"
weight: 30
---

# Configuration

Starkite features a layered configuration system to inject external inputs—such as environment names, ports, or replica counts—into scripts at runtime. Scripts load these variables using typed `var_*` functions. This decouples script logic from the source of the values, whether they are set via command-line flags, YAML files, environment variables, or default fallbacks.

## Variable injection

Starkite evaluates five configuration sources in order of precedence (highest to lowest). A value from a higher-priority source overrides any lower sources:

1. **CLI flags**: `--var key=value`
2. **Variable files**: `--var-file=values.yaml`
3. **Default config**: `~/.starkite/config.yaml` or `./config.yaml` (loaded automatically by `run`, `exec`, `test`, `repl`, and `watch`)
4. **Environment variables**: `STARKITE_VAR_key=value`
5. **Script default**: `var_str("key", "default")`

This hierarchy allows a single script to use embedded defaults, adapt to machine-level config files, and accept runtime command overrides.

### Variable functions

Each accessor function accepts a key name and an optional default value, returning the value coerced to the requested type:

| Function | Returns | Description |
|----------|---------|-------------|
| `var_str(name, default="")` | string | String variable |
| `var_int(name, default=0)` | int | Integer variable |
| `var_bool(name, default=False)` | bool | Boolean variable |
| `var_float(name, default=0.0)` | float | Float variable |
| `var_list(name, default=[])` | list | List variable (detects JSON from CLI) |
| `var_dict(name, default={})` | dict | Dictionary variable (detects JSON from CLI) |
| `var_names()` | list | Sorted list of all defined variable names |

### Access patterns

Scripts typically declare accessors at the top of the file. Nested maps are flattened automatically during load and can be accessed either as a complete dictionary or using dot notation:

```python
# Top-level variables
env = var_str("environment", "dev")
count = var_int("replicas", 3)

# Nested variables (dot notation flattens automatically)
app = var_str("labels.app")               # Returns "myapp"
labels = var_dict("labels")               # Returns {"app": "myapp", "team": "platform"}

# Lists
regions = var_list("regions", ["us-east-1"])

# Enumerate all variables
for name in var_names():
    print(name, "=", var_str(name))
```

To supply variables via the command line, use `--var` (repeatable) or `--var-file`:

```bash
kite run ./deploy.star --var environment=prod --var replicas=5
kite run ./deploy.star --var-file=prod.yaml
```

## Variable files (`--var-file`)

A variable file collects inputs into a single YAML configuration:

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

The runtime maps variables to their corresponding Starlark types. Nested maps are available both as whole dictionaries and as flattened dot-notation keys:

```python
var_str("environment")    # "prod"
var_int("replicas")       # 5
var_str("labels.app")     # "myapp" (flattened key)
var_dict("labels")        # {"app": "myapp", "team": "platform"} (complete dict)
var_list("regions")       # ["us-east-1", "eu-west-1"]
```

You can repeat the `--var-file` flag to layer settings. Keys in later files override earlier ones, and individual `--var` flags override values from files:

```bash
kite run ./deploy.star --var-file=base.yaml --var-file=prod.yaml --var replicas=7
```

Unlike the main `config.yaml` file, a var-file is a flat collection of user variables and does not support runtime keys like `permissions` or `sandbox`.

## Config file format

Starkite loads its global configuration from `~/.starkite/config.yaml` and `./config.yaml`. The file is structured into exactly three top-level sections:

| Section | Purpose |
|---|---|
| `config` | Holds runtime settings and user-defined variables |
| `permissions` | Named permission profiles, selectable via `--permissions=<name>`. See [Permission Guide](security/permission.md#custom-permission-profiles). |
| `sandbox` | Named sandbox profiles, selectable via `--sandbox=<name>`. See [Sandbox Guide](security/sandbox.md). |

The `config` section contains both runtime parameters and user variables. Four reserved keys are parsed into runtime state and cannot be accessed via `var_*`:

| Reserved Key | Purpose |
|---|---|
| `config.project` | Project metadata (name, version) read by build tooling |
| `config.defaults` | Runtime defaults (such as `log_level` and `timeout`) |
| `config.providers` | Default provider configurations (e.g., `ssh`), read directly by modules at construction |

All other keys under `config` are loaded as user variables. The following example demonstrates a `config.yaml` containing runtime settings, user variables, permissions, and sandbox profiles:

```yaml
# ~/.starkite/config.yaml
config:
  # Reserved keys (runtime settings)
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

  # User variables
  environment: dev
  replicas: 3
  labels:
    app: myapp
    team: platform

permissions:
  default: allow-fs        # Implicit default profile
  ci:
    allow: ["fs.read", "os.exec($CWD/**)"]

sandbox:
  default:
    base: net-access       # Bare --sandbox profile fallback
  ci:
    driver: podman         # Binds execution driver to this profile
    base: net-access
    mounts:
      - source: $CWD
        destination: /workspace
        mode: rw
```

Reserved keys (such as `project` or `providers`) do not appear in `var_names()` and cannot be read via `var_*`. Modules read their respective configuration directly from these fields at initialization, ensuring runtime settings do not leak into user script logic.

## Environment variables

Any environment variable prefixed with `STARKITE_VAR_` is automatically loaded as a script variable. Underscores in the variable name are converted to dots to represent nested paths:

```bash
export STARKITE_VAR_DATABASE_HOST=pg.local
export STARKITE_VAR_DATABASE_PORT=5432
```

The script accesses these directly:

```python
host = var_str("database.host")   # "pg.local"
port = var_int("database.port")   # 5432
```

Environment variables rank below file-based configuration but above script-level defaults, making them ideal for host-specific settings or CI runner configurations.
