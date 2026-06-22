---
title: "Examples"
description: "Runnable starkite example scripts, grouped by domain"
weight: 1
---

# Examples

The fastest way to learn what a Starkite script looks like is to read one that already works. This page catalogs runnable `.star` scripts from the [starkite repository](https://github.com/project-starkite/starkite/tree/main/examples), grouped by the domain they exercise — core automation, Kubernetes, AI agents, and the sandbox. Each entry links to source you can clone and run as-is with `kite run <path>`, then adapt to your own task. Start with the card that matches what you are trying to do.

<div class="grid cards" markdown>

-   :material-cog:{ .lg .middle } __Core modules__

    ---

    System info, SSH health checks, HTTP servers and clients — the base automation modules.

    [:octicons-arrow-right-24: Browse](#core-modules)

-   :material-kubernetes:{ .lg .middle } __Kubernetes__

    ---

    Deployments, rolling updates, controllers, webhooks, and full app stacks.

    [:octicons-arrow-right-24: Browse](#kubernetes)

-   :material-robot:{ .lg .middle } __AI__

    ---

    Agent loops and MCP integration with the `ai` and `mcp` modules.

    [:octicons-arrow-right-24: Browse](#ai)

-   :material-shield-lock:{ .lg .middle } __Sandbox__

    ---

    OS-level isolation with `--sandbox` (Linux).

    [:octicons-arrow-right-24: Browse](#sandbox)

</div>

## Core modules

Begin where every script begins: the base modules for talking to the local system, a remote host over SSH, and the network. These examples range from a one-line hello world to a full HTTP server with middleware, so you can pick the one closest to your task and build outward.

| Example | Description |
|---------|-------------|
| [hello.star](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star) | Hello world |
| [sysinfo.star](https://github.com/project-starkite/starkite/blob/main/examples/core/sysinfo.star) | System information gathering |
| [remote-check.star](https://github.com/project-starkite/starkite/blob/main/examples/core/remote-check.star) | Remote server health checks via SSH |
| [http-server/](https://github.com/project-starkite/starkite/tree/main/examples/core/http-server) | REST APIs, webhooks, middleware |

When you want the concepts behind these rather than the finished scripts, read the [Core Modules guides](../core-modules/system.md).

## Kubernetes

Once you are comfortable with the base modules, the `k8s` module turns those same patterns toward a cluster. The examples below run the full range of cluster work — a single deployment, a zero-downtime rolling update, a reconcile loop, an admission webhook, a complete multi-tier stack — so you can match one to the shape of your own operation.

| Example | Description |
|---------|-------------|
| [deploy-k8s](https://github.com/project-starkite/starkite/tree/main/examples/cloud/deploy-k8s) | Basic Kubernetes deployment |
| [quick-deploy](https://github.com/project-starkite/starkite/tree/main/examples/cloud/quick-deploy) | One-line deployments |
| [rolling-update](https://github.com/project-starkite/starkite/tree/main/examples/cloud/rolling-update) | Zero-downtime rolling updates |
| [app-stack](https://github.com/project-starkite/starkite/tree/main/examples/cloud/app-stack) | Full application stack |
| [namespace-stack](https://github.com/project-starkite/starkite/tree/main/examples/cloud/namespace-stack) | Namespace provisioning |
| [multi-env](https://github.com/project-starkite/starkite/tree/main/examples/cloud/multi-env) | Multi-environment deployments |
| [microservices](https://github.com/project-starkite/starkite/tree/main/examples/cloud/microservices) | Microservices deployment |
| [redis-cluster](https://github.com/project-starkite/starkite/tree/main/examples/cloud/redis-cluster) | Redis cluster with Helm |
| [wordpress-stack](https://github.com/project-starkite/starkite/tree/main/examples/cloud/wordpress-stack) | WordPress + MySQL stack |
| [cronjobs](https://github.com/project-starkite/starkite/tree/main/examples/cloud/cronjobs) | Kubernetes CronJobs |
| [cluster-health](https://github.com/project-starkite/starkite/tree/main/examples/cloud/cluster-health) | Cluster health monitoring |
| [debug-pod](https://github.com/project-starkite/starkite/tree/main/examples/cloud/debug-pod) | Debug pod for troubleshooting |
| [controller/](https://github.com/project-starkite/starkite/tree/main/examples/cloud/controller) | Controller reconcile loops |
| [webhook/](https://github.com/project-starkite/starkite/tree/main/examples/cloud/webhook) | Validating and mutating admission webhooks |

For the concepts these scripts rest on — connecting to a cluster, applying manifests, watching resources — see the [Kubernetes guides](../kubernetes/connect.md).

## AI

The `ai` and `mcp` modules let a script drive a model the same way it drives a cluster. These examples live under [`aikite/examples/agent/`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent) and walk the common agent shapes — a run-to-completion loop, a human-in-the-loop REPL, history management for long sessions, and wrapping an external tool server over MCP.

| Example | Description |
|---------|-------------|
| [autonomous_fix.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/autonomous_fix.star) | Autonomous run-to-completion agent |
| [interactive_assistant.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/interactive_assistant.star) | User-in-the-loop REPL agent |
| [history_management.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/history_management.star) | History summarization for long runs |
| [mcp_integration.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/mcp_integration.star) | Wrapping an MCP server's tools |

To understand the agent and MCP patterns these scripts assemble, read the [AI Support guides](../ai/agents.md).

## Sandbox

When you run code you do not fully trust, the examples above gain a second layer: `--sandbox` confines the script to OS-level isolation on Linux. These three show that layer in action — a network fetch under the default profile, offline compute over the working directory, and the sandbox composed with a permission profile for defense in depth.

| Example | Description |
|---------|-------------|
| [netaccess-http-fetch.star](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/netaccess-http-fetch.star) | HTTPS fetch under the default profile |
| [opaque-compute.star](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/opaque-compute.star) | Offline compute over `$CWD` |
| [defense-in-depth.star](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/defense-in-depth.star) | Compose `--sandbox=opaque` with `--permissions=allow-fs` |

The sandbox is Linux-only. For the isolation model and the available profiles, see the [Sandbox guide](../fundamentals/security/sandbox.md).
