---
title: "Kubernetes Controllers"
description: "Create background Kubernetes controllers and reconcile loops in Starlark"
weight: 18
---

# Kubernetes Controllers

A Kubernetes controller is an active reconciliation process that continually aligns the actual state of cluster resources with your declared configuration. Starkite implements this pattern through a background reconcile loop registered via the `k8s.control()` API. The loop runs continuously in the background, watching for API events—such as resource creation, modification, or deletion—and executing Starlark handler functions to correct any detected configuration drift.

## How Reconcile Loops Work

The `k8s.control()` function is a blocking call. It parks the Starlark script and runs a watch-based reconcile loop. The Starkite runtime manages the lower-level controller details, including:
* Reestablishing watch connections during network interruptions.
* Enforcing exponential backoff and rate-limiting for retries.
* Managing concurrent worker threads.
* Handling clean shutdowns on `SIGTERM` or `SIGINT`.

---

## Example: Pod Event Logger

This controller watches a resource and logs lifecycle events. The script registers a reconcile loop that watches all pods in the `default` namespace and prints their status changes:

```python
# pod-logger-controller.star
# Logs lifecycle events for all pods in the default namespace

def reconcile(event, obj):
    # event is a string: ADDED, MODIFIED, or DELETED
    # obj is an AttrDict representing the pod resource
    print("Event:", event, "| Pod Name:", obj.metadata.name, "| Status:", obj.status.phase)

print("Starting pod logger controller...")
k8s.control(
    kind = "pods",
    reconcile = reconcile,
    namespace = "default",
)
```

### Under the Hood
* **Registration**: Calling `k8s.control(kind="pods", ...)` registers the `reconcile` handler for the Pod resource.
* **The Callback**: Whenever a pod is created, updated, or deleted, Kubernetes emits an API event. Starkite intercepts this event and invokes the `reconcile(event, obj)` function.
* **The Resource Object**: The `obj` argument is passed as an `AttrDict`, allowing you to read nested fields using dot notation (e.g., `obj.status.phase`).

---

## Advanced Example: Enforcing Maximum Replicas

## Advanced Example: Enforcing Maximum Replicas

Controllers actively correct configuration drift. The following controller watches deployments labeled with `enforce-max-replicas=true`. If a deployment's replica count exceeds a configured maximum, the controller scales it back down via merge patch and emits a Kubernetes event. Built-in self-echo suppression ensures the controller's patch does not trigger a feedback loop.

```python
# max-replicas-controller.star
# Enforces a maximum replica limit on labeled deployments

max_replicas = var_int("max_replicas", 3)

def reconcile(deploy):
    name = deploy.metadata.name
    ns = deploy.metadata.namespace
    replicas = deploy.spec.get("replicas", 1)

    if replicas > max_replicas:
        print("[DRIFT DETECTED] %s/%s replicas=%d exceeds max=%d" % (ns, name, replicas, max_replicas))
        
        # Patch replicas to allowed maximum
        k8s.patch("deployment", name, {
            "spec": {"replicas": max_replicas}
        }, namespace=ns)

        # Emit an event attached to the deployment
        k8s.event(
            deploy,
            reason = "DriftCorrected",
            message = "Scaled down replicas from %d to %d" % (replicas, max_replicas),
            type = "Normal",
            namespace = ns,
        )
    else:
        print("[IN POLICY] %s/%s replicas=%d within limit (<= %d)" % (ns, name, replicas, max_replicas))

print("Starting max-replicas controller. Max allowed:", max_replicas)
k8s.control(
    "deployments",
    reconcile = reconcile,
    labels = "enforce-max-replicas=true",
    poll = "30s",
    health_port = 8081,
    workers = 2,
)
```

### Running the Controller Locally

Start the controller by passing the script to `kite run`:

```bash
# Start with default max_replicas=3
kite run ./max-replicas-controller.star --allow-all

# Override variables at startup
kite run ./max-replicas-controller.star --allow-all --var max_replicas=5
```

Test drift correction and health probes in another terminal:

```bash
# Create a deployment with 10 replicas and label it
kubectl create deployment alice-web --image=nginx:alpine --replicas=10
kubectl label deployment alice-web enforce-max-replicas=true

# Inspect the auto-corrected replica count and emitted event
kubectl get deployment alice-web
kubectl get events --field-selector involvedObject.name=alice-web

# Probe the embedded health server
curl -i http://localhost:8081/healthz
```

---

## Handler Signatures

`k8s.control()` supports functional child reconciliation, declarative finalization, or specific lifecycle hooks:

