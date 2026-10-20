---
title: "Permission"
description: "Understanding Starkite's rule-based permission system"
weight: 20
---

# Script Permissions

Starkite features a rule-based permission engine that restricts execution capabilities at runtime. The engine intercepts privileged operations—such as filesystem access, network connections, subprocess execution, and environment queries—and validates them against the active sandbox profile.

While kernel sandbox drivers provide OS-level process containment, Starlark permissions operate at the language and API level, enforcing granular authorization before operations reach the operating system.

## Default Zero-Trust Profile

By default, scripts execute under the `deny-all` profile. In this mode, the script can run pure computations and call basic utilities like `load()`, `print()`, and `log()`. Any attempt to access files, the network, environment variables, or external processes is blocked.

```bash
kite run ./script.star            # Runs under the default deny-all profile
```

## Built-in Capability Profiles

Starkite includes five built-in profiles forming a capability ladder, selectable via the `--sandbox-profile` flag:

| Profile | Key Grants |
|---|---|
| `deny-all` (default) | Pure computation (`strings`, `json`, `yaml`, `time`), plus `load()`, `print()`, `log()`. |
| `sandbox-path` | Filesystem read/write within `$CWD`, `/tmp` scratch, environment variables, SQLite. |
| `sandbox-exec` | `sandbox-path` grants + local command execution (`os.exec`, `os.shell`). |
| `sandbox-net` | `sandbox-exec` grants + HTTP client/server, SSH connections, Kubernetes, MCP, containers. |
| `unconfined` | Unrestricted execution anywhere on the host, system process controls, full capabilities (`--sandbox-enabled=false`). |

Example:

```bash
kite run ./script.star --sandbox-profile=sandbox-path
kite run ./script.star --sandbox-profile=sandbox-exec
kite run ./script.star --sandbox-profile=sandbox-net
kite run ./script.star --sandbox-enabled=false        # Run unconfined
```

## Synthesizing Permissions from Sandbox Settings

In Starkite, high-level sandbox settings automatically synthesize corresponding Starlark permission rules:

- `paths.read`: Automatically generates `fs.read` rules for each allowed directory or file.
- `paths.write`: Automatically generates `fs.read`, `fs.write`, and `fs.delete` rules.
- `exec.allow`: Automatically generates `os.exec` allow rules for permitted binaries.
- `exec.deny`: Automatically generates `os.exec` deny rules.
- `network: host` (or `--sandbox-net`): Automatically permits network operations across `http.client`, `http.server`, `ssh`, `k8s`, and `mcp`.

For fine-grained function- or resource-level authorization, configure `calls.allow` and `calls.deny` in `config.yaml` or use CLI flags (`--sandbox-call-allow`, `--sandbox-call-deny`).

## Custom Profiles in `config.yaml`

Define custom profiles in `~/.starkite/config.yaml` or `./config.yaml` under the `sandbox:` section:

```yaml
# ~/.starkite/config.yaml
sandbox:
  ci:
    calls:
      allow: ["k8s.read"]
  deploy:
    network: host
    paths:
      read: ["."]
      write: ["./build"]
    exec:
      allow: ["kubectl", "helm"]
    calls:
      allow:
        - http.client
      deny:
        - http.client(*.internal.*)
```

At runtime, select the profile by name:

```bash
kite run ./deploy.star --sandbox-profile=deploy
```

### Implicit Default Profile

To customize default privileges applied when no `--sandbox-profile` flag is passed, define a profile named `default`:

```yaml
# ~/.starkite/config.yaml
sandbox:
  default:
    paths:
      read: ["."]
    calls:
      allow: ["http.client"]
```

When running scripts without specifying a profile, the runtime automatically applies this default configuration:

```bash
kite run ./script.star        # Executes with working directory read and http.client capabilities
```

### Profile Composition

Custom profiles can inherit from built-in profiles using `base`. Appended rules extend base capabilities, and `deny` rules always take precedence:

```yaml
# ~/.starkite/config.yaml
sandbox:
  deploy:
    base: sandbox-path
    network: host
    calls:
      allow: ["k8s.write"]
      deny: ["fs.delete"]
```

You can also assign a built-in profile name directly as a string value as shorthand:

