---
title: "Editions"
description: "The four starkite binaries and how to switch between them"
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

All editions include the following 27 base modules:

`os`, `fs`, `fmt`, `io`, `vars`, `strings`, `regexp`, `json`, `yaml`, `csv`, `sql`, `time`, `base64`, `hash`, `uuid`, `template`, `gzip`, `zip`, `log`, `concur`, `retry`, `table`, `runtime`, `ssh`, `http`, `inventory`, `test`

For module details, see the [API Reference](../references/api/index.md).

## What `kitecloud` adds

The `kitecloud` edition adds the `k8s` module and the `kite kube` CLI command set. This enables Kubernetes CRUD operations, controller runtimes, custom resource definitions (CRDs), and admission webhooks.

For guides, see the [Kubernetes Guide](../infra/kubernetes.md).

## What `kiteai` adds

The `kiteai` edition adds LLM client integration and Model Context Protocol (MCP) support. This includes multi-provider chat and streaming APIs (`ai` module) and host-server MCP capabilities (`mcp` module).

For guides, see the [AI Agents Guide](../ai/agents.md).

## List and switch editions

Use the `kite edition` subcommand to manage and switch between installed editions on a host. To list installed editions and verify the active one, run:

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

To switch editions, use `kite edition use`. If the requested edition is not locally cached, the tool automatically fetches it from GitHub Releases before activation:

```bash
kite edition use cloud      # Download + activate kitecloud
kite edition use ai         # Download + activate kiteai
kite edition use base       # Switch back to base
```

For local development or offline environments, install custom-built binaries using `--from`:

```bash
kite edition use cloud --from ./bin/kitecloud
```

To uninstall an edition and reclaim disk space (the default base edition cannot be removed):

```bash
kite edition remove cloud
```

For syntax and flags, see the [`kite edition` CLI Reference](../references/cli/edition.md).
