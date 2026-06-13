---
title: "Permission"
description: "Understanding Starkite's rule-based permission system"
weight: 20
---

# Script Permissions

Starkite is shipped with a permission engine that can be configured to provide execution guardrails when running scripts. The engine intercepts every privileged module call (fs write, network connect, command exec, etc) and matches it against a rule set that determines *which operations a script may invoke*.

For OS-level isolation (filesystem visibility, process containment), see [Sandbox](sandbox.md). The two compose cleanly — pair both for any untrusted script.

## Default: deny-all

Starkite scripts run without permissions by default. When you run a script, without specifying a permission profile, the script automatically runs using the `deny-all` permission profile with the following capabilities:

- Ability to invoke pure computation function calls
- Access to functions `load()`, `print` and `log`

At that permission level, a script cannot access resources such as filesystem, network, environment, and exec processes until the script is granted a higher permission profile.

```bash
kite ./script.star            # runs using deny-all permission profile
```

## Permission quick start
Starkite comes with several built-in permission profiles that you can specify at runtime using `--permissions=<profile>` flag:

```bash
kite ./script.star --permissions=allow-local
```

There are five built-in profiles shown below in increasing permissiveness.

| Profile | Additionally | Key grants at this rung |
|---|---|---|
| `deny-all` | — (baseline) | pure-compute modules (`strings`, `json`, `yaml`, `time`, `hash`, …) plus `print` and `log`. No external resource access. |
| `allow-fs` | local files & env | `fs.read` (any file); `fs.write` / `fs.delete` within `$CWD`; `os.env`; `io.prompt`; `sql.open` (SQLite). No network. |
| `allow-net` | low-level protocol net | `http.client`, all `ssh` (`ssh.connect`, `ssh.transfer`), `sql.open` (Postgres, MySQL). |
| `allow-local` | `$CWD` exec, services | `os.exec` under `$CWD`, `http.*` serving, `k8s` read/write/config, `mcp.*` and `ai.*` access. |
| `allow-all` | reads/writes & exec anywhere | Unrestricted access to all module functionalities, including `os.exec` anywhere, `k8s.exec`, and process control. |

Each built-in profile has a boolean alias — `--allow-fs`, `--allow-net`, `--allow-local`, `--allow-all`, `--deny-all` — equivalent to `--permissions=<profile>`:

```bash
kite ./script.star --allow-all       # same as --permissions=allow-all
```

## Custom permission profiles

Starkite users can define custom permission profiles in the Starkite configuration file `~/.starkite/config.yaml` under the `permissions:` map. Each entry specifies a capability set that is granted or denied if it matches an `allow` or `deny` rule respectively. A Starkite permission has the following form:

```yaml
permissions:
  profile-name: {allow: ["allowed list"], deny: ["deny list"]}
```

As an example, the following permissions section of config.yaml defines 3 permissions:

- `ci` - with one `allow` rule
- `deploy` - A more elaborate profile with both `allow` and `deny` rules

```yaml
# ~/.starkite/config.yaml
permissions:
  ci:      { allow: ["k8s.read"] }
  deploy:
    allow:
      - fs.read($CWD/**)
      - fs.write($CWD/build/**)
      - os.exec($CWD/**)
      - http.client
    deny:
      - http.client(*.internal.*)
```

At runtime, a profile can be specified by name using the flag `--permissions=<name>`:

```bash
kite ./build.star    --permissions=deploy
kite ./ci-task.star  --permissions=ci
```
### Defining a default profile

When `config.yaml` contains a permission profile named `default`, it is applied automatically at runtime.

```yaml
# ~/.starkite/config.yaml
permissions:
  default: { allow: ["fs.read", "http.client"] }
```

When the following script is executed without a specified permission, Starkite will automatically apply the `default` permission profile defined in the `config.yaml` above:

```bash
kite ./build.star
```

### Composing from a base

A profile can start from a built-in profile and append rules. `base` names the built-in; `allow` and `deny` extend its lists, and the built-in's default behavior is kept. Deny-first evaluation is unchanged: an appended `deny` carves an exception out of the base's grants, and a base `deny` cannot be re-allowed.

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

A profile whose value is a bare name is shorthand for `base` with no additions — convenient for setting the implicit default from a built-in:

