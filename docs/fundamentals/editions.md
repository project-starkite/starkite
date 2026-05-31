---
title: "Editions"
description: "The four starkite binaries and how to switch between them"
weight: 10
---

# Editions

The default binary is **`kite`**, the all-in-one edition — it bundles every module and is what all examples and documentation use. Reach for a lean edition only when binary size or attack surface is a real constraint. Every edition speaks the same language and ships the same base modules; the lean ones simply omit privileged surface. This page covers all four, when a lean edition is worth it, and the `kite edition` subcommand for switching.

| Binary | Modules | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | **the default** — everything in one binary |
| `kitecmd` | base only | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

The lean editions (`kitecmd` / `kitecloud` / `kiteai`) are a strict subset of `kite`, packaged smaller for space-conscious targets — init containers, edge nodes, CI runners.

## Per-edition Go modules

Each edition is a distinct Go module with its own dependency graph. `kitecmd` links no Kubernetes or LLM client code (~26 MB binary, no transitive cloud-SDK dependencies). `kitecloud` and `kiteai` add only what their domain requires. `kite` composes all three.

Binary size and dependency surface are constraints in init containers, edge nodes, and CI runners — the lean editions exist for those targets.

## What's in "base"

Every edition includes the 27 base modules:

`os`, `fs`, `fmt`, `io`, `vars`, `strings`, `regexp`, `json`, `yaml`, `csv`, `path`, `time`, `base64`, `hash`, `uuid`, `template`, `gzip`, `zip`, `log`, `concur`, `retry`, `table`, `runtime`, `ssh`, `http`, `inventory`, `test`

See [References > API](../references/api/index.md) for the full catalog.

## What `kitecloud` adds

The `k8s` module exposes the full Kubernetes API as a three-tier surface (CRUD, kubectl-equivalent, type-safe constructors), plus `k8s.control()` for controller runtime, `k8s.webhook()` for admission webhooks, and `k8s.obj.crd()` for CRD scaffolding. The `kite kube` subcommand generates controller/webhook artifacts from scripts.

See [Kubernetes](../kubernetes/connect.md) for the working guides.

## What `kiteai` adds

The `genai` module wraps Firebase Genkit Go with multi-provider chat, streaming, tools, and structured output across Anthropic, OpenAI, and Gemini. The `mcp` module provides both an MCP server (`mcp.serve()`) and client (`mcp.connect()`) on top of the Model Context Protocol Go SDK.

See [AI](../ai/agents.md) for the working guides.

## List and switch editions

`kite edition` manages installed editions on a host. List the active and installed editions:

```bash
$ kite edition status
Current edition: ai
Version:         0.1.0-dev
Binary edition:  base

Installed editions:
  * ai    (56 MB)
    cloud (63 MB)
```

The `*` marker indicates the active edition.

Switch to a different edition — `use` downloads the matching binary from GitHub Releases on first switch:

```bash
kite edition use cloud      # download + activate kitecloud
kite edition use ai         # download + activate kiteai
kite edition use base       # switch back to base
```

Install from a local build instead of downloading:

```bash
kite edition use cloud --from ./bin/kitecloud
```

Remove an installed edition (the base edition cannot be removed):

```bash
kite edition remove cloud
```

See the [`kite edition` reference](../references/cli/edition.md) for the full subcommand surface.
