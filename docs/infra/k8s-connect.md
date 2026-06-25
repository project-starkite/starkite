---
title: "Connecting to a cluster"
description: "Configure cluster access, manage kubeconfig contexts, and resolve namespaces"
weight: 10
---

# Connecting to a cluster

The `k8s` module connects to Kubernetes clusters using your local kubeconfig. It requires no manual connection setup or bootstrapping.

## Default Connection

By default, the `k8s` module reads your `$KUBECONFIG` environment variable (falling back to `~/.kube/config`) and uses the current active context and namespace. If `kubectl` is authenticated and working on your machine, Starkite works automatically:

```python
def main():
    # Uses the current kubeconfig context automatically
    nodes = k8s.list("nodes")
    print(len(nodes), "nodes found in cluster")
```

## Configuring Context and Namespace

If you need to target a specific cluster, namespace, or custom kubeconfig path, configure a client using `k8s.config()`. This returns a dedicated client that pins all operations to those settings:

```python
def main():
    # Bind a client to a specific context and namespace
    client = k8s.config(
        context = "prod-cluster",
        namespace = "payments",
    )
    
    # All operations on this client are isolated to the 'payments' namespace
    pods = client.list("pods")
    print("Pods in payments:", len(pods))
```

You can also override the namespace on a per-call basis using the `namespace` keyword argument:

```python
def check_system():
    # Overrides the default namespace for this call only
    system_pods = k8s.list("pods", namespace="kube-system")
    print("System pods:", len(system_pods))
```

## Verifying Connection Details

To inspect and verify the active connection context before running operations, use the connection helper functions:

```python
def print_connection_info():
    print("Active Context:", k8s.context())
    print("Default Namespace:", k8s.namespace_name())
    print("Kubernetes Version:", k8s.version())
```

## Permissions

Accessing the Kubernetes API requires permissions. You must run the script with at least the `allow-local` profile:

```bash
kite run ./cluster_check.star --permissions=allow-local
```
