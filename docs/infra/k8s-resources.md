---
title: "Querying resources"
description: "Retrieve, list, filter, and monitor Kubernetes API objects"
weight: 14
---

# Querying resources

The `k8s` module represents Kubernetes resources as `AttrDict` objects. You can query individual resources, list collections, filter resources server-side, and monitor resource lifecycle events using dot notation or dictionary indexing.

## Fetching a Single Resource

To fetch a specific resource by name and kind, use the `k8s.get()` function. The returned `AttrDict` supports direct dot-notation traversal:

```python
def check_workload():
    # Retrieve deployment details
    dep = k8s.get("deployment", "alice-web", namespace="staging")
    
    # Read fields directly using dot notation
    print("Name:", dep.metadata.name)
    print("Desired Replicas:", dep.spec.replicas)
    print("Active Image:", dep.spec.template.spec.containers[0].image)
```

## Listing and Filtering Resources

To retrieve a collection of resources, use the `k8s.list()` function. This returns a list of `AttrDict` objects that you can iterate or process.

### Server-Side Filtering

You can filter resources on the Kubernetes API server using label selectors or field selectors, reducing network transmission overhead.

```python
def query_filtered_pods():
    # Retrieve running pods with matching labels
    pods = k8s.list(
        kind = "pods",
        namespace = "staging",
        labels = "app=alice-web,team=platform",
        fields = "status.phase=Running",
    )
    
    print("Active Pods:")
    for pod in pods:
        print("  - Pod:", pod.metadata.name, "IP:", pod.status.get("podIP", "unassigned"))
```

### Specialized Inspection Helpers

In addition to general `k8s.list()`, Starkite provides dedicated query helpers for hardware claims and storage resources:

* `k8s.claims(namespace="", labels="")`: Lists `resource.k8s.io/v1` `ResourceClaim` objects.
* `k8s.pvcs(namespace="", labels="")`: Lists `PersistentVolumeClaim` objects.
* `k8s.pvs(labels="")`: Lists cluster-scoped `PersistentVolume` objects.
* `k8s.storage_classes(labels="")`: Lists cluster-scoped `StorageClass` definitions.

```python
def inspect_storage_and_devices():
    # List active hardware claims
    claims = k8s.claims(namespace="ml-workloads")
    for c in claims:
        print("Claim:", c.metadata.name, "Status:", c.status.get("allocation"))

    # Inspect persistent volume claims and matching PVs
    pvcs = k8s.pvcs(namespace="production")
    for pvc in pvcs:
        print("PVC:", pvc.metadata.name, "Phase:", pvc.status.phase, "Volume:", pvc.spec.get("volumeName"))

    # List cluster storage classes
    classes = k8s.storage_classes()
    for sc in classes:
        print("StorageClass:", sc.metadata.name, "Provisioner:", sc.provisioner)
```

## Waiting for Resource Conditions

To coordinate multi-step workflows (such as waiting for a database to become ready before running database migrations), use the `k8s.wait_for()` function. It blocks script execution until the resource reaches the specified condition or the timeout expires.

```python
def deploy_database():
    # Block until the database pod is ready
    print("Waiting for database connection...")
    result = k8s.wait_for(
        kind = "pod",
        name = "alice-db-0",
        namespace = "staging",
        condition = "Ready",
        timeout = "3m",
    )
    if result.ready:
        print("Database is ready. Executing migrations.")
    else:
        print("Database wait timed out:", result.message)
```

## Watching API Events

To stream real-time events from the Kubernetes API, use the `k8s.watch()` function. This establishes a long-lived connection to the API server and dispatches incoming events to a handler function.

```python
def monitor_deployment_events():
    # Define an event handler
    def log_event(event_type, obj):
        print("Event Type:", event_type, "Resource:", obj.metadata.name)
        
    # Watch deployments in the staging namespace for 30 seconds
    print("Starting deployment watch stream...")
    k8s.watch(
        kind = "deployment",
        namespace = "staging",
        timeout = "30s",
        handler = log_event,
    )
    print("Watch stream closed.")
```
