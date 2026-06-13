---
title: "Logging"
description: "Structured logging with the log module"
weight: 20
---

# Logging

When a script does real work, you want to know what it did and why — not as a wall of `print()` calls, but as records you can filter, parse, and route. The `log` module gives you that. Built on Go's `slog`, it emits structured messages that each carry a severity level and an optional set of attributes, and by default it writes them to stderr so your log stream stays separate from whatever data the script prints to stdout.

## Logging messages

Start with the four level functions. Each takes a message and, optionally, a dict of attributes that travel alongside it:

```python
log.info("server started", {"port": 8080})
log.warn("retrying", {"attempt": 3})
log.error("request failed", {"status": 500, "path": "/api"})
log.debug("cache miss", {"key": "user:42"})
```

The level you choose is not just a label — it decides whether the line is emitted at all, since the module filters by severity. The attribute dict is the payload you actually search on later: rendering `port=8080` as a named field beats burying it in the message text, because a downstream tool can key off it.

## Configuring output

How those lines look and where they go is yours to set. Three functions adjust the module-wide behavior:

```python
log.set_level("debug")    # "debug" | "info" | "warn" | "error"
log.set_format("json")    # "text" | "json"
log.set_output("stdout")  # "stderr" | "stdout"
```

`set_level` raises or lowers the severity floor — at `"debug"` everything passes, at `"error"` only failures do, so it is how you trade verbosity against noise. `set_format` switches between human-readable text and machine-readable JSON: pick text when a person is reading the terminal, JSON when a log pipeline is parsing the stream. `set_output` redirects the stream when you need the logs on stdout instead of the default stderr.

## Named loggers

The module-wide settings are convenient for a whole script, but a larger program has parts that want their own voice. Create an independent logger when a component needs its own level, format, or output without touching the global configuration:

```python
l = log.logger(level="debug", format="json", output="stdout")
l.info("custom logger", {"component": "auth"})
```

That logger carries its own settings — here it runs at debug level in JSON to stdout — and changing the module defaults later leaves it untouched, which is the point of asking for an independent one.

When a component should stamp the same attributes on every line it writes, derive a logger that carries them by default with `attrs`:

```python
l = log.logger(format="json")
app_log = l.attrs({"app": "myservice", "version": "1.2.0"})
app_log.info("started")   # includes app and version in every message
```

`attrs` returns a *new* logger rather than mutating the one you called it on, so `app_log` adds `app` and `version` to each message it emits while `l` stays as it was. That saves you from repeating the same key/value pairs on every call and keeps a component's lines tagged consistently.

See the [log API reference](../references/api/log.md) for the full surface, including the per-logger `level`, `format`, and `output` properties and the `try_` variants that return a `Result` instead of raising.
