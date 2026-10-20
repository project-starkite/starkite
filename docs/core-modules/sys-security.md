---
title: "Security"
description: "Process execution gating, privilege enforcement, and OS sandbox boundaries"
weight: 4
---

# Execution Security & Process Isolation

Starkite applies two complementary layers of protection to ensure that script execution and subprocess spawning remain bounded and predictable.

---

## 1. Interpreter-Level Execution Protection

When a script attempts to execute an operating system command (`os.exec()`, `os.try_exec()`, `os.shell()`), the Starkite interpreter intercepts the call and validates the target binary against the active capability profile:

* **Zero-Trust Default**: Subprocess execution is blocked by default under the `deny-all` and `sandbox-path` profiles (`exec.deny: ["*"]`, `no_exec: true`).
* **Binary Allowlisting**: Under `sandbox-exec` and higher (or when using `--sandbox-exec-allow=<bins>`), binaries are checked against allowed execution rules. Passing `--sandbox-exec-allow=kubectl,helm` resolves the binaries across `$PATH` and follows symlink chains into read-only mount rules.
* **Process Permissions**: All process execution (including explicit shell invocations like `os.exec("sh", ["-c", "..."])`) is governed uniformly by standard `os.exec` permission rules.
* **Identity Switching (POSIX)**: Executing a command under a specific user or group identity (using the `userid` and `groupid` optional keyword arguments) is a privileged operation requiring the `unconfined` profile or explicit `os.exec(switch_identity:...)` capability grant.

For instructions on defining custom execution rules and composing profiles, see the [Script Permissions Guide](../fundamentals/security/permission.md).

---

## 2. OS-Level Process Sandboxing

Starkite enables OS-level sandboxing by default across native OS kernel primitives (Linux Landlock + Seccomp-BPF, macOS Seatbelt) and container runtimes (Podman, Docker, nerdctl, gVisor).

Every script runs inside explicit filesystem, network, and execution boundaries defined by its active profile:

### Default Zero-Trust (`deny-all` profile)
* **Filesystem**: Isolated. `$CWD` and host `$HOME` are not mounted. An isolated temporary scratch mount is provided at `/tmp` (`HOME=/tmp`, `TMPDIR=/tmp`).
* **Subprocess Execution**: Blocked at both interpreter and OS kernel boundary.
* **Network**: Disabled (`network: none`). Outbound network requests fail closed.
* **Environment**: Scrubbed to minimal safe baseline (`PATH`, `HOME`, `TMPDIR`, `LANG`, `TERM`).

### Filesystem Processing (`sandbox-path` profile)
* **Filesystem**: `$CWD` mounted read/write with `/tmp` scratch mount.
* **Subprocess Execution**: Blocked.
* **Network**: Disabled (`network: none`).

### Local Execution (`sandbox-exec` profile)
* **Filesystem**: `$CWD` mounted read/write with `/tmp` scratch mount.
* **Subprocess Execution**: Permitted for system binaries (`exec.allow: ["*"]`).
* **Network**: Disabled (`network: none`).

### Cloud-Native & Network Workflows (`sandbox-net` profile / `--sandbox-net`)
* **Filesystem**: `$CWD` mounted read/write, `/tmp` scratch mount, and automatic read-only mounts for system DNS resolvers (`/etc/resolv.conf`, `/etc/hosts`) and TLS CA certificates.
* **Subprocess Execution**: Permitted for system binaries.
* **Network**: Full outbound network connectivity (`network: host`).

For driver configuration, driver override flags (`--sandbox-driver=<driver>`), and programmatic Starlark module execution, see the [Execution Sandbox Guide](../fundamentals/security/sandbox.md).
