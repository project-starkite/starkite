---
title: "CLI tool provider"
description: "Use kite as a secure, portable tool that agents in any language call over the command line"
weight: 30
---

# CLI tool provider

An agent written in any language — Python, Go, TypeScript, a shell script — can use `kite` as a **tool** by invoking it over the command line. Because `kite` is a single static binary with a permission engine and an optional sandbox, it gives a host agent a secure, portable, uniform way to run automation without embedding a language runtime or trusting arbitrary code.

The shape is always the same: the host agent spawns `kite` as a subprocess, passes inputs as flags or stdin, and reads structured output (JSON) from stdout.

## Run a script as a tool

```bash
kite run deploy.star --var image=myapp:v2 --output json
```

The host agent supplies inputs through [`--var`](../fundamentals/configuration.md) and reads the script's JSON output from stdout. From Python:

```python
import json, subprocess

out = subprocess.run(
    ["kite", "run", "deploy.star", "--var", "image=myapp:v2", "--output", "json"],
    capture_output=True, text=True, check=True,
)
result = json.loads(out.stdout)
```

From Go:

```go
out, err := exec.Command("kite", "run", "deploy.star", "--var", "image=myapp:v2").Output()
```

## Inline one-offs

For a quick capability with no script file, `kite exec` runs inline source:

```bash
kite exec 'print(json.encode({"host": os.hostname()}))'
```

## Make it safe

Because the host agent may run untrusted logic, constrain what the tool can do with the [permission engine](../fundamentals/security/permission.md) and, on Linux, the [sandbox](../fundamentals/security/sandbox.md):

```bash
# Restrict to compute, print, and log only
kite run untrusted.star --permissions=deny-all

# OS-level isolation on Linux
kite run untrusted.star --sandbox
```

This lets a host agent expose powerful automation (Kubernetes, SSH, HTTP, filesystem) while bounding the blast radius per invocation.

## Why kite as a tool

- **Portable** — one static binary, no toolchain or interpreter to install on the agent's host.
- **Uniform** — the same `kite run script.star --var k=v --output json` contract regardless of what the script does.
- **Secure** — per-invocation permission profiles and sandboxing, independent of the calling agent.

To expose individual functions to an LLM over a protocol instead of the command line, see [Creating MCP servers](mcp.md). To embed the runtime directly in a Go program, see [Embedding](../references/embedding.md).
