---
title: "Retrieving objects"
description: "Get, list, and filter Kubernetes resources"
weight: 30
---

# Retrieving objects

The `k8s` module reads resources with `get` (one object) and `list` (a collection), with optional label and field selectors.

## Get a single object

```python
dep = k8s.get("deployment", "web", namespace="default")
print(dep["spec"]["replicas"])
```

`k8s.get(kind, name, namespace="", timeout="")` returns the resource as a dict — the same structure `kubectl get -o json` produces.

## List a collection

```python
pods = k8s.list("pods", namespace="default")
for pod in pods:
    print(pod["metadata"]["name"], pod["status"]["phase"])
```

## Filtering with selectors

`list` accepts `labels` and `fields` selectors:

```python
# Label selector
web_pods = k8s.list("pods", namespace="default", labels="app=web")

# Field selector
running = k8s.list("pods", labels="app=web", fields="status.phase=Running")
```

## Waiting for a condition

`wait_for` blocks until a resource reaches a condition (or the timeout expires):

```python
k8s.wait_for("deployment", "web", condition="Available", timeout="2m")
```

## Watching for changes

`watch` streams events; pass a handler to process each one:

```python
def on_event(event_type, obj):
    printf("%s: %s\n", event_type, obj["metadata"]["name"])

k8s.watch("deployment", namespace="default", timeout="30s", handler=on_event)
```

See the [k8s API reference](../references/api/k8s.md#crud) for every read operation and selector option. Objects come back as plain dicts — see [Object representation](objects.md).
