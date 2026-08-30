---
title: "Infrastructure Automation"
description: "Overview of Starkite's cloud-native and fleet-wide SRE automation capabilities"
weight: 1
---

# Infrastructure Automation

Starkite is built for local systems automation, but it also includes dedicated modules for orchestrating larger, cloud-native and node-level infrastructure. To automate these environments, the runtime integrates directly with the Kubernetes API for container orchestration and provides native remote host management over SSH without requiring target-side daemon installation. This allows you to scale local automation scripts across Kubernetes clusters and multi-node fleets using Starkite's sandboxed Starlark environment.

By combining a sandboxed Starlark runtime with native, high-level modules, Starkite allows SREs, platform engineers, and SRE AI agents to run secure, isolated automation scripts locally, inside pipelines, or as background controllers in a cluster.

---

## Cloud-Native Automation with Kubernetes

Starkite integrates directly with the Kubernetes API to automate containerized workloads, manage resources, and deploy custom controllers. By utilizing native Starlark bindings, you can interact with any Kubernetes cluster without installing heavy external CLI tools or writing complex client code.

### Key Features
* **Automatic Authentication**: Resolves cluster access natively by reading local kubeconfig files (`$KUBECONFIG` or `~/.kube/config`) and current contexts, matching the ambient behavior of `kubectl`.
* **Declarative Resource Management**: Creates, retrieves, updates, and deletes Kubernetes resources using structured Starlark dictionaries or raw YAML/JSON manifests.
* **Continuous Control Loops**: Runs background reconciliation loops using `k8s.control()` to monitor cluster state, detect configuration drift, and automatically apply corrections.
* **Dynamic Admission Webhooks**: Intercepts, validates, or mutates incoming Kubernetes API requests before they are persisted, allowing you to enforce custom policies in the cluster.
* **Targeted Namespace Operations**: Targets specific namespaces globally through client configuration or overrides them on a per-call basis using function kwargs.

---

## Host Fleet Automation with SSH

Starkite provides concurrent remote host management by pairing the `ssh` module with the `inventory` module, requiring no target-side software installation. Rather than maintaining proprietary control daemons on remote nodes, the runtime uses native SSH to manage physical servers, virtual machines, and remote instances.

### How Fleet Automation Works
1. **Unified Client Configuration**: You define an SSH client once via `ssh.config()`, specifying target hosts, private keys for authentication, connection timeouts, and optional jump hosts.
2. **Connection Session Reuse**: The runtime establishes connection handshakes once when building the client and reuses these active sessions for all subsequent operations, eliminating repeated handshake overhead.
3. **Concurrent Fan-Out**: When you execute a command (e.g., `client.exec("uptime")`), Starkite dispatches the command across all target hosts concurrently, leveraging parallel workers to minimize latency.
4. **Graceful Error Aggregation**: Instead of aborting the entire run on the first connection or command failure, the runtime aggregates results into a list of `SSHResult` objects. Each result contains the host name, exit code, execution status (`.ok`), `.stdout`, and `.stderr`, allowing scripts to handle partial failures or offline hosts gracefully.
5. **Dynamic Fleet Inventory**: Using the `inventory` module, you can load host lists dynamically from external files (YAML or JSON) and filter them by group, region, or metadata tag before piping them into the SSH client.

### Key Features
* **Daemonless Execution**: Manages remote systems, restarts services, and runs scripts over standard SSH without installing target-side control software.
* **File Orchestration**: Uploads configuration files, packages, or application code and downloads logs or diagnostics across the fleet concurrently.
* **Sudo Privilege Escalation**: Runs administrative commands securely by prompting for or reading sudo credentials at runtime.
* **Structured Fleet Inventories**: Organizes nodes into groups (e.g., `web`, `db`, `staging`) and filters them dynamically to target specific subsets of infrastructure.

---

## The Security Model

Because infrastructure operations run with elevated privileges, Starkite enforces a zero-trust execution model to prevent unauthorized resource access or credential exposure.

* **OS-Level Sandboxing**: On supported environments, Starkite executes scripts within an isolated OS sandbox runtime (using native Linux Landlock, macOS Seatbelt, or container runtimes). This isolates host system access, prevents unintended writes, and secures credentials.

---

## Next Steps

To begin automating your infrastructure, explore these guides:

* [Connecting to a cluster](k8s-connect.md) — Configure cluster access, manage kubeconfig contexts, and resolve namespaces.
* [Object representation](k8s-objects.md) — Understand how Starkite represents Kubernetes resources and handles access semantics.
* [Managing workloads](k8s-workloads.md) — Deploy, scale, autoscale, and monitor container workloads.
* [Querying resources](k8s-resources.md) — Retrieve, list, filter, and watch Kubernetes API objects.
* [Declarative manifests](k8s-manifests.md) — Construct, apply, and generate Kubernetes YAML configs.
* [Kubernetes Controllers](k8s-controllers.md) — Build custom reconciliation loops and background controllers.
* [Admission webhooks](k8s-webhooks.md) — Intercept, validate, and mutate API requests before they are persisted.
* [Connecting with SSH](ssh-connect.md) — Configure SSH clients, manage private key authentication, and establish connections.
* [Executing host commands](ssh-exec.md) — Run remote commands, handle execution results, and manage environment contexts.
* [Transferring files](ssh-transfer.md) — Upload and download files securely across remote hosts.
* [Fleet orchestration](fleet.md) — Discover, filter, and automate changes across entire server fleets.
