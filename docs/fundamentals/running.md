---
title: "Running scripts"
description: "Entry points, the shebang line, exit codes, and cleanup"
weight: 70
---

# Running scripts

A starkite script runs top to bottom. This page covers how execution starts, the optional `main()` entry point, and how a script controls exit and cleanup.

## Ways to run a script

```bash
kite run ./deploy.star                 # run a file
kite run ./deploy.star --var env=prod  # pass variables
kite exec 'print(os.hostname())'     # run inline source
```

A script can also be made directly executable with a shebang:

```python
#!/usr/bin/env kite
print("hello")
```

```bash
chmod +x hello.star
./hello.star
```

## Entry point

A script may define a `main` function as its entry point. After the script's top-level code runs, the runtime calls `main()` automatically:

```python
def main():
    print("hello")
```

`kite run ./hello.star` prints `hello` — no explicit call needed.

Defining `main` is optional. A script that does not define it runs entirely at the top level.

If a script both defines `main` and calls it at the top level, the runtime detects the explicit call and does not call `main` a second time. It records a notice on stderr so the skipped invocation is visible:

```
level=INFO msg="skipping automatic entry-point invocation: script calls it at top level" entrypoint=main script=hello.star
```

The detection is syntactic — it recognizes a direct top-level call such as `main()`. A call reached through an alias or nested inside control flow is not detected and would run `main` twice.

Automatic invocation applies only to the entry script. A `main` defined in a module loaded with `load()` is never called automatically. `main` must be callable with no arguments; script inputs come from the [variable-injection system](configuration.md).

## Exit codes

`exit(code)` ends the script with a process exit code; `fail(msg)` aborts with a non-zero code and an error message:

```python
if not ready:
    fail("preflight check failed")

exit(0)
```

## Cleanup and signals

`defer(fn)` registers a function to run when the script finishes, in last-in-first-out order — useful for releasing resources regardless of how the script exits:

```python
db = sql.open("sqlite", "app.db")
defer(lambda: db.close())
```

`on_signal(name, fn)` registers a handler for an OS signal (for example `"SIGINT"`), letting a long-running script clean up on interruption:

```python
on_signal("SIGINT", lambda: print("interrupted, shutting down"))
```
