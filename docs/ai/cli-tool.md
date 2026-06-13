---
title: "CLI tool provider"
description: "Use kite as a secure, portable tool that agents in any language call over the command line"
weight: 30
---

# CLI tool provider

When you give an agent the ability to act, you take on the problem of bounding what it can do. `kite` solves that problem from the outside: any agent — written in Python, Go, TypeScript, or a shell script — can use it as a **tool** by invoking it over the command line, and because `kite` is a single static binary carrying its own permission engine and optional sandbox, the host agent gets a secure, portable, uniform way to run automation without embedding a language runtime or trusting arbitrary code.

The mechanism never varies. The host agent spawns `kite` as a subprocess, passes inputs as flags or stdin, and reads structured JSON back from stdout. That fixed shape is what lets the same wrapper drive any script, regardless of what the script does inside.

## Run a script as a tool

The simplest tool is a script the agent runs and reads the result of. You feed inputs in through [`--var`](../fundamentals/configuration.md) and ask for JSON on stdout with `--output json`:

```bash
kite run ./deploy.star --var image=myapp:v2 --output json
```

That one contract — inputs as flags, JSON on stdout — is all a host agent needs to wrap. From Python, you spawn the subprocess, capture stdout, and parse it:

```python
import json, subprocess

out = subprocess.run(
    ["kite", "run", "deploy.star", "--var", "image=myapp:v2", "--output", "json"],
    capture_output=True, text=True, check=True,
)
result = json.loads(out.stdout)
```

`result` is now an ordinary Python dictionary the agent can inspect or pass along; nothing about the call depended on the script's internals. The same shape holds in Go, where you run the command and read its output:

```go
out, err := exec.Command("kite", "run", "deploy.star", "--var", "image=myapp:v2").Output()
```

## Inline one-offs

A script file earns its keep when the logic is reusable. When the agent needs a single throwaway capability instead, writing a file is overhead, so `kite exec` runs inline source directly:

```bash
kite exec 'print(json.encode({"host": os.hostname()}))'
```

The host agent reads the printed JSON the same way it would from a script run — the only thing that changed is where the code came from.

## Make it safe

The reason to reach for `kite` at all is that the host agent may run logic you do not fully trust. That trust gap is exactly what the [permission engine](../fundamentals/security/permission.md) and, on Linux, the [sandbox](../fundamentals/security/sandbox.md) are for. Constrain a single invocation with a permission profile, an OS-level boundary, or both:

```bash
# Restrict to compute, print, and log only
kite run ./untrusted.star --permissions=deny-all

# OS-level isolation on Linux
kite run ./untrusted.star --sandbox
```

`--permissions=deny-all` leaves the script able to compute, print, and log but cuts off the filesystem, network, processes, and environment; `--sandbox` adds a process-level boundary underneath that. The payoff is that you can expose genuinely powerful automation — Kubernetes, SSH, HTTP, filesystem — while bounding the blast radius of any one invocation, and you pay that cost per call rather than trusting the calling agent wholesale.

## Why kite as a tool

The case for routing an agent's actions through `kite` comes down to three properties:

- **Portable** — one static binary, no toolchain or interpreter to install on the agent's host.
- **Uniform** — the same `kite run ./script.star --var k=v --output json` contract regardless of what the script does.
- **Secure** — per-invocation permission profiles and sandboxing, independent of the calling agent.

Calling over the command line is one of two ways to put `kite` behind an agent. To expose individual functions to an LLM over a protocol instead, see [Creating MCP servers](mcp.md). To embed the runtime directly in a Go program, see [Embedding](../references/embedding.md).
