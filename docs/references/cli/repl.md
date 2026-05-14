---
title: "kite repl"
description: "Interactive Starlark REPL"
weight: 4
---

Start an interactive Starlark REPL (Read-Eval-Print Loop) with all starkite modules loaded.

## Usage

```bash
kite repl
```

All built-in modules are available. Useful for exploring APIs and quick prototyping.

## Example Session

```text
>>> print(runtime.platform())
linux
>>> t = time.now()
>>> print(t.string("datetime"))
2026-03-15 12:30:00
>>> data = json.encode({"name": "test"})
>>> print(data)
{"name":"test"}
```
