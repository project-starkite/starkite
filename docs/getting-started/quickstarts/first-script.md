---
title: "Run a first script"
description: "Write and run hello.star — the minimum starkite walkthrough"
weight: 1
---

# Run a first script

The shortest path from `kite version` to a working script. Exercises the global helpers (`hostname`, `username`, `cwd`, `env`), `os.exec`, `time.now`, and `printf`.

**Source:** [`examples/core/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star)

## Script

```python
#!/usr/bin/env kite
# hello.star — exercises print, printf, os.exec, time.now, env.

print("Hello from starkite!")

printf("Hostname: %s\n", hostname())
printf("User:     %s\n", username())
printf("Cwd:      %s\n", cwd())

uname = os.exec("uname -s").strip()
printf("Kernel:   %s\n", uname)

printf("Time:     %s\n", time.format(time.now(), time.RFC3339))
printf("Home:     %s\n", env("HOME", "/tmp"))
```

## Run it

```bash
kite run examples/core/hello.star
```

Expected output (host-specific values differ):

```
Hello from starkite!
Hostname: vladimirs-mbp.lan
User:     vladimirvivien
Cwd:      /Users/vladimirvivien/DEV/starkite
Kernel:   Darwin
Time:     2026-05-15T08:51:41-04:00
Home:     /Users/vladimirvivien
```

## What's happening

- `print` and `printf` come from the `fmt` module's global aliases — no `load()` required.
- `hostname`, `username`, `cwd`, and `env` are global aliases on the `os` module.
- `os.exec("uname -s")` returns the command's stdout as a string; `.strip()` removes trailing newline.
- `time.format(time.now(), time.RFC3339)` formats the current instant as RFC 3339.

## Verify behavior

A companion test asserts each helper behaves as expected:

```bash
kite test examples/core/hello_test.star
```

## See also

- [Common subcommands](cli.md) — `kite exec`, `kite repl`, `kite watch`, `kite test`, `kite validate`
- [`fmt` reference](../../references/api/fmt.md) — `print`, `printf`, `sprintf`, `errorf`
- [`os` reference](../../references/api/os.md) — `hostname`, `username`, `cwd`, `env`, `exec`
- [`time` reference](../../references/api/time.md) — `now`, `format`, format presets
