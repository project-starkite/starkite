---
title: "Querying resources"
description: "Retrieve, list, filter, and monitor Kubernetes API objects"
weight: 14
---

# Querying resources

The `k8s` module represents Kubernetes resources as standard Starlark dictionaries. You can query individual resources, list collections, filter resources server-side, and monitor resource lifecycle events.

## Fetching a Single Resource

To fetch a specific resource by name and kind, use the `k8s.get()` function. The returned dictionary matches the resource's JSON representation:

```python
def check_workload():
    # Retrieve deployment details
    dep = k8s.get("deployment", "alice-web", namespace="staging")
    
    # Read fields directly using dictionary indexing
    metadata = dep["metadata"]
    spec = dep["spec"]
    
    print("Name:", metadata["name"])
    print("Desired Replicas:", spec["replicas"])
    print("Active Image:", spec["template"]["spec"]["containers"][0]["image"])
```

## Listing and Filtering Resources

To retrieve a collection of resources, use the `k8s.list()` function. This returns a list of dictionaries that you can iterate or process.

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
        print("  - Pod:", pod["metadata"]["name"], "IP:", pod["status"].get("podIP"))
```

## Waiting for Resource Conditions

To coordinate multi-step workflows (such as waiting for a database to become ready before running database migrations), use the `k8s.wait_for()` function. It blocks script execution until the resource reaches the specified condition or the timeout expires.

```python
def deploy_database():
    # Block until the database pod is ready
    print("Waiting for database connection...")
    k8s.wait_for(
        kind = "pod",
        name = "alice-db-0",
        namespace = "staging",
        condition = "Ready",
        timeout = "3m",
    )
    print("Database is ready. Executing migrations.")
```

## Watching API Events

To stream real-time events from the Kubernetes API, use the `k8s.watch()` function. This establishes a long-lived connection to the API server and dispatches incoming events to a handler function.

```python
def monitor_deployment_events():
    # Define an event handler
    def log_event(event_type, obj):
        print("Event Type:", event_type, "Resource:", obj["metadata"]["name"])
        
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
