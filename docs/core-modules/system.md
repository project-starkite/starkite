---
title: "Execute local commands"
description: "Run shell commands and capture their output with os.exec"
weight: 3
---

# Execute local commands

Most automation is glue between tools you already trust, and starkite treats shelling out as a first-class way to write that glue. The script is the orchestrator: you compose `kubectl`, `helm`, `git`, `make`, and any other CLI by wrapping them in `os.exec` and reading back what they print. The `os` module gives you the two calls you need for that — one that halts the script when a command fails, and one that hands you the failure to decide what to do with.

Which one you reach for depends on how you want failure handled. `os.exec(cmd)` runs the command through `/bin/sh -c`, captures its standard output, and returns that output as a string — but a non-zero exit raises, halting the script. Use it when a failed command means the run cannot continue. `os.try_exec(cmd)` runs the same way but never raises: it returns an `ExecResult` carrying `.ok`, `.stdout`, `.stderr`, and `.code`, leaving the decision to you. Reach for it when failure is a branch in your logic rather than a dead end. Both forms pass `cmd` through the shell, so pipes, redirection, and globbing work exactly as written.

**Source:** [`examples/core/sysinfo.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/sysinfo.star) (excerpted)

## Script

The example below builds a small system report, and it leans on both variants to do it. Host facts come from the global aliases `hostname()` and `cwd()`; the OS string comes from a command whose failure should stop the report; and the disk reading comes from a command whose failure can be skipped without losing the rest:

```python
#!/usr/bin/env kite

print("=" * 60)
print("System Information Report")
print("=" * 60)

# Hostname + working directory (global aliases on the os module)
printf("\n[Host]\n")
printf("  Hostname:  %s\n", hostname())
printf("  Directory: %s\n", cwd())

# OS info — exec returns the command's stdout
os_info = os.exec("uname -a")
printf("\n[Operating System]\n")
printf("  %s\n", os_info.strip())

# Disk usage — try_exec returns an ExecResult; safe to skip if df fails
disk = os.try_exec("df -h / | tail -1")
if disk.ok and disk.stdout:
    fields = disk.stdout.split()
    printf("\n[Disk /]\n")
    printf("  Total:     %s\n", fields[1])
    printf("  Used:      %s (%s)\n", fields[2], fields[4])
    printf("  Available: %s\n", fields[3])
```

The two calls show the contract in practice. `os.exec("uname -a")` returns the command's stdout straight into `os_info`, so you work with the string directly. `os.try_exec("df -h / | tail -1")` returns an `ExecResult` instead, and the script guards on `disk.ok` before touching `disk.stdout` — if `df` is missing or fails, the disk section is simply omitted and the report still prints.

## Run it

Run the script the same way you run any other:

```bash
kite run ./examples/core/sysinfo.star
```

The report prints each section in order, filling in whatever the commands returned on your machine:

```
============================================================
System Information Report
============================================================

[Host]
  Hostname:  dev-host.local
  Directory: /home/alice/projects/starkite

[Operating System]
  Linux dev-host.local 6.5.0-generic #1 SMP ...

[Disk /]
  Total:     466Gi
  Used:      8.7Gi (3%)
  Available: 336Gi
```

The host-specific values differ from machine to machine, but the structure holds: the OS line came from `os.exec`, and the disk block appeared only because `try_exec` reported success.

## What's happening

Two details are worth keeping in mind as you adapt this:

- `os.exec(cmd)` raises on non-zero exit, so use it only when a failed command should halt the script. `os.try_exec(cmd)` returns an `ExecResult` with `.ok`, `.stdout`, `.stderr`, and `.code` instead of halting, so use it when you want to inspect the outcome and continue.
- `os.exec` carries the trailing newline from the underlying shell command, so call `.strip()` when you need the bare value.

## Permissions

Shelling out is a privileged operation, and the permission engine gates it by where the binary lives. Running a command under `$CWD` requires `allow-local`, which grants `os.exec($CWD/**)`; running a command anywhere on the system requires `allow-all`. Reading the environment with `os.env` sits lower, under `allow-fs`. Run a script at the tightest profile that covers the commands it actually invokes — a script that only builds inside its own tree needs `allow-local`, not the unrestricted reach of `allow-all`. See [Permission](../fundamentals/security/permission.md) for the full profile ladder.

## See also

- [`os` reference](../references/api/os.md) — `exec`, `try_exec`, `env`, `setenv`, `which`
- [Language: error handling](../fundamentals/language.md#error-handling) — the `try_` pattern and `ExecResult`
