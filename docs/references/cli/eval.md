---
title: "kite eval"
description: "Evaluate inline Starlark code"
weight: 3
---

Evaluate inline Starlark code directly from the command line.

`kite eval` compiles and executes a Starlark code string passed as a command-line argument. It is designed for one-liners, expression evaluation, quick snippets, and lightweight data transformations without authoring a script file on disk.

## Usage

```bash
kite eval '<code>' [flags]
```

## Security Boundary

Inline code evaluated by `kite eval` operates under the dual-layer security model:
1. **Starlark Language Permissions**: Capability gating restricting module calls and function access.
2. **OS Kernel Sandbox**: Kernel-level isolation (Linux Landlock/Seccomp, macOS Seatbelt, Windows AppContainers) restricting filesystem, network, and subprocess access.

By default, evaluation runs under zero-trust (`deny-all`): pure computation is enabled, while filesystem, network, and subprocess access require explicit permission flags.

## Variable Injection

Inject values into the Starlark runtime using `--var` or `--var-file`:

```bash
kite eval 'print("Deploying to: " + var_str("ENV"))' --var ENV=production
```

## Examples

### Expression and Arithmetic Evaluation

```bash
kite eval 'print(2 ** 10)'
```

### JSON and YAML Manipulation

```bash
kite eval '
data = {"status": "ok", "tags": ["prod", "us-east"]}
print(json.encode(data))
'
```

### Host Information

```bash
kite eval 'print(runtime.platform() + "/" + runtime.arch())'
```

### Sandboxed Network Request

```bash
kite eval 'print(http.get("https://api.github.com/zen").body)' --sandbox-net-allow
```

### Working Directory Read Access

```bash
kite eval 'print(fs.read_text("./config.yaml"))' --sandbox-path-read=.
```

## Flags

`kite eval` supports standard execution flags:

| Flag | Description | Default |
|------|-------------|---------|
| `--var key=value` | Inject runtime variable | |
| `--var-file path` | Load runtime variables from YAML file | |
| `--output format` | Output formatting (`text`, `json`, `yaml`) | `text` |
| `--timeout seconds` | Execution timeout in seconds | `300` |
| `--dry-run` | Preview actions without executing side-effects | `false` |
| `--sandbox-profile name` | Active sandbox profile (`deny-all`, `sandbox-path`, etc.) | `""` |
| `--sandbox-path-read path` | Grant read access to path | |
| `--sandbox-path-write path` | Grant read and write access to path | |
| `--sandbox-net-allow` | Allow outbound network egress | |
| `--sandbox-enabled=false` | Disable sandbox confinement | `true` |
