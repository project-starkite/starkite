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

For environments requiring strong process isolation (such as executing untrusted scripts or running automations in multi-tenant systems), Starkite provides a rootless Linux sandbox backed by an embedded **gVisor** kernel.

When sandboxing is enabled (via the `--sandbox` flag or `STARKITE_SECURITY_SANDBOX` environment variable), the guest process is isolated across UTS, IPC, PID, Mount, and User namespaces. The filesystem and network behavior of external commands vary depending on the active sandbox profile:

### Opaque Sandbox (`strict` / `opaque` profiles)
* **Filesystem**: The host filesystem is completely hidden. The sandboxed process only sees an empty root directory, with the script's working directory (`$CWD`) mounted as read/write, and a temporary in-memory filesystem mounted at `/tmp`.
* **Process Execution**: Running external commands (such as `os.exec("uname")` or `os.exec("git")`) will fail with `fork/exec /bin/sh: no such file or directory` because no host binaries, libraries, or shell interpreters are mounted inside the sandbox.
* **Network**: Only the loopback interface (`127.0.0.1`) is available. Outbound network requests are blocked.

### Network Access Sandbox (`net-access` profile)
* **Filesystem & Process Execution**: Same isolation as the Opaque profile, but mounts read-only host configuration files (`/etc/resolv.conf`, `/etc/hosts`, and system TLS certificates) to support secure network client operations.
* **Network**: Fully bridged to the host network namespace to support outbound client connections.

### Host Sandbox (`host` profile)
* **Filesystem & Process Execution**: Bind-mounts host system paths (including `/usr`, `/bin`, `/lib`, and `/lib64`) read-only into the sandbox. This allows the script to execute host command-line utilities (like `uname`, `git`, or `psql`) inside the isolated container.
* **Network**: Fully bridged to the host network namespace.
* **Isolation Guard**: Although host binaries can be run, they execute completely isolated within the sandbox's namespaces. For example, host process lists are invisible, and the UTS namespace is isolated (causing `hostname()` to return an empty string rather than the host's actual hostname).

For detailed architecture diagrams, OCI specifications, and AppArmor configuration steps, see the [Execution Sandbox Guide](../fundamentals/security/sandbox.md).
