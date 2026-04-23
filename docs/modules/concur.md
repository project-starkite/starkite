---
title: "concur"
description: "Concurrent execution with map, each, and exec"
weight: 20
---

The `concur` module provides concurrent execution primitives for running functions in parallel.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `concur.map(items, func, workers=0, timeout="", on_error="abort")` | `list` | Apply `func` to each item concurrently, returning results in order |
| `concur.each(items, func, workers=0, timeout="", on_error="abort")` | `None` | Apply `func` to each item concurrently (no return values) |
| `concur.exec(*fns, timeout="", on_error="abort")` | `tuple` | Execute multiple functions concurrently, returning their results |

## Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `workers` | `0` | Maximum number of concurrent workers. `0` = unbounded (one goroutine per item) |
| `timeout` | `""` | Maximum time to wait (e.g. `"30s"`, `"5m"`). Empty = no timeout |
| `on_error` | `"abort"` | Error strategy: `"abort"` stops on first error; `"continue"` collects all results |
| `items` | | Any iterable: list, tuple, or other Starlark iterable |

When `on_error="continue"`, failed items return `Result` objects (with `.ok`, `.value`, `.error`) instead of raising.

## Examples

### Parallel map

```python
def fetch_status(host):
    result = exec("ping -c1 -W1 " + host)
    return result.ok

hosts = ["host-1", "host-2", "host-3", "host-4"]
statuses = concur.map(hosts, fetch_status, workers=4)
for host, ok in zip(hosts, statuses):
    print(host, "->", "up" if ok else "down")
```

### Parallel each (side effects only)

```python
def deploy(service):
    exec("kubectl rollout restart deployment/" + service)
    log.info("restarted", {"service": service})

services = ["frontend", "backend", "worker"]
concur.each(services, deploy, workers=3, timeout="2m")
```

### Execute independent functions

```python
def get_pods():
    return exec("kubectl get pods -o json").stdout

def get_nodes():
    return exec("kubectl get nodes -o json").stdout

def get_services():
    return exec("kubectl get svc -o json").stdout

pods, nodes, services = concur.exec(get_pods, get_nodes, get_services, timeout="30s")
```

### Error handling with continue

```python
def risky_operation(item):
    if item == "bad":
        fail("item is bad")
    return item.upper()

results = concur.map(
    ["good", "bad", "fine"],
    risky_operation,
    on_error="continue",
)

for r in results:
    if r.ok:
        print("Success:", r.value)
    else:
        print("Failed:", r.error)
```

### Bounded concurrency

```python
# Limit to 2 concurrent API calls
urls = ["https://api.example.com/a", "https://api.example.com/b", "https://api.example.com/c"]
responses = concur.map(urls, http.get, workers=2)
```

> **Note:**
All `concur` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

