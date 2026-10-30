---
title: "Sandbox"
description: "Zero-trust OS-level and container runtime isolation"
weight: 30
---

# Execution Sandbox

Starkite enforces zero-trust execution by default. Every script executed via `kite run`, `kite exec`, or `kite test` runs inside an OS-level kernel boundary and Starlark capability gate under the `deny-all` profile. Confined scripts are isolated from host credentials, unauthorized filesystems, network egress, and ambient host environment variables.

## Default Zero-Trust Confinement

When you invoke Starkite, the execution sandbox engages automatically (`--sandbox-enabled=true`). You do not need to pass an opt-in flag to secure script execution.

Under the default `deny-all` profile, Starkite guarantees the following runtime invariants:

- **Network Egress Blocked**: The sandbox isolates network namespaces or installs socket filtering (`network: none`), preventing outbound network requests.
- **Filesystem & Working Directory Confinement**: The working directory (`$CWD`) and host `$HOME` are not mounted into the sandbox. Scripts run with an isolated `rw,noexec` temporary scratch directory mounted at `/tmp` (`HOME=/tmp`, `TMPDIR=/tmp`).
- **Subprocess Execution Lockdown**: Subprocess spawning is disabled (`exec.deny: ["*"]`, `no_exec: true`). Attempts to execute binaries on the host fail immediately.
- **Environment Sanitization**: Ambient host environment variables are scrubbed. The script receives only a minimal, safe baseline (`PATH`, `HOME=/tmp`, `TMPDIR=/tmp`, `LANG=C.UTF-8`, `TERM`).
- **Memory-Resident Closures**: Local `.star` files and modules loaded via top-level `load()` statements are resolved and pre-loaded into memory before sandbox initialization, allowing modular scripts to execute without requiring filesystem mount permissions.

## Supported Sandbox Drivers

Starkite isolates processes using native operating system primitives or container runtimes:

| Driver | Platform | Mechanism | Overhead |
|---|---|---|---|
| `landlock` | Linux | Unprivileged Landlock kernel syscalls (`landlock_create_ruleset`) and Seccomp-BPF filters | In-process (0 subprocess overhead) |
| `seatbelt` | macOS | Dynamic Seatbelt binding (`sandbox_init` with SBPL profiles) | In-process (0 subprocess overhead) |
| `windows` | Windows | AppContainer token isolation, Job Object limits, NTFS DACLs, and WFP socket filtering | Process-isolation (outer broker / inner worker) |
| `podman` | Linux, macOS, Windows | Ephemeral rootless OCI container with auto-bind mounts | Low subprocess overhead |
| `docker` | Linux, macOS, Windows | Ephemeral Docker container execution | Low subprocess overhead |
| `nerdctl` | Linux, macOS | Ephemeral containerd runtime execution | Low subprocess overhead |
| `gvisor` | Linux | Application kernel sandboxing via `runsc` (Sentry) | Container / MicroVM overhead |

When invoked without an explicit `--sandbox-driver`, Starkite auto-detects the host driver: `landlock` on Linux, `seatbelt` on macOS, `windows` on Windows, or a configured container runtime if native drivers are unavailable.

## Built-in Capability Ladder

Privileges are organized into a five-rung capability ladder, where each profile is a strict superset of the prior:

| Profile | Network | Filesystem (`$CWD`) | Subprocess Execution | Purpose |
|---|---|---|---|---|
| `deny-all` (default) | `none` | Isolated (`[]`) | Blocked (`no_exec: true`) | Pure computation, print, and log. Top-level `load()` closures are pre-loaded into memory automatically. |
| `sandbox-path` | `none` | `$CWD` read/write, `/tmp` scratch | Blocked (`no_exec: true`) | Local file processing, code generation, and SQLite databases within the working directory. |
| `sandbox-exec` | `none` | `$CWD` read/write, `/tmp` scratch | Allowed (`exec.allow: ["*"]`) | Running local build tools, compilers, and CLI utilities without network access. |
| `sandbox-net` | `host` | `$CWD` read/write, `/tmp` scratch + DNS/TLS certs | Allowed (`exec.allow: ["*"]`) | Full cloud-native workflows (`http`, `ssh`, `k8s`, `mcp`, `containers`). |
| `unconfined` | `host` | Unrestricted host filesystem | Unrestricted | Completely disables OS sandbox confinement and Starlark permission checks (`--sandbox-enabled=false`). |

## Runtime Environment Profiles

