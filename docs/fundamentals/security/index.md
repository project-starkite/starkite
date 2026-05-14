---
title: "Security"
description: "Permissions and sandbox: the two-layer model"
weight: 20
---

# Security

Starkite secures scripts in two distinct, composable layers. Use either independently, or use both together for defense in depth.

## Layer 1 — Permissions

The **permission engine** intercepts every privileged module call (filesystem write, network connect, command exec, Kubernetes apply, …) and matches it against a rule set. Rules are written in a compact `module.category(funcs?:resource?)` grammar; the engine ships with three built-in profiles (`allow-all`, `strict`, `deny-all`) and supports user-defined profiles from `~/.starkite/security.yaml`, file paths, inline rules, and script frontmatter.

Permissions are a pure-Go check inside the same process — there's no kernel call, no namespace, no measurable overhead. Available on every platform that runs starkite.

→ [Permissions](permissions.md)

## Layer 2 — Sandbox

The **sandbox** runs the entire script process inside a [gVisor](https://gvisor.dev) user-space kernel. The script sees only the directories you explicitly mount, network access can be cut to loopback-only, and even a complete compromise of the runtime can't reach the host. Two built-in profiles: `default` (host network, `$CWD` writable, no `$HOME`) and `strict` (loopback-only, `$CWD` + `/tmp` only, no outbound).

Sandbox is Linux-only and depends on unprivileged user namespaces. Composes cleanly with the permission engine.

→ [Sandbox](sandbox.md)

## When to use which

| Goal | Use |
|---|---|
| Stop a trusted script from doing one specific thing it shouldn't | Permissions |
| Run a third-party script you haven't audited | Sandbox + Permissions |
| Run untrusted scripts on a shared host (Linux) | Sandbox + Permissions |
| Bound a CI job's blast radius | Sandbox + Permissions |
| Quickly limit a script to `$CWD`-only filesystem | `--permissions=strict` |
| Cut all network access and host filesystem visibility | `--sandbox=strict` |

The two layers answer different questions: permissions answers *"which operations is this script allowed to invoke?"*; sandbox answers *"what slice of the host can the process see at all?"*. Pairing both is the recommended posture for anything untrusted.
