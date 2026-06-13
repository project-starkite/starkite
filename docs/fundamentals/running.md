---
title: "Running scripts"
description: "Entry points, the shebang line, exit codes, and cleanup"
weight: 70
---

# Running scripts

Running a script is the moment everything else serves: the modules, the permissions, the sandbox all exist so that a file of Starlark can execute and produce a result. A starkite script runs top to bottom, and the runtime gives you a few controlled places to hook into that flow — where execution starts, an optional `main()` entry point, and how the script decides to exit and clean up after itself. Each of those is a deliberate seam, and the sections below take them in the order a run encounters them.

## Ways to run a script

Most runs start from a file you hand to `kite`. The `run` command executes a script, `--var` feeds it values from the command line, and `exec` runs a fragment of source inline without a file at all:

```bash
kite run ./deploy.star                 # run a file
kite run ./deploy.star --var env=prod  # pass variables
kite exec 'print(os.hostname())'     # run inline source
```

Each of these spawns the same engine; the only difference is where the source comes from. When you want a script to behave like any other command on the system, make it directly executable with a shebang line so the shell invokes `kite` for you:

```python
#!/usr/bin/env kite
print("hello")
```

```bash
chmod +x hello.star
./hello.star
```

After `chmod +x`, the script is a first-class executable — it runs from a path, a `PATH` lookup, or a cron entry with no mention of `kite` at the call site.

## Entry point

A loose script that just runs top to bottom is the simplest case, but as a script grows you usually want a named place where execution begins. Define a `main` function and the runtime calls it automatically once the top-level code has finished:

```python
def main():
    print("hello")
```

With that definition in place, `kite run ./hello.star` prints `hello` — you write no explicit call, because the runtime supplies it. Defining `main` stays optional; a script without it runs entirely at the top level, exactly as before.

Auto-invocation creates one hazard worth understanding: if a script both defines `main` and calls it at the top level, naive behavior would run it twice. The runtime guards against that by detecting the explicit call and declining to invoke `main` a second time. It records a notice on stderr so the skipped invocation is never silent:

```
level=INFO msg="skipping automatic entry-point invocation: script calls it at top level" entrypoint=main script=hello.star
```

That detection is syntactic — it recognizes a direct top-level call such as `main()`, and nothing subtler. A call reached through an alias or buried inside control flow slips past it and would run `main` twice, so keep the explicit call plain if you write one at all.

Auto-invocation is scoped to the entry script alone. A `main` defined in a module pulled in with `load()` is never called automatically, which keeps a library from running itself the moment it is imported. The entry point must be callable with no arguments; everything a script needs from the outside arrives through the [variable-injection system](configuration.md) rather than function parameters.

## Exit codes

A run ends either by reaching the end of the script or by saying so explicitly, and the explicit path is what lets a script signal success or failure to whatever invoked it. `exit(code)` ends the script with a given process exit code, and `fail(msg)` aborts immediately with a non-zero code and an error message:

```python
if not ready:
    fail("preflight check failed")

exit(0)
```

Reach for `fail` at a failed precondition so the caller sees both a non-zero status and the reason; reach for `exit(0)` to end cleanly once the work is done. A shell, a CI step, or a parent script reads that exit code to decide what happens next.

## Cleanup and signals

Ending a run is rarely just about the exit code — a script that opened a database handle or acquired a lock has to release it, and it has to do so however the run ends. `defer(fn)` registers a function to run when the script finishes, in last-in-first-out order, so cleanup stays next to the acquisition it undoes:

```python
db = sql.open("sqlite", "app.db")
defer(lambda: db.close())
```

Registering the close immediately after the open means the handle is released whether the script exits normally, calls `fail`, or falls off the end — you never have to thread cleanup through every return path.

A deferred function runs when the script decides to finish, but a long-running script can also be stopped from outside by an OS signal. `on_signal(name, fn)` registers a handler for one — `"SIGINT"`, for instance — so an interruption becomes a chance to shut down deliberately rather than a hard kill:

```python
on_signal("SIGINT", lambda: print("interrupted, shutting down"))
```

With the handler in place, pressing Ctrl-C runs your shutdown code instead of tearing the process down mid-operation, which is what lets a watcher or a server stop without leaving resources dangling.
