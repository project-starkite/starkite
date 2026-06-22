---
title: "Starkite and Kubernetes"
description: "Configure cluster access and authenticate with the k8s module"
weight: 10
---

# Starkite and Kubernetes

Starkite's Infrastructure pillar is built natively around Kubernetes cluster operations. Rather than offering generic cloud provider modules (for AWS, GCP, or Azure) or duplicating Terraform-style infrastructure provisioning, Starkite integrates directly with the Kubernetes API to automate workloads, manage resources, and deploy custom controllers.

Before a script can list pods, apply a manifest, or roll a node, the `k8s` module needs to know which cluster to talk to. It resolves that the same way `kubectl` does: it reads `$KUBECONFIG` (or `~/.kube/config`) and uses the current context. So if `kubectl` already works on your machine, the module works too — you write cluster operations directly, with no connection step to set up first.

That means the simplest script names a resource and goes:

```python
# Uses the current kubeconfig context
nodes = k8s.list("nodes")
print(len(nodes), "nodes")
```

The call lands on whatever context `kubectl config current-context` would report, lists the nodes there, and returns them as a list you can count and iterate.

## Selecting a context and namespace

The current context is the right default for an interactive run, but automation often has to target a specific cluster or namespace regardless of what your local config happens to point at. When you need that control, `k8s.config()` returns a client bound to the context, namespace, or kubeconfig path you name, and you drive the cluster through that client instead of the module's top-level functions:

```python
client = k8s.config(context="prod-cluster", namespace="payments")
pods = client.list("pods")
```

Here the client is pinned to the `prod-cluster` context with `payments` as its default namespace, so `client.list("pods")` reads pods from `payments` on that cluster no matter what the ambient kubeconfig selects.

You do not always need a separate client to cross a namespace boundary. Most module functions accept a `namespace` kwarg directly, and it overrides the default for that one call:

```python
k8s.list("pods", namespace="kube-system")
```

When you omit the kwarg, the call falls back to the client's default namespace — the one fixed by `k8s.config()`, or the kubeconfig's default when you let the module use the current context.

## Inspecting connection details

When a run targets the wrong cluster or namespace, the cheapest fix is to confirm where the client actually points before debugging anything else. Three helpers report exactly that:

```python
print(k8s.context())          # current context name
print(k8s.namespace_name())   # default namespace
print(k8s.version())          # server version
```

`k8s.context()` and `k8s.namespace_name()` echo back the resolved context and default namespace, and `k8s.version()` reaches the cluster for its server version — so a successful call also doubles as proof that the connection works end to end.

## Permissions

Reaching a cluster costs authority. The `k8s` module's `read`, `write`, and `config` access sits in the `allow-local` profile, so a script that lists nodes or loads a kubeconfig with `k8s.config()` needs at least that profile; under the default `deny-all` the call is denied. Grant it at the command line:

```bash
kite ./cluster.star --permissions=allow-local
```

The more dangerous operations cost more: `k8s.exec`, which runs a command inside a pod, is gated separately at `allow-all`. See [Permission](../fundamentals/security/permission.md) for the full ladder.

## Edition note

The `k8s` module is in the default `kite` binary and the lean `kitecloud` edition — not in `kitecmd` or `kiteai`. See [Editions](../fundamentals/editions.md). For the full surface, see the [k8s API reference](../references/api/k8s.md).