For executing interpreted scripts and language runtimes under native process execution (`kite exec`) or script evaluation, Starkite provides pre-configured runtime profiles. These profiles build on `deny-all` isolation, dynamically resolving and mounting language-specific package stores and virtual environments while keeping host credentials strictly inaccessible:

| Profile | Base | Runtime | Auto-Resolved Mounts | Credential Protection |
|---|---|---|---|---|
| `python-env` | `deny-all` | Python | Active `$VIRTUAL_ENV`, `./.venv`, `./venv`, system library paths | Blocks `~/.ssh`, `~/.aws`, `~/.kube`, `~/.gnupg` |
| `node-env` | `deny-all` | Node.js | Project `./node_modules`, global `$NVM_BIN` modules, system module paths | Blocks `~/.ssh`, `~/.aws`, `~/.kube`, `~/.gnupg` |
| `ruby-env` | `deny-all` | Ruby | `$GEM_HOME`, `$GEM_PATH`, system gem libraries | Blocks `~/.ssh`, `~/.aws`, `~/.kube`, `~/.gnupg` |

### Path Dependency Resolution & Home Credential Protection

When a runtime profile is active, Starkite scans the environment to unveil standard interpreter libraries and project dependencies read-only:

- **Python Virtual Environments**: If `$VIRTUAL_ENV` is set, or if `./.venv` or `./venv` exists in the working directory, Starkite mounts the directory read-only into the sandbox. System library locations (`/usr/lib/python3*`, `/opt/homebrew/lib/python3*`, or Windows `%LOCALAPPDATA%\Programs\Python`) are detected and unveiled.
- **Node.js Modules**: Local `./node_modules` and global `$NVM_BIN` package directories are mounted read-only.
- **Ruby Gems**: Gem directories declared in `$GEM_HOME` and `$GEM_PATH` are mounted read-only.

**Credential Invariant**: Even if a virtual environment or project directory is situated within your home directory, Starkite enforces an inviolable credential barrier. Sensitive paths including `~/.ssh`, `~/.aws`, `~/.kube`, `~/.gnupg`, `~/.config/gcloud`, `~/.docker`, and `~/.gitconfig` are excluded from auto-mounts and denied.

Select a profile with `--sandbox-profile=<name>`:

```bash
kite run ./calc.star                                       # Runs under default 'deny-all'
kite run ./process_files.star --sandbox-profile=sandbox-path
kite run ./build_project.star --sandbox-profile=sandbox-exec
kite run ./deploy_stack.star --sandbox-profile=sandbox-net
kite run ./untrusted.star --sandbox-enabled=false         # Disables sandbox (runs unconfined)
```

## Composable CLI Capability Flags

Rather than switching whole profiles, you can selectively grant specific capabilities from the command line on top of the zero-trust default:

| CLI Flag | Description |
|---|---|
| `--sandbox-path-read=<paths>` | Grants read access to specific host paths (comma-separated or repeatable). |
| `--sandbox-path-write=<paths>` | Grants read and write access to specific host paths (comma-separated or repeatable). |
| `--sandbox-exec-allow=<bins>` | Permits execution of specific binary names, absolute paths, or `*`. |
| `--sandbox-exec-deny=<bins>` | Denies execution of specific binaries (takes precedence over allow rules). |
| `--sandbox-call-allow=<rules>` | Permits specific Starlark API module calls (e.g. `fs.read`, `http.client`). |
| `--sandbox-call-deny=<rules>` | Denies specific Starlark API module calls. |
| `--sandbox-net-allow[=<target>]` | Grants outbound network access to endpoint(s), or `*` / bare flag for unrestricted host networking. |
| `--sandbox-net-deny[=<target>]` | Denies outbound network access to endpoint(s), or `*` / bare flag for complete air-gapping. |
| `--sandbox-net-mode=<mode>` | Selects network confinement mode (`host`, `loopback`, `none`). |
| `--sandbox-max-memory=<limit>` | Sets maximum memory ceiling for sandboxed process (e.g. `512MB`, `2GB`). |
| `--sandbox-driver=<driver>` | Selects the sandbox driver (`landlock`, `seatbelt`, `podman`, `docker`, `nerdctl`, `gvisor`, `auto`). |
| `--sandbox-image=<image>` | Specifies the OCI image when using a container sandbox driver. |
| `--sandbox-enabled=false` | Disables sandbox confinement and runs unconfined. |

Example compositions:

