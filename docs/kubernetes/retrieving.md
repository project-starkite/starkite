---
title: "Retrieving objects"
description: "Get, list, and filter Kubernetes resources"
weight: 30
---

# Retrieving objects

Most automation starts by reading what is already in the cluster — the deployment you are about to scale, the pods behind a service, the job you are waiting on. The `k8s` module gives you two reads for that: `get` returns a single named object, and `list` returns a collection, with optional label and field selectors to narrow it. Both come back as plain dicts, so you walk the result the same way you would walk parsed JSON. Reading is covered by the `k8s.read` capability, which the `allow-local` profile grants, so a read-only script runs without reaching for broader permission.

## Get a single object

When you know exactly which object you want, name it. `k8s.get(kind, name, namespace="", timeout="")` fetches one resource and hands it back as a dict — the same structure `kubectl get -o json` produces:

```python
dep = k8s.get("deployment", "web", namespace="default")
print(dep["spec"]["replicas"])
```

The returned dict mirrors the object's full shape, so you index straight into `spec`, `status`, or `metadata` to pull out the field you came for — here, the replica count.

## List a collection

When you want every object of a kind rather than one by name, reach for `list`. It returns a list of dicts, one per resource, which you iterate:

```python
pods = k8s.list("pods", namespace="default")
for pod in pods:
    print(pod["metadata"]["name"], pod["status"]["phase"])
```

Each element is a full object dict, so the loop can read any field on the resource — above, the name and lifecycle phase of every pod in the namespace.

## Filtering with selectors

Listing an entire namespace is rarely what you want on a busy cluster, so `list` accepts `labels` and `fields` selectors to filter server-side before the results ever reach your script:

```python
# Label selector
web_pods = k8s.list("pods", namespace="default", labels="app=web")

# Field selector
running = k8s.list("pods", labels="app=web", fields="status.phase=Running")
```

The first call returns only pods carrying `app=web`; the second narrows further to those whose phase is `Running`. Because the API server applies the selectors, you transfer and iterate fewer objects, which matters when the unfiltered set is large.

## Waiting for a condition

Reads tell you the cluster's state right now, but automation often needs to pause until that state changes. `wait_for` blocks until a named resource reaches a condition, or the timeout expires:

```python
k8s.wait_for("deployment", "web", condition="Available", timeout="2m")
```

This holds the script until the `web` deployment reports `Available`, returning the final observed resource — so the next line runs only once the deployment is actually ready, rather than racing it. The `timeout` caps how long you are willing to wait before the call gives up.

## Watching for changes

When you need to react to a stream of changes instead of a single condition, `watch` delivers events as they happen. Pass a handler and it is called once per event:

```python
def on_event(event_type, obj):
    printf("%s: %s\n", event_type, obj["metadata"]["name"])

k8s.watch("deployment", namespace="default", timeout="30s", handler=on_event)
```

The handler receives the event type (added, modified, deleted) and the object dict for each change, so `on_event` can inspect or act on every deployment update as it arrives. The `timeout` bounds how long the watch runs before it returns.

See the [k8s API reference](../references/api/k8s.md#crud) for every read operation and selector option. Objects come back as plain dicts — see [Object representation](objects.md).