```yaml
# ~/.starkite/config.yaml
permissions:
  default: allow-fs        # = { base: allow-fs }; unflagged runs get this ceiling
  everything: allow-all    # selectable as --permissions=everything
```

`base` (and the bare-name shorthand) accepts built-in profile names only — profiles cannot compose on other config profiles.

## Permission rule grammar

The Starkite permissions rules are expressed using a uniform grammar of the form:

```
rule     := module "." category [ "(" [ funclist ":" ] resource ")" ]
funclist := func_name ("," func_name)*
```

Where:

- `module` - the name of a module (built-in or loaded)
- `category` - the name of a category of functionalities (e.g. `read`) against a resource
- `funclist` - list of exact function names that match the applied rule (e.g. `read_file`)
- `:` - separator when functions are applied to a set of resources
- `resource` - a resource name or path used in the permission

The function list and `resource` are both optional. When both appear, they are separated by `:`.

The following shows some example permission rules.

| Pattern | Matches |
|---|---|
| `*.*` | every module, every category, every operation |
| `fs.*` | every category in `fs` |
| `fs.read` | any function in `fs.read`, any resource |
| `fs.read(/etc/**)` | any function in `fs.read`, resource matching glob |
| `fs.read(read_file:*)` | only the `read_file` function, any resource |
| `fs.read(read_file,read_bytes:/etc/**)` | either function, resource matching glob |
| `os.exec($CWD/**)` | exec of any binary resolving to a path under `$CWD` |

During execution, `deny` rules are evaluated first followed by `allow` rules. Unmatched rules are denied.

The contents inside parentheses are parsed as a function list only when they consist of bare identifiers separated by commas, followed by `:`. Otherwise they are treated as a resource pattern:

```
fs.read(/etc/**)              → resource: /etc/**
fs.read(read_file:*)          → functions: [read_file], resource: *
fs.read(read_file,glob:/x/*)  → functions: [read_file, glob], resource: /x/*
fs.read(/some,path:with-colon)→ resource: /some,path:with-colon  (no valid funclist prefix)
```

### Path expansion

Starkite permission rules support path expansions and glob suffixes. Special rule variables `$CWD` and `$HOME` expand at startup using the process's working directory and the user's home as shown in the permissions snippet below:

```yaml
allow:
  - fs.read($CWD/**)               # any file under the project directory
  - fs.read($HOME/.config/myapp/*) # any file under the specified user path
```

When a rule path does not include either of these special prefixes, paths are matched verbatim.

## Modules and categories

The following are modules and categories that are checked by the permission engine at runtime. The majority of the standard library modules (strings, data encoding, math, time, regexp, templates, etc.) are compute only functions and are not checked.

| Module | Categories | What's checked |
|---|---|---|
| `fs` | `read`, `write`, `delete` | path access |
| `os` | `exec`, `env`, `process` | command execution, env reads/writes, chdir/exit |
| `http` | `client`, `server` | outgoing HTTP, listening servers |
| `ssh` | `connect`, `transfer` | remote exec, SCP up/down |
| `k8s` | `read`, `write`, `exec`, `config` | API access, kubectl-exec, kubeconfig load |
| `sql` | `open` | database connections (scoped by driver, e.g. `sqlite`, `postgres`, `mysql`) |
| `ai` | `generate` | LLM calls (model name as resource) |
| `mcp` | `client`, `server` | MCP connections + servers |
| `io` | `prompt` | interactive prompts |

## Loaded modules

Code reached through `load()` — a local directory module or an installed module — runs under the same runtime permission as its containing script. A module dependency that needs more permissions than were granted to its hosting script will fail. For example, a script run with `--allow-fs` that loads a module which calls `k8s.read` will fail:

```
kite ./deploy.star --allow-fs
#   deploy.star reads/writes local files   → allowed
#   the loaded module calls k8s.read(...)  → denied (k8s is allow-local)
```

The fix is to rerun the script with an elevated permission profile:

```
kite ./deploy.star --allow-local
```

## Augmenting security with `--sandbox`

The permission level of a script can be combined with Starkite's sandbox for additional security:

```bash
kite ./untrusted.star --sandbox=opaque --permissions=deny-all
```

The `--sandbox` flag causes Starkite to confine access to resources (filesystem, processes, network) at the kernel level via gVisor. See [Sandbox](sandbox.md).