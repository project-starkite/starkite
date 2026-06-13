---
title: "Configuration"
description: "Variable injection and the config.yaml file"
weight: 30
---

# Configuration

Most automation scripts need inputs that change from run to run — the target environment, a replica count, a list of regions — without editing the script each time. Starkite supplies those inputs through a layered variable-injection system, backed by an optional `config.yaml`. A script reads every input through the same `var_*` functions, so it never has to know whether a value arrived from a CLI flag, a file, the environment, or a built-in default. You wire the values in from outside; the script just asks for them by name.

## Variable injection

When a script asks for a variable, Starkite looks for it in five places and takes the first one that has a value. The sources are ordered so that the more specific and more immediate a source is, the more it wins — a flag you type on the command line beats a value baked into a config file, which in turn beats the fallback you wrote in the script:

1. **CLI flags** — `--var key=value`
2. **Variable files** — `--var-file=values.yaml`
3. **Default config** — `~/.starkite/config.yaml` or `./config.yaml` (loaded by every script-executing command: `run`, `exec`, `test`, `repl`, `watch`)
4. **Environment** — `STARKITE_VAR_key=value`
5. **Script default** — `var_str("key", "default")`

This ordering is what lets one script serve every environment: the script carries sensible defaults, the config file sets per-machine values, and a `--var` at launch overrides any of it for a one-off run.

### Variable functions

A script reaches a value through a typed accessor. Each function takes the variable name and an optional default to use when no source supplies the value, and returns the value already coerced to the Go type you asked for:

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

In practice you call these accessors at the top of a script to gather its inputs. Top-level values come back directly; nested maps are reachable both as a whole dict and through dot notation, since Starkite flattens nested keys automatically as it loads them:

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

Here `labels.app` and `labels` read the same data two ways — the flattened leaf and the intact dict — and `var_names()` lets a script discover what it was given rather than hard-coding the names. To feed those variables in from the command line, use `--var` (which you can repeat) or point at a `--var-file`:

```bash
kite run ./deploy.star --var environment=prod --var replicas=5
kite run ./deploy.star --var-file=prod.yaml
```

## Variable files (`--var-file`)

When a run needs more than a handful of values, typing each one as a `--var` becomes unwieldy. A var-file collects them in one place. It is a plain YAML map of variables — every top-level key is a variable, with no sections and no reserved keys, because the file holds values only:

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

The script reads that file through the same accessors it would use for any other source, and the YAML structure carries straight through:

```python
var_str("environment")    # "prod"
var_int("replicas")       # 5
var_str("labels.app")     # "myapp"  — nested maps flatten to dot notation
var_dict("labels")        # {"app": "myapp", "team": "platform"} — and stay whole
var_list("regions")       # ["us-east-1", "eu-west-1"]
```

Values keep their YAML types: integers read with `var_int`, lists with `var_list`, maps with `var_dict`. As with config, a nested map is available both whole (`var_dict("labels")`) and flattened (`var_str("labels.app")`).

Because a single file rarely covers every case, the flag repeats. Later files override earlier ones on key collisions, and a `--var` still overrides them all — so you can layer a base file, an environment-specific file, and a last-minute override in one command:

```bash
kite run ./deploy.star --var-file=base.yaml --var-file=prod.yaml --var replicas=7
```

One distinction matters: a var-file is not a config file. It does not use the three-section schema described next. If you put a `config:`, `permissions:`, or `sandbox:` key in a var-file, it is read as an ordinary variable named `config.…`, `permissions.…`, or `sandbox.…` — permission and sandbox profiles are defined only in `config.yaml`.

## Config file format

That brings us to `config.yaml` itself — the single configuration file for the Starkite runtime, read from `~/.starkite/config.yaml` and from `./config.yaml` in the working directory. Unlike a var-file, it is structured: it has exactly **three top-level sections**, and any other top-level key is an error rather than a stray variable:

| Section | Purpose |
|---|---|
| `config` | Arbitrary configuration: runtime settings and user variables |
| `permissions` | Named permission profiles, selectable with `--permissions=<name>`; a profile named `default` becomes the implicit profile when no flag is given. Profiles can compose on a built-in via `base`; a bare-name value is shorthand for `base`. See [Permission](security/permission.md#custom-permission-profiles). |
| `sandbox` | Named sandbox profiles, selectable with `--sandbox=<name>`; a profile named `default` is what a bare `--sandbox` selects. Profiles can compose on a built-in rung via `base`; a bare-name value is shorthand for `base`. See [Sandbox](security/sandbox.md). |

The `config:` section is where variables and runtime settings live together, and that mixing is deliberate but bounded. Four keys inside it are **reserved** — Starkite parses them into runtime state and they are **not** reachable through `var_*`:

| Reserved key | Purpose |
|---|---|
| `config.project` | Project metadata (name, version). Read by tooling; not user variables. |
| `config.defaults` | Runtime defaults (log_level, timeout). Read by the runtime; not user variables. |
| `config.providers` | Provider-specific defaults (`ssh`, etc.). Read by the relevant module at construction time; not user variables. |
| `config.active_edition` | The active edition for `kite edition use`. |

Every **other** key under `config:` becomes a user variable accessible via `var_*`, and nested maps flatten into dot-notation just as they do from a var-file. The example below shows all three sections at once — reserved runtime keys and user variables side by side under `config:`, then permission and sandbox profiles:

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
  default: allow-fs        # shorthand for {base: allow-fs} — the implicit profile
  ci:
    allow: ["fs.read", "os.exec($CWD/**)"]

sandbox:
  default: net-access      # shorthand for {base: net-access} — bare --sandbox selects this
  builder:
    network: host
    mounts:
      - source: $CWD
        destination: $CWD
        mode: rw
      - destination: /tmp
        type: tmpfs
```

The split is what keeps the two worlds from leaking into each other. The reserved keys (`config.project.name`, `config.providers.ssh.user`, and so on) never appear in `var_names()`, and `var_str("providers.ssh.user")` returns the default rather than the configured value — provider config is read by the relevant module's factory (`ssh.config(...)`), not through `var_*`. So a script cannot accidentally read runtime plumbing as if it were one of its own inputs.

## Environment variables

The remaining source needs no file at all. Any environment variable prefixed with `STARKITE_VAR_` is picked up automatically, with underscores in the name converted to dots so the value lands at a nested key:

```bash
export STARKITE_VAR_DATABASE_HOST=pg.local
export STARKITE_VAR_DATABASE_PORT=5432
```

The script reads those exactly as it would any other variable — the prefix and the underscore-to-dot rule are the only translation:

```python
host = var_str("database.host")   # "pg.local"
port = var_int("database.port")   # 5432
```

This source sits below the config file and above the script default in the precedence order, which makes it a natural fit for values that belong to a host or a CI runner rather than to a script or a single command.
