---
title: "Editions"
description: "The four starkite binaries"
weight: 10
---

# Editions

A Starkite edition is a packaged binary containing a specific set of runtime modules. Providing multiple editions allows you to minimize binary size and reduce dependency graphs in space-constrained or security-sensitive environments. All editions share a common language core and base modules.

By default, the all-in-one **`kite`** binary is used. Reach for a lean edition only if binary footprint or dependency auditing constraints require it.

| Binary | Included Modules | Use Case |
|---|---|---|
| `kite` | Base + Kubernetes + AI/MCP | **Default all-in-one** developer workstation binary |
| `kitecmd` | Base only | Minimal system scripting, general automation, and CI tasks |
| `kitecloud` | Base + Kubernetes (`k8s` module + `kite kube` CLI commands) | Cloud-native ops and manifest workflows |
| `kiteai` | Base + LLM clients + MCP capabilities | Agentic AI orchestration and LLM tool integration |

Lean editions (`kitecmd`, `kitecloud`, and `kiteai`) are strict subsets of `kite`, packaged smaller for space-conscious environments like init containers, edge nodes, and CI runners.

## Per-edition Go modules

To optimize binary footprints, each edition is defined as a distinct Go module with independent dependency graphs. This ensures omitted modules are never compiled in. For example, `kitecmd` links no Kubernetes or LLM client libraries, producing a ~26 MB binary with no cloud-native library overhead. Lean editions are tailored for container init steps, edge computing, or CI runners; developers typically use the all-in-one `kite` binary.

## Base modules

All editions include the following 28 base modules:

`os`, `fs`, `fmt`, `io`, `vars`, `strings`, `regexp`, `json`, `yaml`, `csv`, `sql`, `time`, `base64`, `hash`, `uuid`, `template`, `gzip`, `zip`, `log`, `concur`, `retry`, `table`, `runtime`, `ssh`, `http`, `fleet`, `inventory`, `test`

For module details, see the [API Reference](../references/api/index.md).

## What `kitecloud` adds

The `kitecloud` edition adds the `k8s` module and the `kite kube` CLI command set. This enables Kubernetes CRUD operations, controller runtimes, custom resource definitions (CRDs), and admission webhooks.

For guides, see the [Kubernetes Guide](../infra/k8s-connect.md).

## What `kiteai` adds

The `kiteai` edition adds LLM client integration and Model Context Protocol (MCP) support. This includes multi-provider chat and streaming APIs (`ai` module) and host-server MCP capabilities (`mcp` module).

For guides, see the [AI Agents Guide](../ai/agents.md).
