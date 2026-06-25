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

Real-world controllers do more than log events—they actively correct configuration drift. The following controller watches deployments labeled with `enforce-max-replicas=true`. If a deployment's replica count exceeds a configured maximum, the controller automatically scales it back down to the allowed limit using Server-Side Apply.

```python
# max-replicas-controller.star
# Enforces a maximum replica limit on labeled deployments

# Read maximum replica limit from runtime variable
max_replicas = var_int("max_replicas", 3)

def reconcile(event, obj):
    # Handle resource deletions
    if event == "DELETED":
        log.info("Resource deleted", {"namespace": obj.metadata.namespace, "name": obj.metadata.name})
        return

    # Read current replicas, defaulting to 1 if not set
    replicas = obj.spec.replicas
    if replicas == None:
        replicas = 1

    # Check if replicas exceed the allowed maximum
    if replicas > max_replicas:
        log.info("Scaling down resource", {
            "namespace": obj.metadata.namespace,
            "name": obj.metadata.name,
            "current": replicas,
            "max": max_replicas,
        })
        
        # Apply the correction using Server-Side Apply
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
        print("Resource within limits:", obj.metadata.name, "replicas:", replicas)

# Register the reconcile loop
print("Starting max-replicas controller. Max allowed:", max_replicas)
k8s.control(
    kind = "deployments",
    reconcile = reconcile,
    labels = "enforce-max-replicas=true",
    resync = "1m",
)
```

### Running the Controller Locally

Start the controller by passing the script to `kite run`:

```bash
# Start with default max_replicas=3
kite run ./max-replicas-controller.star --permissions=allow-local

# Override variables at startup
kite run ./max-replicas-controller.star --permissions=allow-local --var max_replicas=5
```

In a separate terminal, trigger a violation to test the controller:

```bash
# Create a deployment with 10 replicas
kubectl create deployment alice-web --image=nginx --replicas=10

# Label the deployment to bring it under controller scope
kubectl label deployment alice-web enforce-max-replicas=true

# The controller will detect the drift and scale it down to the maximum allowed limit
```

---

## Handler Signatures

`k8s.control()` allows you to register specific event handlers or a single catch-all reconcile function:

| Handler | Signature | Trigger Event |
|:---|:---|:---|
| `on_create` | `fn(obj)` | Triggered on `ADDED` events when resources are created |
| `on_update` | `fn(old, new)` | Triggered on `MODIFIED` events when resources are changed |
| `on_delete` | `fn(obj)` | Triggered on `DELETED` events when resources are removed |
| `reconcile` | `fn(event, obj)` | Catch-all handler triggered on any event type (passed as a string) |

---

## Configuration Parameters

You can customize the loop execution using the following optional arguments in `k8s.control()`:

* **`labels`**: Filters watched resources using a standard label selector (e.g., `labels = "app=web"`).
* **`resync`**: A duration string (e.g., `"1m"`, `"5m"`) that forces the controller to re-run handlers on all known resources periodically, ensuring eventual consistency.
* **`workers`**: An integer defining the maximum number of concurrent executions allowed for your handlers.
* **`namespace`**: Pins the watch loop to a single namespace instead of watching all namespaces cluster-wide.

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
