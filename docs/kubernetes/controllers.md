---
title: "Controllers"
description: "Build a Kubernetes controller with k8s.control() — reconcile loops, label filters, periodic resync"
weight: 60
---

# Controllers

A controller is how you keep a cluster converging on the state you want long after a one-shot script would have exited. You describe what a resource should look like, and the controller watches that resource and corrects it whenever reality drifts. In Starkite you write one with `k8s.control()`: a `.star` script that registers a handler and then hands control to the runtime, which drives a watch-based reconcile loop for as long as the process lives.

That blocking shape is the point. `k8s.control()` does not return — like `http.serve()`, it parks the script and becomes the program. Events arrive from the Kubernetes API server, deduplicate per resource, and dispatch to your handler; the runtime owns the parts you would otherwise rewrite by hand — watch reconnection, exponential backoff, rate-limited retries, and clean shutdown on SIGTERM or SIGINT. The cost is that the script is now a service, not a task: it holds a watch connection open, consumes a worker slot, and runs until something stops it.

The example below puts that loop to work enforcing a ceiling. It watches Deployments labeled `enforce-max-replicas=true`, and whenever one exceeds the configured maximum it scales the Deployment back down. Along the way it shows the three pieces you reach for most: the `reconcile` handler, label-selector filtering, and periodic resync.

**Source:** [`examples/cloud/controller/deployment-scaler.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/controller/deployment-scaler.star)

## Script

The handler is an ordinary function. It receives the event type and the object, decides what the object should be, and applies the correction. Here `reconcile` ignores deletions, treats a missing replica count as one, and re-applies the Deployment at the maximum whenever the live count runs over:

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

The `k8s.control()` call at the bottom is the line that never returns. It names the kind to watch, registers `reconcile` as the handler, scopes the watch to the label, and asks for a one-minute resync — and from there the runtime takes over.

## Run it

A controller needs something to react to, so you run it in one shell and feed it work from another. Start the controller first; with no overriding flag it enforces the default of three, and `--var` lets you raise that without touching the script:

```bash
kite run ./examples/cloud/controller/deployment-scaler.star
kite run ./examples/cloud/controller/deployment-scaler.star --var max_replicas=5
```

Now give it a violation to find. In a second shell, create a Deployment that exceeds the limit and apply the label the controller filters on:

```bash
kubectl create deployment nginx --image=nginx --replicas=10
kubectl label deployment nginx enforce-max-replicas=true
# the controller scales it down to max_replicas
```

The moment the label lands, the Deployment enters the watch, `reconcile` fires, and the controller scales it back to the maximum. Because the loop is long-running, you stop it explicitly with Ctrl-C — the blocking `k8s.control()` call registers a SIGTERM/SIGINT handler that tears down the watch cleanly rather than leaving a dangling connection.

## Handler shape

One handler is rarely the whole story. `k8s.control()` accepts four handler kwargs, and you pick the one — or the combination — that fits the workload. The catch-all `reconcile` sees every event with the event type as a string; the per-event handlers let you split create, update, and delete logic apart:

| Handler | Signature | Called on |
|---|---|---|
| `on_create` | `fn(obj)` | `ADDED` events |
| `on_update` | `fn(old, new)` | `MODIFIED` events |
| `on_delete` | `fn(obj)` | `DELETED` events |
| `reconcile` | `fn(event, obj)` | catch-all (any event type as a string) |

Whichever you use, the object arrives as an `AttrDict`. You read it with dot-access (`obj.metadata.namespace`) and mutate it with bracket-access (`obj["spec"]["replicas"] = 3`); list elements are AttrDicts too, so the same access rules apply all the way down.

## What's happening

The `reconcile` keyword is only one of several knobs on `k8s.control()`, and each one trades a little behavior for a little cost. The ones you will reach for:

- **`k8s.control(kind, ...)`** blocks the script and runs the reconcile loop. The first positional argument is the resource kind; everything after it is a kwarg.
- **`labels = "k=v"`** narrows the watch to resources matching the selector. Leave it off and every resource of that kind is watched — more events, more handler invocations.
- **`resync = "1m"`** re-fires `reconcile` on every known resource on that interval even when no API event has arrived. It is the safety net for events the watch missed, and the price is a periodic burst of handler calls.
- **`workers = N`** caps how many handler invocations run at once. The default of 1 serializes per resource; raise it only when reconciles are independent and you need the throughput.
- **`namespace = "ns"`** confines the watch to a single namespace. Omit it for a cluster-wide controller.
- **`k8s.apply(manifest)`** inside the handler performs a server-side apply. Pair it with owner references when you want the objects you create to be garbage-collected along with their parent.

## Generating deployment artifacts

Running a controller from your shell is how you develop it; shipping it to a cluster is a different job. A controller in production is a Deployment with an identity and a least-privilege grant, and `kite kube gen-controller-artifacts` writes that supporting cast for you — Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, and Deployment, plus an optional Dockerfile:

```bash
kite kube gen-controller-artifacts \
    --controller examples/cloud/controller/deployment-scaler.star \
    --image myregistry/myapp-controller:v1 \
    --namespace myapp-system > deploy.yaml

kubectl apply -f deploy.yaml
```

What comes out is a single manifest you apply with `kubectl`. When the controller manages a custom resource rather than a built-in kind, add `--resource <path>` and the generator emits a CRD alongside the rest (see [`examples/cloud/controller/resource.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/controller/resource.star) for the CRD definition pattern).

## See also

- [`k8s` reference](../references/api/k8s.md) — `control`, `obj.crd`, owned-resource watches, leader election
- [`kite kube` reference](../references/cli/kube.md) — `gen-controller-artifacts` flags
- [Webhooks](webhooks.md) — admission webhooks built on the same blocking-server pattern
- More controller examples: [`examples/cloud/controller/`](https://github.com/project-starkite/starkite/tree/main/examples/cloud/controller) — `configmap-sync.star` (event logging), `leader-election.star` (HA), `deploy-controller.star` (full programmatic install)
