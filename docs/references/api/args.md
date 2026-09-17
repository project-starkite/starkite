---
title: "args"
description: "Declarative CLI flag and positional argument parsing"
weight: 23
---

The `args` module provides declarative CLI flag and positional argument parsing for Starkite scripts. Like all core modules, it is auto-loaded in every script without `load()`.

## Functions

### Flag and Positional Declarations

| Function | Returns | Description |
|----------|---------|-------------|
| `args.string(name, ...)` | `None` | Declare a string flag |
| `args.int(name, ...)` | `None` | Declare an integer flag with optional range limits |
| `args.bool(name, ...)` | `None` | Declare a boolean flag |
| `args.float(name, ...)` | `None` | Declare a floating-point flag with optional range limits |
| `args.list(name, ...)` | `None` | Declare a multi-value flag (repeatable or comma-separated) |
| `args.positional(name, ...)` | `None` | Declare a positional argument |

### Parser

| Function | Returns | Description |
|----------|---------|-------------|
| `args.parse()` | `ArgsResult` | Parse forwarded script arguments against declared schema |

All functions also support the `try_` error pattern (e.g., `args.try_string()`, `args.try_parse()`) which returns a `Result` struct (`res.ok`, `res.value`, `res.error`).

---

## Schema Declaration API

### `args.string`

```python
args.string(
    name,
    shorthand = "",          # Single-character alias (e.g., "e" -> -e)
    default = "",            # Default value when flag is omitted
    choices = [],            # Optional allowed string choices
    required = False,        # Require flag on CLI
    help = "",               # Description displayed in --help
    var_fallback = "",       # Fallback key in VarStore (--var)
)
```

### `args.int`

```python
args.int(
    name,
    shorthand = "",
    default = 0,
    min = None,              # Lower bound (inclusive)
    max = None,              # Upper bound (inclusive)
    required = False,
    help = "",
    var_fallback = "",
)
```

### `args.bool`

```python
args.bool(
    name,
    shorthand = "",
    default = False,
    help = "",
    var_fallback = "",
)
```

Boolean flags do not take arguments on the CLI. Passing `--<flag>` sets the value to `True`. Passing `--no-<flag>` explicitly sets the value to `False`.

### `args.float`

```python
args.float(
    name,
    shorthand = "",
    default = 0.0,
    min = None,              # Lower bound (inclusive)
    max = None,              # Upper bound (inclusive)
    required = False,
    help = "",
    var_fallback = "",
)
```

### `args.list`

```python
args.list(
    name,
    shorthand = "",
    default = [],
    item_type = "string",    # Element type: "string", "int", or "float"
    required = False,
    help = "",
    var_fallback = "",
)
```

List flags accept multiple values through repeated flags (e.g., `--tag web --tag api`) or comma-separated lists (e.g., `--tag web,api`).

### `args.positional`

```python
args.positional(
    name,
    required = True,         # Whether argument is mandatory
    default = None,          # Fallback value if optional
    help = "",               # Description displayed in --help
)
```

Positional arguments are resolved in the order declared. All required positionals must precede optional positionals.

---

## Accessing Parsed Values

`args.parse()` parses the forwarded command-line arguments and returns an immutable `ArgsResult` object:

```python
opts = args.parse()

# 1. Dot-notation attribute access (hyphens normalized to underscores)
print(opts.service_name)
print(opts.dry_run)

# 2. Dictionary-style indexing
print(opts["service_name"])

# 3. Safe lookup with default
val = opts.get("optional_key", "fallback")
```

---

## Automated Help Generation

When `--help` or `-h` is supplied after the script target, Starkite intercepts the execution before running the script, formats a standard Unix help screen, prints it to stdout, and exits with code `0`:

```bash
kite run ./deploy.star --help
```

```text
Usage: kite run ./deploy.star [flags] <service-name>

Arguments:
  <service-name>              Target service identifier (required)

Flags:
  -e, --environment string    Deployment environment (choices: dev, staging, prod) (default: "dev")
  -r, --replicas int          Number of replicas (range: 1..100) (default: 1)
  -p, --preview               Preview deployment without changes
  -t, --tags strings          Resource tags (repeatable or comma-separated) (default: ["web"])
  -h, --help                  Show help for deploy.star
```

---

## Ambient Variable Fallback (`var_fallback`)

When a flag has `var_fallback` defined and the flag is omitted from the CLI invocation, `args.parse()` checks the ambient `VarStore` (populated from `--var`, `--var-file`, environment variables `STARKITE_VAR_*`, or `~/.starkite/config.yaml`):

```python
args.string("environment", default="dev", var_fallback="env")
```

1. If `--environment staging` is passed, value is `"staging"`.
2. If `--environment` is omitted, but `--var env=staging` is present, value is `"staging"`.
3. If neither is provided, value falls back to `"dev"`.

---

## Passing Colliding Flags via `--`

Kite runtime flags (such as `--dry-run`, `--timeout`, and `--permissions`) are consumed by the Go binary before reaching the script. To pass a colliding flag name directly to the script, place the POSIX `--` delimiter before script arguments:

```bash
# Bypasses kite runtime flags and forwards --timeout and --dry-run directly to the script:
kite run ./deploy.star -- --timeout 5s --dry-run
```

---

## Unhandled Argument Detection

If CLI arguments are passed to a script that does not call `args.parse()`, execution halts immediately after the script finishes with exit code `6` (`ExitUsageError`):

```text
Error: unhandled arguments: --foo bar
  Script './deploy.star' did not process these arguments (args.parse() was not called).
exit status 6
```
