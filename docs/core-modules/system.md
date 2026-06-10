---
title: "Execute local commands"
description: "Run shell commands and capture their output with os.exec"
weight: 3
---

# Execute local commands

Shelling out to other tools is a first-class operation in starkite. The script is the orchestrator: starkite scripts commonly compose `kubectl`, `helm`, `git`, `make`, and other CLIs by wrapping them in `os.exec` calls and parsing the output.

The contract has two variants. `os.exec(cmd)` runs a command through `/bin/sh -c`, captures stdout, and returns it as a string — raising on non-zero exit (halt-on-failure). `os.try_exec(cmd)` returns a `Result` with `.ok`, `.stdout`, `.stderr`, and `.exit_code` instead of raising (capture-and-decide). Both pass `cmd` through the shell, so pipes, redirection, and shell globbing work as written.

**Source:** [`examples/core/sysinfo.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/sysinfo.star) (excerpted)

## Script

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

# Disk usage — try_exec returns a Result; safe to skip if df fails
disk = os.try_exec("df -h / | tail -1")
if disk.ok and disk.stdout:
    fields = disk.stdout.split()
    printf("\n[Disk /]\n")
    printf("  Total:     %s\n", fields[1])
    printf("  Used:      %s (%s)\n", fields[2], fields[4])
    printf("  Available: %s\n", fields[3])
```

## Run it

```bash
kite run ./examples/core/sysinfo.star
```

Expected output (host-specific values differ):

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

## What's happening

- `os.exec(cmd)` raises on non-zero exit. Use it when failure should halt the script.
- `os.try_exec(cmd)` returns a `Result` with `.ok`, `.stdout`, `.stderr`, `.exit_code` instead of halting on non-zero exit.
- `cmd.strip()` removes the trailing newline `os.exec` carries from the underlying shell command.

## See also

- [`os` reference](../references/api/os.md) — `exec`, `try_exec`, `env`, `setenv`, `which`
- [Language: error handling](../fundamentals/language.md#error-handling) — the `try_` pattern and `Result` type
