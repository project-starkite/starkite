---
title: "Running scripts"
description: "Entry points, the shebang line, exit codes, and cleanup"
weight: 70
---

# Running scripts

Starkite scripts are Starlark programs executed from top to bottom by the `kite` runtime. The runtime governs the script's execution lifecycle, managing the process from initial load through termination and final resource cleanup.

## Ways to run a script

Starkite scripts can be executed using the `kite` CLI via two primary commands:

| Command | Purpose | Reference |
|---------|---------|-----------|
| `kite run` | Executes a Starlark script file, directory, or module. | [`kite run` CLI Reference](../references/cli/run.md) |
| `kite exec` | Executes an inline Starlark code fragment directly from the shell. | [`kite exec` CLI Reference](../references/cli/exec.md) |

Both execution methods spawn the same underlying engine. 

```bash
kite run ./deploy.star                 # run a file
kite run ./deploy.star --var env=prod  # pass variables
kite exec 'print(os.hostname())'       # run inline source
```

To run a script directly without invoking the `kite` binary explicitly, configure a shebang line and make the file executable:

```python
#!/usr/bin/env kite
print("hello")
```

```bash
chmod +x hello.star
./hello.star
```

Once configured with execution permissions, the script runs from a system path or a scheduler (like cron) identical to a native binary.

## Entry point

For scripts with structured execution logic, you can define a `main()` function. The runtime automatically invokes `main()` after executing the top-level script code:

```python
def main():
    print("hello")
```

Defining `main` is optional; scripts without it execute sequentially at the top level.

If a script defines `main` and also calls it explicitly at the top level, the runtime avoids double-execution by skipping the automatic invocation. A notice is printed to stderr when this occurs:

```
level=INFO msg="skipping automatic entry-point invocation: script calls it at top level" entrypoint=main script=hello.star
```

This detection is syntactic; it only recognizes a direct top-level call like `main()`. Calls made via an alias or within control flow structures will bypass this guard, causing `main` to run twice.

Automatic invocation applies only to the entry script. A `main` function defined in an imported module is ignored, preventing libraries from executing automatically upon load. 

The entry point function must accept no arguments. External variables are passed to the script via the [variable-injection system](configuration.md) instead of function parameters.

## Exit codes

Scripts exit either by completing top-to-bottom execution or by calling an explicit termination function. To signal execution status to shell environments, use `exit(code)` or `fail(msg)`:

* **`exit(code)`**: Terminates script execution immediately with the specified process exit code.
* **`fail(msg)`**: Aborts execution, prints the error message, and returns a non-zero exit code.

```python
ready = False
if not ready:
    fail("preflight check failed")

exit(0)
```

## Cleanup and signals

To release resources like database handles or file locks regardless of execution success or failure, register cleanup functions using `defer(fn)`. Deferred functions run in last-in-first-out (LIFO) order when the script finishes:

```python
db = sql.open("sqlite", "app.db")
defer(lambda: db.close())
```

Deferred functions execute whether the script completes normally, calls `fail`, or exits early.

Long-running scripts can handle OS signals to perform clean shutdowns using `on_signal(name, fn)`. For example, trapping `"SIGINT"` allows a script to stop gracefully:

```python
on_signal("SIGINT", lambda: print("interrupted, shutting down"))
```

Registering a signal handler replaces the default process termination behavior, allowing the script to release resources before exiting.

