---
title: "Security"
description: "Process execution security, permissions, and sandbox isolation in Starkite"
weight: 40
---

# Process Security

Starkite manages process execution security through two layers of defense: API-level capability checks (Permission Profiles) and kernel-level process isolation (Execution Sandboxing). This section describes how these security layers apply specifically to running external command-line utilities.

---

## 1. Interpreter-Level Execution Protection

When a script attempts to execute an operating system command using the direct APIs (`os.exec()`, `os.try_exec()`) or the shell APIs (`os.shell()`, `os.try_shell()`), the Starkite interpreter intercepts the call and validates the target binary's absolute path against the active permission profile:

* **Local Command Execution**: Running a binary located inside the script's working tree (`$CWD`) is permitted under the **`allow-local`** profile or higher. This allows scripts to run local scripts or vendored binaries, but blocks access to system-wide utilities.
* **System Command Execution**: Running a binary located anywhere else on the host system (such as `/bin/ls`, `/usr/bin/git`, or `/usr/bin/psql`) is blocked by default. It is authorized for direct execution under the **`allow-all`** profile, and for shell execution under the **`allow-all-shell`** profile.
* **Shell-Wrapped Execution**: Running command strings in a shell wrapper via `os.shell()` or `os.try_shell()` is blocked by default in all profiles, including `allow-all`, and requires the **`allow-all-shell`** profile.
* **Identity Switching (POSIX)**: Executing a command under a specific user or group identity (using the `userid` and `groupid` optional keyword arguments) is a highly privileged operation. Direct execution switching requires the **`allow-all`** profile and the `os.exec(switch_identity:...)` capability, while shell execution switching requires the **`allow-all-shell`** profile and the `os.shell(switch_identity:...)` capability.

For instructions on defining custom execution rules and composing profiles, see the [Script Permissions Guide](../fundamentals/security/permission.md).

---

## 2. OS-Level Process Sandboxing

For environments requiring process isolation (such as executing untrusted scripts or running automations in multi-tenant environments), Starkite provides a pluggable OS-level sandbox architecture supporting native OS kernel primitives (Linux Landlock, macOS Seatbelt), container runtimes (Podman, Docker, nerdctl), and external gVisor.

When sandboxing is enabled (via `--sandboxed`, `--sandbox-profile=<name>`, shortcut flags `--sandbox-opaque`/`--sandbox-net`/`--sandbox-host`, or `STARKITE_SANDBOX_PROFILE` environment variable), the script process is confined to explicit filesystem and network boundaries depending on the active sandbox profile:

### Opaque Sandbox (`opaque` profile / `--sandbox-opaque`)
* **Filesystem**: The host filesystem is restricted. The sandboxed process operates within the script's working directory (`$CWD`) mounted as read/write, with a private in-memory filesystem mounted at `/tmp`.
* **Process Execution**: Running external commands without explicit binary paths or mounts is restricted because host user directories and unapproved system paths are omitted.
* **Network**: Disabled (loopback only). Outbound network requests are blocked.

### Network Access Sandbox (`net-access` profile / `--sandbox-net`)
* **Filesystem & Process Execution**: Same isolation as the Opaque profile, with read-only access to host network configuration (`/etc/resolv.conf`, `/etc/hosts`, and system CA certificates) to support outbound TLS operations.
* **Network**: Egress networking enabled for client connections.

### Host Sandbox (`host` profile / `--sandbox-host`)
* **Filesystem & Process Execution**: Grants read-only access to host system paths (such as `$HOME`, `/usr`, `/bin`, `/lib`, and `/lib64`) while confining all writes strictly to `$CWD` and `/tmp`. This allows scripts to inspect host configuration and execute host CLI utilities without modifying host files.
* **Network**: Egress networking enabled.

For driver configuration, driver override flags (`--sandbox-driver=<driver>`), and programmatic Starlark module execution, see the [Execution Sandbox Guide](../fundamentals/security/sandbox.md).