```bash
# Grant working directory read/write access
kite run ./generate.star --sandbox-path-write=.

# Grant specific CLI tools and network egress
kite run ./deploy.star --sandbox-exec-allow=kubectl,helm --sandbox-net-allow

# Grant working directory read and selective outbound HTTP
kite run ./fetch.star --sandbox-path-read=. --sandbox-net-allow
```

### Runtime Usability Automations

Starkite eliminates common sandbox friction through automated runtime analysis:

1. **Static AST `load()` Closure Pre-Loading**: Local `.star` scripts referenced by top-level `load()` statements are discovered, parsed, and cached in memory before the sandbox boundary is locked down. Scripts can import helper libraries and modules without needing filesystem read grants for `$CWD`.
2. **Automatic Binary & Symlink Unveiling**: When you specify allowed binaries via `--sandbox-exec-allow=kubectl,go`, Starkite searches `$PATH`, resolves symlink chains (such as package manager links in `/opt/homebrew` or `/usr/local`), and mounts the underlying binary and its required directories read-only without exposing unauthorized paths.
3. **Automatic Network Resolution Assets**: When you enable network access (`--sandbox-net-allow` or `network: host`), the sandbox automatically unveils system DNS resolvers and TLS CA certificate bundles (`/etc/resolv.conf`, `/etc/hosts`, `/etc/ssl/certs`, `/etc/ca-certificates`, and macOS `/private/etc/ssl`).

## Environment Isolation & Forwarding

Starkite sanitizes the process environment so host secrets cannot leak into scripts or subprocesses. You control environment variable visibility using a 3-tier synthesis model:

1. **Selective Host Forwarding**: Forward an ambient variable from the host environment:
   ```bash
   kite run ./deploy.star --env KUBECONFIG -e AWS_REGION
   ```
2. **Explicit Assignment**: Inject specific key-value pairs without reading the host environment:
   ```bash
   kite run ./deploy.star --env DEPLOY_ENV=production -e REPLICAS=5
   ```
3. **Full Host Inheritance**: Forward the complete ambient host environment when required:
   ```bash
   kite run ./local_tool.star --env-inherit
   ```

## Custom Profiles in `config.yaml`

Define reusable sandbox profiles in `~/.starkite/config.yaml` or `./config.yaml`:

```yaml
# ~/.starkite/config.yaml
sandbox:
  # Default overrides applied when no profile is explicitly passed
  default:
    paths:
      read: ["."]

  # Deployment profile for cloud automation
  deploy:
    network: host
    paths:
      read: ["."]
      write: ["./build", "./dist"]
    exec:
      allow: ["kubectl", "helm"]
    calls:
      allow: ["k8s.write", "http.client"]
    env:
      KUBECONFIG:
      DEPLOY_STAGE: production

  # Strictly isolated compute profile
  isolated:
    network: none
    paths:
      read: ["/data/readonly"]
    no_exec: true
    max_memory: "512MB"
    timeout: "60s"
```

Activate a configured profile using `--sandbox-profile`:

```bash
kite run ./deploy.star --sandbox-profile=deploy
```

## Actionable Denial Diagnostics

When a script or subprocess attempts an action prohibited by the sandbox or permission layer, Starkite formats an actionable diagnostic error indicating the exact CLI flag or configuration setting required to grant the privilege:

```text
Error: script error: permission denied: fs.read read_file("./config.yaml") - no matching allow rule
  grant "fs.read" with --sandbox-profile=sandbox-path (or higher), or in the sandbox: section of config.yaml
  Hint: pass --sandbox-path-read=. on CLI, or add "." to sandbox.<profile>.paths.read in config.yaml
```

```text
Error: script error: permission denied: os.exec exec("/usr/bin/kubectl") - blocked by deny rule: os.exec
  Hint: pass --sandbox-exec-allow=kubectl on CLI, or add "kubectl" to sandbox.<profile>.exec.allow in config.yaml
```

When running under `--debug`, Starkite also alerts you if a script requests an environment variable that exists on the host but was scrubbed by zero-trust sanitization:

```text
[DEBUG] env "KUBECONFIG" requested by script is present on host but scrubbed by sandbox.
  Hint: pass --env KUBECONFIG on CLI, or declare "KUBECONFIG:" in sandbox.<profile>.env in config.yaml
```

### Process Execution Diagnostics & Telemetry

When launching external processes with `kite exec`, Starkite inspects child exit codes and standard error to diagnose kernel sandbox denials versus application dependency failures:

- **Python Missing Modules**: If a Python command fails with `ModuleNotFoundError: No module named '<name>'`, Starkite appends remediation guidance:
  ```text
  Hint: pass --sandbox-profile=python-env on CLI, or add dependencies via --sandbox-path-read=<path>
  ```
