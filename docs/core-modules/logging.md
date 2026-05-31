---
title: "Logging"
description: "Structured logging with the log module"
weight: 20
---

# Logging

The `log` module provides structured logging built on Go's `slog`. Messages carry a level, an optional set of attributes, and go to stderr by default.

## Logging messages

```python
log.info("server started", {"port": 8080})
log.warn("retrying", {"attempt": 3})
log.error("request failed", {"status": 500, "path": "/api"})
log.debug("cache miss", {"key": "user:42"})
```

The optional second argument is a dict of structured attributes rendered alongside the message.

## Configuring output

```python
log.set_level("debug")    # "debug" | "info" | "warn" | "error"
log.set_format("json")    # "text" | "json"
log.set_output("stdout")  # "stderr" | "stdout"
```

`set_level` filters by severity; `set_format` switches between human-readable text and machine-readable JSON; `set_output` redirects the stream.

## Named loggers

For component-scoped logging, create a logger with bound attributes:

```python
logger = log.logger("worker", {"region": "us-east-1"})
logger.info("processing")   # includes region=us-east-1 on every line
```

See the [log API reference](../references/api/log.md) for the full surface, including per-logger level and format.
