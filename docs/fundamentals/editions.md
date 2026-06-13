---
title: "Editions"
description: "The four starkite binaries and how to switch between them"
weight: 10
---

# Editions

An edition is a build of Starkite that carries a particular set of modules. The point of having more than one is to let you trade reach for size: a single all-in-one binary can do everything, but on a space-constrained target you may not want to pay for modules you never call. Starkite ships four editions so you can pick the smallest binary that still covers your workload, without learning a different tool for each one — every edition speaks the same language and ships the same base modules. This page covers all four, when a lean edition earns its keep, and the `kite edition` subcommand for switching between them.

The default is **`kite`**, the all-in-one edition. It bundles every module, and it is what all examples and documentation assume. Reach for a lean edition only when binary size or attack surface is a real constraint; otherwise `kite` is the binary you want.

| Binary | Modules | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | **the default** — everything in one binary |
| `kitecmd` | base only | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

The lean editions — `kitecmd`, `kitecloud`, and `kiteai` — are each a strict subset of `kite`, packaged smaller for space-conscious targets such as init containers, edge nodes, and CI runners. A script that runs under `kite` runs unchanged under any lean edition that carries the modules it loads; the lean ones simply omit privileged surface you are not using.

## Per-edition Go modules

That size difference is not cosmetic — each edition is a distinct Go module with its own dependency graph, so what an edition leaves out, it never compiles in. `kitecmd` links no Kubernetes or LLM client code, which keeps it around 26 MB with no transitive cloud-SDK dependencies. `kitecloud` and `kiteai` add only what their domain requires, and `kite` composes all three.

The payoff is most concrete where binary size and dependency surface are themselves the constraint — init containers, edge nodes, and CI runners. The lean editions exist for those targets; on a developer workstation the all-in-one `kite` is the simpler choice.

## What's in "base"

Whichever edition you run, you start from the same floor. Every edition includes the 27 base modules:

`os`, `fs`, `fmt`, `io`, `vars`, `strings`, `regexp`, `json`, `yaml`, `csv`, `path`, `time`, `base64`, `hash`, `uuid`, `template`, `gzip`, `zip`, `log`, `concur`, `retry`, `table`, `runtime`, `ssh`, `http`, `inventory`, `test`

See [References > API](../references/api/index.md) for the full catalog. Everything above this floor is what each non-base edition adds.

## What `kitecloud` adds

On top of base, `kitecloud` brings the Kubernetes surface. The `k8s` module exposes the full Kubernetes API as a three-tier surface (CRUD, kubectl-equivalent, type-safe constructors), plus `k8s.control()` for controller runtime, `k8s.webhook()` for admission webhooks, and `k8s.obj.crd()` for CRD scaffolding. The `kite kube` subcommand generates controller and webhook artifacts from scripts.

See [Kubernetes](../kubernetes/connect.md) for the working guides.

## What `kiteai` adds

Where `kitecloud` reaches into clusters, `kiteai` reaches into models. The `genai` module wraps Firebase Genkit Go with multi-provider chat, streaming, tools, and structured output across Anthropic, OpenAI, and Gemini. The `mcp` module provides both an MCP server (`mcp.serve()`) and a client (`mcp.connect()`) on top of the Model Context Protocol Go SDK.

See [AI](../ai/agents.md) for the working guides.

## List and switch editions

You do not have to choose one edition and reinstall to change your mind. The `kite edition` subcommand manages the editions installed on a host. Start by asking what is active and what is already installed:

```bash
$ kite edition status
Current edition: ai
Version:         0.1.0-dev
Binary edition:  base

Installed editions:
  * ai    (56 MB)
    cloud (63 MB)
```

The `*` marker points at the active edition — here, `ai`.

To change which edition is active, use `use`. The first time you switch to an edition that is not yet installed, `use` downloads the matching binary from GitHub Releases before activating it:

```bash
kite edition use cloud      # download + activate kitecloud
kite edition use ai         # download + activate kiteai
kite edition use base       # switch back to base
```

When you are developing an edition or working offline, point `use` at a binary you built yourself with `--from`, and it installs that instead of downloading:

```bash
kite edition use cloud --from ./bin/kitecloud
```

When you no longer need an installed edition, remove it to reclaim the space. The base edition is the fallback and cannot be removed:

```bash
kite edition remove cloud
```

See the [`kite edition` reference](../references/cli/edition.md) for the full subcommand surface.