- **Node.js Missing Modules**: If Node.js terminates with `Error: Cannot find module '<name>'`, Starkite suggests:
  ```text
  Hint: pass --sandbox-profile=node-env on CLI, or grant access via --sandbox-path-read=node_modules
  ```
- **Dynamic Linker Failures**: When an executable fails due to an unmounted shared library (`cannot open shared object file` or `image not found`), Starkite recommends:
  ```text
  Hint: grant read access to library location via --sandbox-path-read=<dir>
  ```
- **Isolated Network Failures**: When network connectivity fails under an isolated network profile:
  ```text
  Hint: pass --sandbox-net-allow on CLI, or add endpoint to sandbox.<profile>.network.allow in config.yaml
  ```

Under `--debug`, `kite exec` emits structured telemetry to standard error reporting command, exit code, execution duration, timeout status, and authoritative sandbox driver:

```text
[telemetry] command="python3 test.py" exit_code=0 duration=42.1ms timed_out=false driver=seatbelt
```

## Security Invariants

Starkite sandbox drivers enforce core kernel-level invariants across all execution profiles:

### Write-XOR-Execute (W^X)

All writable mount locations—including `$CWD`, `/tmp`, and custom writable paths—are mounted with `noexec` flags. A script cannot generate a binary or script inside a writable directory and execute it as a child process. Binaries must reside on immutable, read-only system paths.

### Subprocess Execution Lockdown

When running under profiles where `no_exec: true` is active (such as `deny-all` and `sandbox-path`), process execution syscalls (`SYS_EXECVE`/`SYS_EXECVEAT` in Linux Seccomp-BPF and `process-exec` in macOS Seatbelt) are trapped and blocked at the kernel boundary.

### Shared Memory & IPC Isolation

Host shared memory segments (`/dev/shm`, `/run/shm`) and System V IPC primitives (`shmget`, `semget`, `msgget`) are isolated. Container drivers run with `--ipc=private`, and Linux Landlock excludes shared memory devices from sandbox rulesets.

### Process Inspection & Signal Boundary

The Linux Seccomp-BPF filter prevents sandboxed processes from inspecting or modifying host processes (`SYS_PTRACE`, `SYS_PROCESS_VM_READV`, `SYS_PROCESS_VM_WRITEV`) and restricts signal delivery (`SYS_KILL`) strictly to processes within the sandbox process tree.

### Windows AppContainer & Job Object Isolation

On Windows, Starkite isolates processes using five coordinated Win32 kernel primitives:
- **Win32 Job Objects**: Enforces `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` for process tree termination on exit, alongside memory ceilings, CPU quotas, and UI isolation (`JOB_OBJECT_UILIMIT_ALL`).
- **Ephemeral AppContainer Profiles**: Derives a scoped AppContainer security identifier (`S-1-15-2-...`) for the session, executing child processes in an unprivileged execution context.
- **NTFS Discretionary Access Control (DACL)**: Surgically unmasks only the script file and explicit mount targets, leaving parent directories, `%USERPROFILE%`, and host credentials inaccessible.
- **Write-XOR-Execute ($W\oplus X$) Enforcement**: Temporary scratch directories grant read and write permissions while explicitly denying `FILE_EXECUTE` in a single DACL transaction.
- **Process Mitigations & WFP**: Blocks unauthorized subprocess creation when execution is disallowed and enforces network isolation through Windows Filtering Platform socket filters.

## Starlark `sandbox` Module API

Starkite also exposes programmatic sandbox management through the built-in `sandbox` Starlark module:

```python
# Create a programmatic sandbox configuration
box = sandbox.config(
    driver="auto",
    network="host",
    no_exec=False,
    allowed_exec_paths=["/bin/echo"],
    mounts=[
        {"source": ".", "destination": "/workspace", "mode": "rw"},
    ],
    timeout="30s",
)

# Run a subprocess within the configured sandbox
result = box.exec("echo", ["hello-from-sandbox"])
if result.ok:
    print(result.stdout)
```

### Non-Escalation Security Invariant

When running inside an active parent sandbox, child sandboxes created via `sandbox.config` or `sandbox.run_script` cannot elevate privileges beyond the parent boundary. The runtime enforces an intersection rule:

$$\text{Effective Permissions} = \text{Parent Permissions} \cap \text{Requested Permissions}$$

If the parent sandbox has `network: none`, child sandboxes cannot request `network: host`.
