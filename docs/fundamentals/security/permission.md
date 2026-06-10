---
title: "Permission"
description: "Rule-based gating of privileged module calls — what scripts may invoke"
weight: 20
---

# Permission

The permission engine intercepts every privileged module call (filesystem write, network connect, command exec, Kubernetes apply, LLM generate, …) and matches it against a rule set. It answers *which operations a script may invoke*. This page covers the default, the built-in profile ladder, custom profiles, the rule grammar, and how the engine composes with the sandbox.

The permission engine is pure Go: no kernel call, no namespace, no measurable overhead. Available on every platform that runs starkite.

For OS-level isolation (filesystem visibility, process containment), see [Sandbox](sandbox.md). The two compose cleanly — pair both for any untrusted script.

## Default: deny-all

Without `--permissions`, and with no `default` profile configured, a script runs under `deny-all`: it may perform pure computation plus `print` and `log`, and nothing else. Filesystem, network, environment, and exec are all denied until the script is granted a higher profile.

```bash
kite ./script.star            # deny-all unless a config default is set
```

To make a machine's everyday ceiling more permissive without a flag on every run, define a `default` profile in `~/.starkite/config.yaml` (see [Custom profiles](#custom-profiles)). An unspecified `--permissions` then resolves to that profile.

## Quick start

```bash
kite ./script.star --permissions=allow-fs           # read any file; write within $CWD; env
kite ./script.star --permissions=allow-local        # serve, $CWD exec, k8s, ai
kite ./script.star --permissions=allow-all          # unrestricted
kite ./script.star --permissions=./team.yaml#deploy # custom profile from a file
```

Each built-in profile has a boolean alias — `--allow-fs`, `--allow-net`, `--allow-local`, `--allow-all`, `--deny-all` — equivalent to `--permissions=<profile>`:

```bash
kite ./script.star --allow-fs        # same as --permissions=allow-fs
kite ./script.star --allow-all       # same as --permissions=allow-all
```

Set at most one permission selector: combining two aliases, or an alias with `--permissions=`, is an error.

A script launched directly via shebang carries no flag, so it runs under the configured `default` profile, or `deny-all` when none is set. To grant a different profile, invoke it as `kite run <script> --allow-fs` (or any selector above).

## Built-in profiles

The five built-in profiles form a strict capability ladder; each is a superset of the one below.

| Profile | Adds over previous | Cumulative capability |
|---|---|---|
| `deny-all` | — (baseline) | pure-compute modules (`strings`, `json`, `yaml`, `time`, `hash`, …) plus `print` and `log`. No external resource access. |
| `allow-fs` | local files & env | `fs.read` (any file); `fs.write` / `fs.delete` within `$CWD`; `os.env`; `io.prompt`. No network. |
| `allow-net` | low-level protocol net | `http.client`, all `ssh` (`ssh.connect`, `ssh.transfer`). |
| `allow-local` | serve, `$CWD` exec, services | `http.server`, `os.exec` of binaries under `$CWD`, `ai.generate`, `k8s.read`/`write`/`config`, `mcp.client`, `mcp.server`. |
| `allow-all` | unrestricted writes & exec | `fs.write` / `fs.delete` anywhere, `os.exec` of any binary, `k8s.exec` (pod shell), `os.process` (signal/kill). |

`allow-local` grants the full functional surface — files, low-level network, ai/k8s/mcp, serve, and exec scoped to `$CWD` — while withholding the capabilities that turn a script into arbitrary machine control (unrestricted `os.exec`, `k8s.exec`, process control). That withheld set is the line between `allow-local` and `allow-all`.

```bash
kite ./analyze.star --permissions=allow-fs
kite ./deploy.star  --permissions=allow-local
```

## Custom profiles

User-defined profiles live in `~/.starkite/config.yaml` under a `permissions:` map. Each entry is allow-list only: a capability is granted only if it matches an `allow` rule and no `deny` rule. There is no per-profile default — unmatched calls are denied.

```yaml
# ~/.starkite/config.yaml
permissions:
  default: { allow: ["fs.read", "http.client"] }        # this machine's everyday ceiling
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

Select a named profile with `--permissions=<name>`:

```bash
kite ./build.star    --permissions=deploy
kite ./ci-task.star  --permissions=ci
```

A profile named `default` is otherwise ordinary, with one special role: it is the implicit profile when `--permissions` is unspecified. A named profile that is not defined — including `default` when it has no entry — is an error.

### From a file

Pass a profile file by path. When the file holds exactly one profile, the `#name` fragment is optional; with multiple, select one explicitly:

```yaml
# team.yaml
permissions:
  deploy: { allow: ["fs.read($CWD/**)", "k8s.write"] }
  ci:     { allow: ["k8s.read"] }
```

```bash
kite ./build.star --permissions=./team.yaml#deploy
```

### Inline rules

For one-off invocations, pass rules directly. The value starts with `allow:` or `deny:`. Rules within a clause are comma-separated; clauses are separated by `;`. Inline rules layer on a `deny-all` baseline — anything not allowed is denied.

```bash
kite ./script.star --permissions=allow:fs.read
kite ./script.star --permissions='allow:fs.read,fs.write,os.exec($CWD/**)'
kite ./script.star --permissions='allow:fs.read($CWD/**);deny:http.client'
```