```yaml
# ~/.starkite/config.yaml
sandbox:
  default: sandbox-path        # Implicit default is sandbox-path
```

## Rule Grammar

Starkite permission rules use a structured format:

`module.category[([function_list:]resource)]`

* **module**: Target module name (e.g., `fs`, `os`, `http`).
* **category**: Target category name (e.g., `read`, `write`, `exec`, `client`).
* **function_list**: Optional comma-separated list of specific function names.
* **resource**: Optional path, host pattern, or glob.

### Common Rule Patterns

| Pattern | Matches |
|---|---|
| `*.*` | Any module, category, and operation |
| `fs.*` | Any operation in the `fs` module |
| `fs.read` | Any read operation in the `fs` module on any path |
| `fs.read(/etc/**)` | Any read operation in the `fs` module on paths matching `/etc/**` |
| `fs.read(read_file:*)` | Only the `read_file` function on any path |
| `fs.read(read_file,read_bytes:/etc/**)` | `read_file` or `read_bytes` on paths matching `/etc/**` |
| `os.exec($CWD/**)` | Execution of any binary located under `$CWD` |

Evaluation is **deny-first**. Any operation not explicitly matched by an `allow` rule (or blocked by a `deny` rule) is denied.

### Path Expansion

Rule paths support `$CWD` and `$HOME` environment variables, expanding them to the current working directory and user home directory respectively. Standard globbing (`*`, `**`) applies:

```yaml
calls:
  allow:
    - fs.read($CWD/**)               # Read any file under the project directory
    - fs.read($HOME/.config/myapp/*) # Read files under the specified user path
```

Without these variables, paths are matched verbatim.

## Checked Modules and Categories

The permission engine only checks modules that access host resources. Pure-compute modules (such as `strings`, `math`, `yaml`, or `json`) bypass the permission engine.

| Module | Categories | Gated Operations Checked |
|---|---|---|
| `fs` | `read`, `write`, `delete` | Path reads, writes, and deletions |
| `os` | `exec`, `shell`, `env`, `process` | Direct process execution, shell process execution, env reads/writes, directory/process controls |
| `http` | `client`, `server` | Outbound HTTP requests, listening HTTP servers |
| `ssh` | `connect`, `transfer` | Remote SSH execution, file uploads/downloads (SCP) |
| `k8s` | `read`, `write`, `exec`, `config` | Cluster reads/writes, kubectl exec, kubeconfig loading |
| `sql` | `open` | Database connections (scoped by driver, e.g., `sqlite`, `postgres`) |
| `mcp` | `client`, `server` | MCP connections and hosting |
| `io` | `prompt` | Terminal user prompts |

## Identity Switching (POSIX)

Starkite supports executing command-line processes under specified user or group credentials in POSIX environments by passing `userid` and `groupid` to process execution functions (`os.exec()`, `os.try_exec()`).

On non-Windows platforms, Starkite maps the `userid` and `groupid` parameters to Go's `syscall.Credential` struct and attaches it to the command's `SysProcAttr`:

```go
cred := &syscall.Credential{Uid: uid, Gid: gid}
cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
```

On Windows platforms, credential switching is unsupported; attempting to pass `userid` or `groupid` will immediately return an execution error.

Because switching process credentials is a sensitive operation, the interpreter validates identity-switching requests against a dedicated permission rule. The script must be granted the `switch_identity` capability for the target binary:

```yaml
sandbox:
  deploy:
    calls:
      allow:
        - os.exec(switch_identity:/usr/bin/git)
```

By default, the **`unconfined`** profile authorizes identity switching for execution.

## Loaded Modules and Trust Boundaries

When you import a module using `load()`, it runs under the exact same permissions as the calling script. Loading external code does not bypass or expand the script's permission boundary. If an imported module attempts any operation not permitted by the calling script's profile, the runtime blocks the action and raises an actionable permission denial error.

Furthermore, third-party modules declaring required capabilities in `mod.yaml` must not exceed operator grants. If a module's declared requirements exceed the active profile, execution halts before code runs with an actionable hint:

```bash
kite run ./app.star
# If a loaded module requires network: host under the default profile:
# Error: module "acme/netmod" requires capabilities exceeding active profile "default":
#   - network: host (active: none)
#   Hint: rerun with required capabilities: kite run --sandbox-net ./app.star
```
