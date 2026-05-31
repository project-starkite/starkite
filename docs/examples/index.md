---
title: "Examples"
description: "Runnable starkite example scripts, grouped by domain"
weight: 1
---

# Examples

Runnable `.star` scripts in the [starkite repository](https://github.com/project-starkite/starkite/tree/main/examples), grouped by domain. Each links to source you can run with `kite run <path>`.

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

| Example | Description |
|---------|-------------|
| [hello.star](https://github.com/project-starkite/starkite/blob/main/examples/core/hello.star) | Hello world |
| [sysinfo.star](https://github.com/project-starkite/starkite/blob/main/examples/core/sysinfo.star) | System information gathering |
| [remote-check.star](https://github.com/project-starkite/starkite/blob/main/examples/core/remote-check.star) | Remote server health checks via SSH |
| [http-server/](https://github.com/project-starkite/starkite/tree/main/examples/core/http-server) | REST APIs, webhooks, middleware |

See the [Core Modules guides](../core-modules/system.md) for the concepts behind these.

## Kubernetes

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

See the [Kubernetes guides](../kubernetes/connect.md) for the concepts behind these.

## AI

Agent and MCP patterns: [`aikite/examples/agent/`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent).

| Example | Description |
|---------|-------------|
| [autonomous_fix.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/autonomous_fix.star) | Autonomous run-to-completion agent |
| [interactive_assistant.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/interactive_assistant.star) | User-in-the-loop REPL agent |
| [history_management.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/history_management.star) | History summarization for long runs |
| [mcp_integration.star](https://github.com/project-starkite/starkite/blob/main/aikite/examples/agent/mcp_integration.star) | Wrapping an MCP server's tools |

See the [AI Support guides](../ai/agents.md) for the patterns behind these.

## Sandbox

OS-level isolation with `--sandbox` (Linux only). See the [Sandbox guide](../fundamentals/security/sandbox.md).

| Example | Description |
|---------|-------------|
| [default-http-fetch.star](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/default-http-fetch.star) | HTTPS fetch under the default profile |
| [strict-compute.star](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/strict-compute.star) | Offline compute over `$CWD` |
| [defense-in-depth.star](https://github.com/project-starkite/starkite/blob/main/examples/sandbox/defense-in-depth.star) | Compose `--sandbox=strict` with `--permissions=strict` |
