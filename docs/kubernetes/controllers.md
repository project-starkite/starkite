---
title: "Controllers"
description: "Build a Kubernetes controller with k8s.control() — reconcile loops, label filters, periodic resync"
weight: 60
---

# Controllers

A starkite controller is a `.star` script that registers handlers with `k8s.control()`. The function blocks the script (like `http.serve()`) and runs a watch-driven reconcile loop: events arrive from the Kubernetes API server, deduplicate per resource, and dispatch to the handler. Watch reconnection, exponential backoff, rate-limited retries, and SIGTERM/SIGINT shutdown are handled by the runtime.

The example below enforces a maximum replica count on Deployments labeled `enforce-max-replicas=true`. When a labeled Deployment exceeds the configured maximum, the controller scales it back down. Demonstrates the `reconcile` handler, label-selector filtering, and periodic resync.

**Source:** [`examples/cloud/controller/deployment-scaler.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/controller/deployment-scaler.star)

## Script

```python
#!/usr/bin/env kite
# deployment-scaler.star — enforces max replicas on labeled deployments

max_replicas = var_int("max_replicas", 3)

def reconcile(event, obj):
    if event == "DELETED":
        printf("[DELETED] %s/%s\n", obj.metadata.namespace, obj.metadata.name)
        return

    replicas = obj.spec.replicas
    if replicas == None:
        replicas = 1

    if replicas > max_replicas:
        printf("[SCALE DOWN] %s/%s from %d to %d replicas\n",
            obj.metadata.namespace, obj.metadata.name, replicas, max_replicas)
        k8s.apply({
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": obj.metadata.name,
                "namespace": obj.metadata.namespace,
            },
            "spec": {"replicas": max_replicas},
        })
    else:
        printf("[OK] %s/%s has %d replicas (max: %d)\n",
            obj.metadata.namespace, obj.metadata.name, replicas, max_replicas)

printf("Enforcing max %d replicas on deployments with label enforce-max-replicas=true...\n", max_replicas)
k8s.control("deployments",
    reconcile = reconcile,
    labels = "enforce-max-replicas=true",
    resync = "1m",
)
```

## Run it

In one shell, start the controller:

```bash
kite run ./examples/cloud/controller/deployment-scaler.star
kite run ./examples/cloud/controller/deployment-scaler.star --var max_replicas=5
```

In another shell, create a labeled Deployment that exceeds the limit:

```bash
kubectl create deployment nginx --image=nginx --replicas=10
kubectl label deployment nginx enforce-max-replicas=true
# the controller scales it down to max_replicas
```

Stop the controller with Ctrl-C. The `k8s.control()` call is blocking and registers a SIGTERM/SIGINT handler that cleans up the watch.

## Handler shape

`k8s.control()` accepts any of four handler kwargs. Use the one (or combination) that matches the workload:

| Handler | Signature | Called on |
|---|---|---|
| `on_create` | `fn(obj)` | `ADDED` events |
| `on_update` | `fn(old, new)` | `MODIFIED` events |
| `on_delete` | `fn(obj)` | `DELETED` events |
| `reconcile` | `fn(event, obj)` | catch-all (any event type as a string) |

Objects are passed as `AttrDict` — dot-access for reads (`obj.metadata.namespace`), bracket-access for writes (`obj["spec"]["replicas"] = 3`). List elements are also AttrDicts.

## What's happening

- **`k8s.control(kind, ...)`** blocks the script and runs a reconcile loop. The first positional arg is the resource kind; everything else is a kwarg.
- **`labels = "k=v"`** filters the watch to resources matching the label selector. Without it, every resource of that kind is watched.
- **`resync = "1m"`** re-fires `reconcile` on every known resource every minute, even without an API event — the safety net for missed events.
- **`workers = N`** caps concurrent handler invocations (default 1, per-resource serialization).
- **`namespace = "ns"`** restricts the watch to a single namespace; omit for cluster-wide.
- **`k8s.apply(manifest)`** inside the handler performs a server-side apply. Pair with owner references for cascading delete behavior.

## Generating deployment artifacts

For shipping a controller to a cluster, `kite kube gen-controller-artifacts` generates the supporting manifests (Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, Deployment) plus an optional Dockerfile:

```bash
kite kube gen-controller-artifacts \
    --controller examples/cloud/controller/deployment-scaler.star \
    --image myregistry/myapp-controller:v1 \
    --namespace myapp-system > deploy.yaml

kubectl apply -f deploy.yaml
```

Add `--resource <path>` to also generate a CRD when the controller manages a custom resource (see [`examples/cloud/controller/resource.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/controller/resource.star) for the CRD definition pattern).

## See also

- [`k8s` reference](../references/api/k8s.md) — `control`, `obj.crd`, owned-resource watches, leader election
- [`kite kube` reference](../references/cli/kube.md) — `gen-controller-artifacts` flags
- [Webhooks](webhooks.md) — admission webhooks built on the same blocking-server pattern
- More controller examples: [`examples/cloud/controller/`](https://github.com/project-starkite/starkite/tree/main/examples/cloud/controller) — `configmap-sync.star` (event logging), `leader-election.star` (HA), `deploy-controller.star` (full programmatic install)
