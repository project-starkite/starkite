---
title: "Connect to cluster"
description: "Configure cluster access for the k8s module"
weight: 10
---

# Connect to cluster

The `k8s` module talks to a cluster using the same kubeconfig resolution as `kubectl`. By default it reads `$KUBECONFIG` (or `~/.kube/config`) and uses the current context — so a script that already works with `kubectl` needs no extra setup.

```python
# Uses the current kubeconfig context
nodes = k8s.list("nodes")
print(len(nodes), "nodes")
```

## Selecting a context and namespace

`k8s.config()` builds a client bound to a specific context, namespace, or kubeconfig path:

```python
client = k8s.config(context="prod-cluster", namespace="payments")
pods = client.list("pods")
```

Most module functions also accept a `namespace` kwarg directly; when omitted, the client's default namespace is used:

```python
k8s.list("pods", namespace="kube-system")
```

## Inspecting connection details

```python
print(k8s.context())          # current context name
print(k8s.namespace_name())   # default namespace
print(k8s.version())          # server version
```

## Edition note

The `k8s` module is in the default `kite` binary and the lean `kitecloud` edition — not in `kitecmd` or `kiteai`. See [Editions](../fundamentals/editions.md). For the full surface, see the [k8s API reference](../references/api/k8s.md).
