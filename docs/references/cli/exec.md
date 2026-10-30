---
title: "kite exec"
description: "Launch a native process or script in the sandbox"
weight: 2
---

Launch an arbitrary external native process or executable script directly under the Starkite kernel sandbox engine, completely bypassing the Starlark runtime.

`kite exec` runs pre-existing binaries, system tools, compiled executables, and shell scripts under the same kernel-level isolation drivers (Linux Landlock/Seccomp, macOS Seatbelt, Windows AppContainers) used for Starkite scripts.

## Usage

```bash
kite exec [flags] [--] <command> [args...]
```

## Security Boundary

The kernel sandbox is authoritative across the entire child process hierarchy:
* **Native Machine Code Isolation**: Restricts filesystem access, network egress, memory limits, and process capabilities at the OS kernel boundary.
* **Starlark Permissions Ignored**: Starlark language-level permissions (such as `--sandbox-call-*`) do not apply to native binaries; passing them produces an informational notice.

## POSIX `--` Delimiter

Use the POSIX `--` delimiter to cleanly separate Starkite runner flags from arguments intended for the child command:

```bash
# Flags before '--' configure the Starkite runner; flags after '--' are passed to curl:
kite exec --sandbox-net-allow -- curl -s -I https://example.com
```

When no `--` delimiter is present, Starkite parses known runner flags up to the first positional argument representing the command to run.

## Target Resolution Rules

When resolving `<command>`:

1. **Path References (`./`, `../`, `/`, or `.\`)**:
   - The file must exist and must not be a directory.
   - The file must have executable permission bits (`chmod +x` on POSIX systems; matching `PATHEXT` on Windows).
2. **Bare Command Names**:
   - Resolved automatically by searching directories in `$PATH`.
   - If not found in `$PATH`, execution halts with an error.

To execute a Starkite script (`.star`) or directory module (`mod.yaml`), use [`kite run`](run.md) instead.

## Stdio Streaming and Exit Codes

* **Zero-Overhead Streaming**: Standard input, standard output, and standard error streams are attached directly to your terminal. Interactive inputs, pipes, and colored terminal output function seamlessly.
* **Exact Exit Status**: `kite exec` exits with the exact status code returned by the child process (e.g., if a child exits with code `42`, `kite exec` terminates with exit code `42`).

## Examples

### Launch a System Utility Under Zero-Trust Isolation

```bash
kite exec /bin/echo "Running inside isolated sandbox"
```

### Outbound Network Egress with `curl`

```bash
kite exec --sandbox-net-allow -- curl -s https://api.github.com/zen
```

### Executing a Shell Script with Working Directory Access

```bash
kite exec --sandbox-path-write=. ./deploy.sh
```

### Forwarding Environment Variables

```bash
kite exec --env CI_COMMIT_SHA --env DEPLOY_ENV=staging ./scripts/release.sh
```

### Running Language Interpreters

```bash
kite exec --sandbox-net-allow -- python3 -c "import urllib.request; print(urllib.request.urlopen('https://example.com').status)"
```

### Running Python with Virtual Environment Access

```bash
kite exec --sandbox-profile=python-env -- python3 script.py
```

### Running Node.js with Dependency Modules

```bash
kite exec --sandbox-profile=node-env -- node app.js
```

## Runtime Environment Profiles

Starkite includes built-in profiles tailored for language ecosystems:

| Profile | Target Runtime | Baseline Isolation | Automatic Mounts |
|---------|----------------|---------------------|------------------|
| `python-env` | Python (`python3`, `python`) | `deny-all` | `$VIRTUAL_ENV`, `./.venv`, `./venv`, standard library and site-packages |
| `node-env` | Node.js (`node`, `npm`, `npx`) | `deny-all` | `./node_modules`, `$NVM_BIN`, global module paths |
| `ruby-env` | Ruby (`ruby`, `bundle`) | `deny-all` | `$GEM_HOME`, `$GEM_PATH`, system gem repositories |

### Credential Protection

Runtime profiles automatically discover and mount dependency trees read-only while shielding sensitive user paths in `$HOME`:
* Shielded paths: `.ssh`, `.aws`, `.kube`, `.gnupg`, `.config/gcloud`, `.docker`, `.gitconfig`.
* Dependency stores or virtual environments located inside shielded directories are blocked from automatic mounting.

### Execution Denial Diagnostics & Telemetry

When a command terminates with a non-zero exit status, Starkite analyzes standard error and appends actionable remediation guidance:
* **Missing Python Modules** (`ModuleNotFoundError`): Suggests passing `--sandbox-profile=python-env` or adding explicit paths via `--sandbox-path-read=<path>`.
* **Missing Node.js Modules** (`Cannot find module`): Suggests passing `--sandbox-profile=node-env` or adding `--sandbox-path-read=node_modules`.
* **Missing Shared Libraries** (`cannot open shared object file`, `image not found`): Suggests granting read access to library paths via `--sandbox-path-read=<dir>`.
* **Blocked Outbound Network Connections**: When running under network isolation (`--sandbox-net-deny` or default `none`), detects connection and DNS resolution failures and hints to pass `--sandbox-net-allow`.

Under `--debug`, `kite exec` emits structured execution telemetry to standard error reporting command, exit code, duration, timeout status, and driver:
```text
[telemetry] command="python3 script.py" exit_code=0 duration=25.4ms timed_out=false driver=landlock
```

## Flags

`kite exec` supports all standard sandbox runner flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--sandbox-profile name` | Built-in profile (`deny-all`, `python-env`, `node-env`, `ruby-env`, `sandbox-path`, etc.) or named profile in `config.yaml` | `""` |
| `--sandbox-driver driver` | Kernel isolation driver (`auto`, `landlock`, `seatbelt`, `windows`, `podman`, `docker`) | `""` |
| `--sandbox-path-read path` | Grant read access to host path (repeatable) | |
| `--sandbox-path-write path` | Grant read and write access to host path (repeatable) | |
| `--sandbox-net-allow` | Allow outbound network access | |
| `--sandbox-net-deny` | Deny outbound network access | |
| `--sandbox-max-memory limit` | Memory limit (e.g. `512MB`, `2GB`) | `""` |
| `--timeout seconds` | Process execution timeout | `300` |
| `--env, -e KEY[=VALUE]` | Forward host environment variable or inject assignment | |
| `--env-inherit` | Inherit full ambient environment | `false` |
| `--sandbox-enabled=false` | Disable sandbox confinement and run unconfined | `true` |
