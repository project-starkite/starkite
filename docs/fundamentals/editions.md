---
title: "Editions"
description: "The four binaries and what each one ships"
weight: 10
---

Starkite is distributed as four independent binaries. Every edition speaks the same language and ships the same base modules; the differences are what privileged surface they expose.

| Binary | Modules | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | you want everything in one binary — recommended for new users |
| `kitecmd` | base only | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

## Per-edition Go modules

Each edition is a distinct Go module with its own dependency graph. `kitecmd` links no Kubernetes or LLM client code (~26 MB binary, no transitive cloud-SDK dependencies). `kitecloud` and `kiteai` add only what their domain requires. `kite` composes all three.

This matters in constrained environments — init containers, edge nodes, CI runners — where binary size and dependency surface are liabilities.

## What's in "base"

Every edition includes the 27 base modules:

`os`, `fs`, `fmt`, `io`, `vars`, `strings`, `regexp`, `json`, `yaml`, `csv`, `path`, `time`, `base64`, `hash`, `uuid`, `template`, `gzip`, `zip`, `log`, `concur`, `retry`, `table`, `runtime`, `ssh`, `http`, `inventory`, `test`

See [References > API](../references/api/index.md) for the full catalog.

## What `kitecloud` adds

The `k8s` module exposes the full Kubernetes API as a three-tier surface (CRUD, kubectl-equivalent, type-safe constructors), plus `k8s.control()` for controller runtime, `k8s.webhook()` for admission webhooks, and `k8s.obj.crd()` for CRD scaffolding. The `kite kube` subcommand generates controller/webhook artifacts from your scripts.

See [Kubernetes](../kubernetes/index.md) for the working guides.

## What `kiteai` adds

The `genai` module wraps Firebase Genkit Go with multi-provider chat, streaming, tools, and structured output across Anthropic, OpenAI, and Gemini. The `mcp` module provides both an MCP server (`mcp.serve()`) and client (`mcp.connect()`) on top of the Model Context Protocol Go SDK.

See [AI](../ai/index.md) for the working guides.

## Switching editions

`kite edition use cloud` (or `ai`, or `cmd`) downloads and selects the right binary for the current host without you having to manage `$PATH` directly. See [`kite edition`](../references/cli/edition.md) for the full subcommand surface.