No profile-plus-rules composition is accepted on the command line (e.g. `allow-net` plus extra rules). Compose in a `config.yaml` profile instead.

## Rule grammar

A rule has the form:

```
module.category[(functions:resource)]
```

The functions list and resource are both optional. When both appear, they are separated by `:`.

| Pattern | Matches |
|---|---|
| `*.*` | every module, every category, every operation |
| `fs.*` | every category in `fs` |
| `fs.read` | any function in `fs.read`, any resource |
| `fs.read(/etc/**)` | any function in `fs.read`, resource matching glob |
| `fs.read(read_file:*)` | only the `read_file` function, any resource |
| `fs.read(read_file,read_bytes:/etc/**)` | either function, resource matching glob |
| `os.exec($CWD/**)` | exec of any binary resolving to a path under `$CWD` |

Deny rules are evaluated first, then allow rules; an unmatched call is denied.

`os.exec` resolves the invoked binary to its filesystem path and matches that path against the rule. A bare command name resolves via `PATH`. So `os.exec($CWD/**)` permits project-local tools but denies `/usr/bin/uname`; granting a system binary requires `allow-all` or an explicit path rule such as `os.exec(/usr/bin/**)`.

`fs.read`, `fs.write`, and `fs.delete` likewise resolve their target to an absolute path before matching, so a `$CWD`-scoped rule applies to relative paths too. This is why `allow-fs` reads any file but writes or deletes only within `$CWD`:

```bash
# allow-fs: read anywhere, write only inside the working tree
kite ./report.star --allow-fs
#   read_text("/etc/hosts")        → allowed (read is unscoped)
#   path("out.json").write_text(…) → allowed (relative path is under $CWD)
#   path("/tmp/x").write_text(…)   → denied  (outside $CWD; needs allow-all)
```

### Path expansion

`$CWD` and `$HOME` expand at startup using the process's working directory and the user's home:

```yaml
allow:
  - fs.read($CWD/**)        # any file under the project directory
  - fs.read($HOME/.config/myapp/*)
```

Resources without these prefixes are matched verbatim against globs.

### Function lists vs resources

The contents inside parentheses are parsed as a function list only when they consist of bare identifiers separated by commas, followed by `:`. Otherwise they are treated as a resource pattern:

```
fs.read(/etc/**)              → resource: /etc/**
fs.read(read_file:*)          → functions: [read_file], resource: *
fs.read(read_file,glob:/x/*)  → functions: [read_file, glob], resource: /x/*
fs.read(/some,path:with-colon)→ resource: /some,path:with-colon  (no valid funclist prefix)
```

## Modules and categories

These categories go through the permission check. Anything not listed (string manipulation, data encoding, math, time, regexp, templates, etc.) is unchecked and always works.

| Module | Categories | What's checked |
|---|---|---|
| `fs` | `read`, `write`, `delete` | path access |
| `os` | `exec`, `env`, `process` | command execution, env reads/writes, chdir/exit |
| `http` | `client`, `server` | outgoing HTTP, listening servers |
| `ssh` | `connect`, `transfer` | remote exec, SCP up/down |
| `k8s` | `read`, `write`, `exec`, `config` | API access, kubectl-exec, kubeconfig load |
| `ai` | `generate` | LLM calls (model name as resource) |
| `mcp` | `client`, `server` | MCP connections + servers |
| `io` | `prompt` | interactive prompts |

## Loaded modules

Code reached through `load()` — a local directory module or an installed module — runs under the **same runtime permission as the entry script**. A module declares nothing about its own capabilities; downloading or importing it grants no authority.

A dependency that needs more than the entry script was granted fails at the gated call, and the run is restarted at a higher profile. For example, a script run with `--allow-fs` that loads a module which calls `k8s.read`:

```
kite ./deploy.star --allow-fs
#   deploy.star reads/writes local files   → allowed
#   the loaded module calls k8s.read(...)    → denied (k8s is allow-local)
# re-run granting the capability the dependency needs:
kite ./deploy.star --allow-local
```

A denial raised inside a loaded module names that module (see below).

## Composing with `--sandbox`

`--permissions` and `--sandbox` are independent, composable layers. Combined, they provide defense in depth:

```bash
kite ./untrusted.star --sandbox=strict --permissions=deny-all
```

`--permissions` blocks operations at the Starlark API level inside one process. `--sandbox` confines the OS view (filesystem, processes, network) at the kernel level via gVisor. A bypass in one is contained by the other. See [Sandbox](sandbox.md).

## Permission errors

A denied operation names the `module.category` and prescribes how to grant it:

```python
os.exec("uname -s")
# Error: permission denied: os.exec exec("/usr/bin/uname") - no matching allow rule
#   grant "os.exec" with --permissions=allow-all (or higher),
#   or an allow rule in ~/.starkite/config.yaml
```

The suggested profile is the lowest built-in tier that would actually grant the specific call: exec of a `$CWD` binary suggests `allow-local`, exec of a system binary suggests `allow-all`. A denial by an explicit `deny` rule names that rule instead.

When the call originates inside a `load()`'d module, the message attributes it to that module:

```
# Error: permission denied (module "deploy-helpers"): k8s.read read("pods") - no matching allow rule
```
