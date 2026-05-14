---
title: "Language"
description: "Variable injection and error handling patterns"
weight: 30
---

Starkite scripts are Starlark — a deterministic, Python-derived language — extended with two core conventions that show up across every module: a layered variable-injection system, and the `try_` prefix for error handling.

## Variable Injection

Starkite resolves variables from five sources, highest priority first:

1. **CLI flags** — `--var key=value`
2. **Variable files** — `--var-file=values.yaml`
3. **Default config** — `~/.starkite/config.yaml` or `./config.yaml`
4. **Environment** — `STARKITE_VAR_key=value`
5. **Script default** — `var_str("key", "default")`

### Variable Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `var_str(name, default="")` | string | String variable |
| `var_int(name, default=0)` | int | Integer variable |
| `var_bool(name, default=False)` | bool | Boolean variable |
| `var_float(name, default=0.0)` | float | Float variable |
| `var_list(name, default=[])` | list | List variable (auto-detects JSON from CLI) |
| `var_dict(name, default={})` | dict | Dict variable (auto-detects JSON from CLI) |
| `var_names()` | list | Sorted list of all variable names |

### Config File Format

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

### Access Patterns

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

### Environment Variables

Environment variables prefixed with `STARKITE_VAR_` are picked up automatically. Underscores in the name become dots:

```bash
export STARKITE_VAR_DATABASE_HOST=pg.local
export STARKITE_VAR_DATABASE_PORT=5432
```

```python
host = var_str("database.host")   # "pg.local"
port = var_int("database.port")   # 5432
```

## Error Handling

Every starkite function that can fail has a `try_` variant that returns a `Result` instead of raising. The `Result` type has three attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | bool | `True` if the operation succeeded |
| `value` | any | Return value on success |
| `error` | string | Error message on failure |

### The try_ Pattern

```python
# Without try_ — raises on failure
content = read_text("/etc/hosts")

# With try_ — returns Result
result = fs.path("/etc/missing").try_read_text()
if result.ok:
    print(result.value)
else:
    print("Error:", result.error)
```

### Constructing Results

The `Result()` built-in constructs Result values, which is useful with `retry`:

```python
def check_service():
    resp = http.url("http://localhost:8080/health").try_get()
    if resp.ok and resp.value.status_code == 200:
        return Result(ok=True, value="healthy")
    return Result(ok=False, error="unhealthy")

result = retry.do(check_service, max_attempts=5, delay="2s")
```

### Object Method Variants

Objects support `try_` on their methods too:

```python
# File objects
f = json.file("config.json")
result = f.try_decode()

# Path objects
p = fs.path("/tmp/data.txt")
result = p.try_read_text()

# HTTP
url = http.url("https://api.example.com/data")
result = url.try_get()
```

### Module-Level Factories

Factory functions also have `try_` variants:

```python
result = json.try_file("maybe-missing.json")
if result.ok:
    data = result.value.decode()
```
