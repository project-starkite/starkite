---
title: "retry"
description: "Retry logic with configurable backoff and jitter"
weight: 21
---

The `retry` module provides retry logic with fixed delay and exponential backoff strategies.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `retry.do(func, max_attempts=3, delay="1s", retry_on=None, on_retry=None, timeout="", jitter=False)` | `RetryResult` | Retry `func` with fixed delay between attempts |
| `retry.with_backoff(func, max_attempts=5, delay="500ms", max_delay="30s", retry_on=None, on_retry=None, timeout="", jitter=True)` | `RetryResult` | Retry `func` with exponential backoff |

## Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `func` | | The function to retry (takes no arguments, returns a value) |
| `max_attempts` | `3` / `5` | Maximum number of attempts |
| `delay` | `"1s"` / `"500ms"` | Initial delay between retries |
| `max_delay` | `"30s"` | Maximum delay (backoff only) |
| `retry_on` | `None` | A predicate `func(error) -> bool` to decide whether to retry. `None` = retry all errors |
| `on_retry` | `None` | A callback `func(attempt, error)` called before each retry |
| `timeout` | `""` | Overall timeout for all attempts. Empty = no timeout |
| `jitter` | `False` / `True` | Add random jitter to delays |

## RetryResult

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | `bool` | `True` if the function eventually succeeded |
| `value` | `any` | The return value on success |
| `error` | `string` | The last error message on failure |
| `attempts` | `int` | Total number of attempts made |
| `elapsed` | `DurationValue` | Total time spent retrying |
| `errors` | `list` | All error messages from failed attempts |

## Examples

### Simple retry

```python
def connect():
    result = exec("curl -sf http://service:8080/health")
    if not result.ok:
        fail("health check failed")
    return result.stdout

r = retry.do(connect, max_attempts=5, delay="2s")
if r.ok:
    print("Connected after", r.attempts, "attempts")
else:
    print("Failed after", r.attempts, "attempts:", r.error)
```

### Exponential backoff

```python
def call_api():
    resp = http.get("https://api.example.com/data")
    if resp.status_code >= 500:
        fail("server error: " + str(resp.status_code))
    return resp.json()

r = retry.with_backoff(call_api, max_attempts=5, delay="500ms", max_delay="10s")
if r.ok:
    print("Got data:", r.value)
```

### Selective retry

```python
def is_transient(err):
    return "timeout" in err or "503" in err

def fetch():
    resp = http.get("https://api.example.com/resource")
    if resp.status_code != 200:
        fail(str(resp.status_code))
    return resp.json()

r = retry.do(fetch, retry_on=is_transient, max_attempts=3)
```

### Retry with callback

```python
def on_retry(attempt, err):
    log.warn("retrying", {"attempt": attempt, "error": err})

def deploy():
    result = exec("kubectl apply -f manifest.yaml")
    if not result.ok:
        fail(result.stderr)

r = retry.do(deploy, max_attempts=3, delay="5s", on_retry=on_retry)
```

### With timeout

```python
r = retry.with_backoff(
    connect,
    max_attempts=10,
    delay="1s",
    max_delay="30s",
    timeout="2m",
    jitter=True,
)
print("Attempts:", r.attempts, "Elapsed:", r.elapsed)
if not r.ok:
    for i, err in enumerate(r.errors):
        print("  attempt", i + 1, ":", err)
```

> **Note:**
All `retry` functions support `try_` variants that return a `Result` instead of raising an error.