| Handler | Signature | Trigger Event / Lifecycle Role |
|:---|:---|:---|
| `reconcile` | `fn(obj)` or `fn(event, obj)` | Main reconciliation loop. Can return `None`, requeue duration string (e.g., `"10s"`), or `list[KubeResource]` of desired child resources |
| `finalize` | `fn(obj)` | Declarative teardown hook executed when `metadata.deletionTimestamp` is set before the finalizer is removed |
| `on_create` | `fn(obj)` | Triggered on `ADDED` events when resources are created |
| `on_update` | `fn(old, new)` | Triggered on `MODIFIED` events when resources are changed |
| `on_delete` | `fn(obj)` | Triggered on `DELETED` events when resources are removed |

---

## Configuration Parameters

You can customize controller execution using keyword arguments in `k8s.control()`:

* **`labels`**: Filters watched resources using a standard label selector (e.g., `labels="app=web"`).
* **`field_selector`**: Filters watched resources using field expressions (e.g., `field_selector="metadata.name=web"`).
* **`poll`**: A duration string (e.g., `"30s"`) that periodically requeues all resources for background reconciliation.
* **`resync`**: Informer full cache resync interval (e.g., `"10m"`).
* **`health_port`**: An integer port number (e.g., `8081`) running an embedded HTTP server exposing `/healthz` and `/readyz`.
* **`finalizer`**: Custom finalizer name. Automatically added to active resources and removed once `finalize()` returns cleanly.
* **`generation_changed`**: Boolean (default `True`). Suppresses reconciliation when updates only modify status or metadata without changing `.metadata.generation`.
* **`watch_owned`**: A list of child resource kinds (e.g., `["deployments", "services"]`) whose `ownerReferences` point to the primary resource.
* **`watch_related`**: A list of secondary resource mappings: `[{"kind": "secrets", "map_func": map_fn}]` or `[("secrets", map_fn)]`.
* **`workers`**: Integer count of concurrent workqueue workers.
* **`leader_election`**: Boolean enabling distributed `coordination.k8s.io/v1` `Lease` locking. Standby replicas maintain passive informers; `/readyz` returns 200 on the leader and 503 on standbys.
* **`identity`**: Identifier string for the replica (defaults to `<hostname>_<pid>`).

---

## Operator Pattern: Child Reconciliation & Finalizers

When writing Custom Resource Definition (CRD) operators, `reconcile(obj)` can return a list of desired child resources. The runtime automates child ownership, Server-Side Apply, and orphan pruning:

```python
def reconcile(site):
    name = site.metadata.name
    ns = site.metadata.namespace
    replicas = site.spec.get("replicas", 1)

    child_dep = k8s.obj.deployment(
        name = name,
        namespace = ns,
        replicas = replicas,
        containers = [k8s.obj.container(name="web", image="nginx:alpine")],
    )
    child_svc = k8s.obj.service(
        name = name,
        namespace = ns,
        ports = [k8s.obj.service_port(port=80, target_port=80)],
    )

    # Returning desired child resources triggers:
    # 1. Automatic ownerReference injection pointing to site
    # 2. Dynamic auto-watching on deployment and service kinds
    # 3. Server-Side Apply with fieldManager="starkite"
    # 4. Orphan pruning when items are removed from this list
    # 5. Inferred Ready condition in status.conditions
    return [child_dep, child_svc]

def finalize(site):
    # Runs before finalizer is stripped and resource is removed
    print("Performing pre-deletion cleanup for", site.metadata.name)
    return None

k8s.control(
    "staticsites",
    reconcile = reconcile,
    finalize = finalize,
    finalizer = "tutorial.starkite.io/finalizer",
    health_port = 8081,
    workers = 2,
)
```

---

## Deploying Controllers to a Cluster

Once you have validated your controller script locally, you must deploy it to run continuously inside your Kubernetes cluster. Rather than requiring you to author complex deployment manifests and role-based access control (RBAC) configurations by hand, the Starkite CLI automates this process.

### Generating Manifests

The `kite kube gen-controller-artifacts` command inspects your Starlark script and generates a complete, self-contained Kubernetes manifest stream containing:
1. **Namespace**: A dedicated namespace to isolate your controller.
2. **ServiceAccount**: The identity your controller runs under.
3. **ClusterRole and Binding**: The RBAC permissions required by the controller to watch resources and apply modifications.
4. **Deployment**: A single-replica workload that executes your Starlark script within the cluster.

Run the generator and save the output to a file:

```bash
# Generate all deployment and RBAC manifests
kite kube gen-controller-artifacts \
    --controller ./max-replicas-controller.star \
    --image myregistry/max-replicas-controller:v1 \
    --namespace alice-system > deploy.yaml
```

### Applying to the Cluster

Deploy the generated manifests directly to your cluster using `kubectl`:

```bash
# Deploy the controller and its RBAC resources
kubectl apply -f deploy.yaml
```

The controller will start running in the `alice-system` namespace, automatically mounting the required cluster credentials and executing the Starlark reconcile loop as a background daemon.
