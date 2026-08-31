---
title: "Permission"
description: "Understanding Starkite's rule-based permission system"
weight: 20
---

# Script Permissions

Starkite features a rule-based permission engine that restricts execution capabilities at runtime. The engine intercepts privileged operations—such as filesystem access, network connections, and command execution—and validates them against a permission profile.

For kernel-level isolation (such as filesystem invisibility and process containment), see [Sandbox](sandbox.md).

## Default profile

By default, scripts execute under the `deny-all` profile. In this mode, the script can run pure computations and call basic utilities like `load()`, `print()`, and `log()`. Any attempt to access files, the network, environment variables, or external processes is blocked.

```bash
kite ./script.star            # Runs under the deny-all profile
```

## Built-in permission profiles

Starkite includes five built-in profiles forming a capability ladder, selectable via the `--permissions` flag or its boolean aliases:

| Profile | Flag Alias | Key Grants |
|---|---|---|
| `deny-all` | `--deny-all` | Compute-only (`strings`, `json`, `yaml`, `time`), plus `load()`, `print()`, `log()` |
| `allow-fs` | `--allow-fs` | File reads, file writes within `$CWD`, environment variables, SQLite |
| `allow-net` | `--allow-net` | HTTP clients, SSH connections, remote SQL databases |
| `allow-local` | `--allow-local` | Local command execution (within `$CWD`), HTTP servers, Kubernetes, MCP, AI |
| `allow-all` | `--allow-all` | Unrestricted execution anywhere on the host, system process controls, full capabilities |

Example:

```bash
kite ./script.star --allow-fs        # Shorthand alias
kite ./script.star --permissions=allow-fs
```

## Custom permission profiles

Define custom profiles in `~/.starkite/config.yaml` under the `permissions` section. Each profile lists explicit `allow` and `deny` rules:

```yaml
# ~/.starkite/config.yaml
permissions:
  ci: { allow: ["k8s.read"] }
  deploy:
    allow:
      - fs.read($CWD/**)
      - fs.write($CWD/build/**)
      - os.exec($CWD/**)
      - http.client
    deny:
      - http.client(*.internal.*)
```

At runtime, select the profile by name:

```bash
kite ./build.star --permissions=deploy
```

### Implicit default profile

To define a default profile that Starkite applies when no `--permissions` flag is provided, name the profile `default`:

```yaml
# ~/.starkite/config.yaml
permissions:
  default: { allow: ["fs.read", "http.client"] }
```

When running scripts without specifying a permissions flag, the runtime automatically applies this default profile:

```bash
kite run ./script.star        # Executes with fs.read and http.client capabilities
```

### Profile composition

Custom profiles can inherit from built-in profiles using `base`. Appended rules extend base capabilities, and `deny` rules always take precedence:

```yaml
# ~/.starkite/config.yaml
permissions:
  deploy:
    base: allow-fs
    allow: ["k8s.write", "os.exec($CWD/**)"]
    deny: ["fs.delete"]
  almost-everything:
    base: allow-all
    deny: ["os.exec"]        # everything except process execution
```

You can assign a built-in profile name directly as a string value. This is a shorthand for inheriting the built-in profile verbatim, without adding any rules:

```yaml
# ~/.starkite/config.yaml
permissions:
  default: allow-fs        # Implicit default is allow-fs
  everything: allow-all    # Selectable via --permissions=everything
```

> [!NOTE]
> The `base` field accepts built-in profile names only. Profiles cannot inherit from other custom profiles.

## Rule grammar

Starkite permission rules use a structured format:

`module.category[([function_list:]resource)]`

* **module**: Target module name (e.g., `fs`, `os`).
* **category**: Target category name (e.g., `read`, `exec`).
* **function_list**: Optional comma-separated list of specific function names.
* **resource**: Optional path, host pattern, or glob.

### Common rule patterns

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

### Path expansion

Rule paths support `$CWD` and `$HOME` environment variables, expanding them to the current working directory and user home directory respectively. Standard globbing (`*`, `**`) applies:

```yaml
allow:
  - fs.read($CWD/**)               # Read any file under the project directory
  - fs.read($HOME/.config/myapp/*) # Read files under the specified user path
```

Without these variables, paths are matched verbatim.

## Checked modules and categories

The permission engine only checks modules that access host resources. Pure-compute modules (such as `strings`, `math`, or `json`) bypass the permission engine.

| Module | Categories | Grated Operations Checked |
|---|---|---|
| `fs` | `read`, `write`, `delete` | Path reads, writes, and deletions |
| `os` | `exec`, `shell`, `env`, `process` | Direct process execution, shell process execution, env reads/writes, directory/process controls |
| `http` | `client`, `server` | Outbound HTTP requests, listening HTTP servers |
| `ssh` | `connect`, `transfer` | Remote SSH execution, file uploads/downloads (SCP) |
| `k8s` | `read`, `write`, `exec`, `config` | Cluster reads/writes, kubectl exec, kubeconfig loading |
| `sql` | `open` | Database connections (scoped by driver, e.g., `sqlite`, `postgres`) |
| `ai` | `generate` | LLM calls (model name checked as a resource) |
| `mcp` | `client`, `server` | MCP connections and hosting |
| `io` | `prompt` | Terminal user prompts |

## Identity Switching (POSIX)

Starkite supports executing command-line processes under specified user or group credentials in POSIX environments by passing `userid` and `groupid` to process execution functions (`os.exec()`, `os.try_exec()`).

### Credential Configuration

On non-Windows platforms, Starkite maps the `userid` and `groupid` parameters to Go's `syscall.Credential` struct and attaches it to the command's `SysProcAttr`:

```go
cred := &syscall.Credential{Uid: uid, Gid: gid}
cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
```

On Windows platforms, credential switching is unsupported; attempting to pass `userid` or `groupid` will immediately return an execution error.

### Security Authorization

Because switching process credentials is a sensitive operation, the Starkite interpreter validates identity-switching requests against a dedicated permission rule. The script must be granted the `switch_identity` capability for the target binary:

```go
libkite.Check(thread, "os", permCategory, "switch_identity", execTarget)
```

By default, the **`allow-all`** profile authorizes identity switching for execution (`os.exec`/`os.try_exec`). In custom profiles, you can explicitly grant this capability:

```yaml
permissions:
  restricted-deploy:
    allow:
      - os.exec(switch_identity:/usr/bin/git) # Allow identity switching for git
```

## Loaded modules

When you import a module using `load()`, it runs under the exact same permissions as the calling script. Loading external code does not bypass or expand the script's permission boundary. If an imported module attempts any operation not permitted by the calling script's profile, the runtime blocks the action and raises a permission error.

```
kite ./deploy.star --allow-fs
#   deploy.star reads/writes local files   → allowed
#   the loaded module calls k8s.read(...)  → denied (k8s is allow-local)
```

To resolve this, rerun the script with the required capabilities:

```bash
kite ./deploy.star --allow-local
```