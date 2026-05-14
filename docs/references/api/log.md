---
title: "log"
description: "Structured logging with configurable levels, formats, and outputs"
weight: 18
---

The `log` module provides structured logging built on Go's `slog` package.

## Module-Level Functions

### Logging

| Function | Returns | Description |
|----------|---------|-------------|
| `log.debug(msg, attrs={})` | `None` | Log a debug message with optional attributes |
| `log.info(msg, attrs={})` | `None` | Log an info message with optional attributes |
| `log.warn(msg, attrs={})` | `None` | Log a warning message with optional attributes |
| `log.error(msg, attrs={})` | `None` | Log an error message with optional attributes |

### Configuration

| Function | Returns | Description |
|----------|---------|-------------|
| `log.set_level(level)` | `None` | Set the module-level log level (`"debug"`, `"info"`, `"warn"`, `"error"`) |
| `log.set_format(format)` | `None` | Set the module-level log format (`"text"` or `"json"`) |
| `log.set_output(output)` | `None` | Set the module-level output (`"stderr"` or `"stdout"`) |

### Logger Factory

| Function | Returns | Description |
|----------|---------|-------------|
| `log.logger(level="info", format="text", output="stderr")` | `Logger` | Create a new independent logger |

## Logger Object

### Properties

| Property | Type | Description |
|----------|------|-------------|
| `level` | `string` | Current log level |
| `format` | `string` | Current format (`"text"` or `"json"`) |
| `output` | `string` | Current output target |

### Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `debug(msg, attrs={})` | `None` | Log a debug message |
| `info(msg, attrs={})` | `None` | Log an info message |
| `warn(msg, attrs={})` | `None` | Log a warning message |
| `error(msg, attrs={})` | `None` | Log an error message |
| `attrs(dict)` | `Logger` | Return a new logger with additional default attributes |

## Formats

| Format | Description |
|--------|-------------|
| `"text"` | Human-readable text output (slog `TextHandler`) |
| `"json"` | Structured JSON output (slog `JSONHandler`) |

## Examples

### Basic logging

```python
log.info("server started", {"port": 8080, "env": "production"})
log.warn("disk space low", {"path": "/data", "free_gb": 2})
log.error("connection failed", {"host": "db-01", "error": "timeout"})
```

### Setting log level

```python
log.set_level("debug")
log.debug("verbose output", {"step": "init"})
```

### JSON format

```python
log.set_format("json")
log.info("request", {"method": "GET", "path": "/api/v1/pods"})
# {"time":"...","level":"INFO","msg":"request","method":"GET","path":"/api/v1/pods"}
```

### Custom logger

```python
l = log.logger(level="debug", format="json", output="stdout")
l.info("custom logger", {"component": "auth"})
```

### Logger with default attributes

```python
l = log.logger(format="json")
app_log = l.attrs({"app": "myservice", "version": "1.2.0"})
app_log.info("started")  # includes app and version in every message
```

> **Note:**
All `log` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

