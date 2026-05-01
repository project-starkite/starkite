---
title: "k8s"
description: "Kubernetes resource management (Cloud edition)"
weight: 27
---

!!! note "Cloud functionality"
    The `k8s` module is available in `kite` (all-in-one) and `kitecloud`. It is **not** available in `kitecmd` or `kiteai`. See [Cloud Edition](../guides/cloud-edition.md).

The `k8s` module provides full Kubernetes resource management — CRUD, high-level workloads, watches, logs, exec, port-forward, node operations, metrics, controllers, admission webhooks, and typed object constructors.

All functions that perform I/O accept a `timeout` kwarg (duration string, e.g., `"30s"`, `"5m"`). Most take an optional `namespace` kwarg; when omitted, the client's default namespace is used.

## Quick reference

| Category | Functions |
|----------|-----------|
| [CRUD](#crud) | `get`, `list`, `create`, `apply`, `delete`, `patch`, `label`, `annotate`, `status` |
| [Watch & wait](#watch-and-wait) | `watch`, `wait_for` |
| [High-level workloads](#high-level-workloads) | `deploy`, `run`, `expose`, `scale`, `autoscale`, `rollout`, `set_image`, `set_env`, `set_resources` |
| [Logs, exec, port-forward, copy](#logs-exec-port-forward-copy) | `logs`, `logs_follow`, `exec`, `port_forward`, `cp` |
| [Describe](#describe) | `describe` |
| [Node operations](#node-operations) | `drain`, `cordon`, `uncordon`, `taint`, `untaint` |
| [Metrics](#metrics) | `top_nodes`, `top_pods` |
| [Context helpers](#context-helpers) | `context`, `namespace_name`, `version`, `api_resources` |
| [Controllers](#controllers) | `control` |
| [Webhooks](#admission-webhooks) | `webhook` |
| [Objects](#object-constructors) | `k8s.obj.crd`, `k8s.yaml`, `k8s.config` |

## CRUD

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.get(kind, name, namespace="", timeout="")` | `dict` | Get a single resource |
| `k8s.list(kind, namespace="", labels="", fields="", timeout="")` | `list[dict]` | List resources with optional label/field selectors |
| `k8s.create(manifest, namespace="", dry_run=False, timeout="")` | `dict` | Create a resource from a manifest (dict or YAML string) |
| `k8s.apply(manifest, namespace="", field_manager="starkite", dry_run=False, force=False, timeout="")` | `dict` | Apply a resource (server-side apply) |
| `k8s.delete(kind, name, namespace="", propagation="Background", timeout="")` | `None` | Delete a resource |
| `k8s.patch(kind, name, patch, namespace="", type="merge", timeout="")` | `dict` | Patch a resource. `type`: `"merge"`, `"strategic"`, or `"json"` |
| `k8s.label(kind, name, labels, namespace="", timeout="")` | `dict` | Set labels on a resource |
| `k8s.annotate(kind, name, annotations, namespace="", timeout="")` | `dict` | Set annotations on a resource |
| `k8s.status(obj, status, namespace="", timeout="")` | `dict` | Update the status subresource of a resource. Pass the full resource dict as `obj` and the new status dict as `status` |

### Example — status subresource update

```python
# Update the .status of a custom resource
obj = k8s.get("myapp", "demo", namespace="default")
k8s.status(obj, {"ready": True, "message": "initialized"}, namespace="default")
```

## Watch and wait

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.watch(kind, namespace="", labels="", timeout="", handler=None)` | `list` or `None` | Watch a resource kind. If `handler` is supplied, call it per event (`handler(event_type, obj)`) and return `None`; otherwise collect events and return a list of `{"type": ..., "object": ...}` dicts. `timeout` caps wall-clock duration |
| `k8s.wait_for(kind, name, condition="", namespace="", timeout="")` | `dict` | Block until the named resource meets the given condition (e.g., `"Available"`, `"Ready"`, `"Complete"`) or the timeout expires. Returns the final observed resource |

### Example — watch deployments in a namespace

```python
def on_event(event_type, obj):
    printf("%s: %s\n", event_type, obj["metadata"]["name"])

k8s.watch("deployment", namespace="default", timeout="30s", handler=on_event)
```

### Example — wait for rollout

```python
k8s.wait_for("deployment", "web", condition="Available",
             namespace="default", timeout="5m")
```

## High-level workloads

### Deploy and run

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.deploy(name, image, replicas=1, port=0, namespace="", labels=None, env=None, timeout="")` | `dict` | Create a Deployment |
| `k8s.run(name, image, command=None, namespace="", restart="Never", rm=False, timeout="3m")` | `dict` | Run a one-off Pod (like `kubectl run`) |
| `k8s.expose(kind, name, port, target_port=0, type="ClusterIP", namespace="", timeout="")` | `dict` | Expose a resource as a Service |

### Scale and rollout

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.scale(kind, name, replicas, namespace="", timeout="")` | `dict` | Scale a resource to the given replica count |
| `k8s.autoscale(kind, name, min=1, max=10, cpu_percent=80, namespace="", timeout="")` | `dict` | Create a HorizontalPodAutoscaler |
| `k8s.rollout(kind, name, action="status", namespace="", timeout="")` | `dict` | Manage rollouts. `action`: `"status"`, `"restart"`, `"pause"`, `"resume"` |

### Configuration

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.set_image(kind, name, container, image, namespace="", timeout="")` | `dict` | Update the container image of a resource |
| `k8s.set_env(kind, name, env, namespace="", container="", timeout="")` | `dict` | Set environment variables on a resource |
| `k8s.set_resources(kind, name, requests=None, limits=None, namespace="", container="", timeout="")` | `dict` | Set resource requests and limits |

## Logs, exec, port-forward, copy

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.logs(name, namespace="", container="", tail=0, since="", previous=False, timeout="")` | `string` | Fetch pod logs. `tail` caps line count; `since` is a duration string (e.g., `"10m"`); `previous=True` reads the previous container instance |
| `k8s.logs_follow(name, handler, namespace="", container="", tail=0, timeout="")` | `None` | Stream logs, calling `handler(line)` per line. Blocks until the pod ends or `timeout` elapses |
| `k8s.exec(name, command, namespace="", container="", timeout="")` | `dict` | Run a command in a pod. `command` may be a string (executed via `/bin/sh -c`) or a list (argv). Returns `{"stdout", "stderr", "exit_code"}` |
| `k8s.port_forward(name, port, local_port=0, namespace="")` | `dict` | Forward a local port to a pod port. Blocks until interrupted. `local_port=0` picks a free port |
| `k8s.cp(pod, src, dst, namespace="", container="", timeout="")` | `None` | Copy files to/from a pod. Use `pod:path` as `src` to download, `pod:path` as `dst` to upload |

### Example — tail logs

```python
def handle_line(line):
    if "ERROR" in line:
        printf("!! %s\n", line)

k8s.logs_follow("web-abc123", handle_line, namespace="default", tail=100)
```

### Example — exec into a pod

```python
result = k8s.exec("web-abc123", ["cat", "/etc/hostname"], namespace="default")
print(result["stdout"])
```

### Example — copy a file out of a pod

```python
k8s.cp("web-abc123", "web-abc123:/var/log/app.log", "./local-app.log",
       namespace="default")
```

## Describe

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.describe(kind, name, namespace="", timeout="")` | `string` | Return a human-readable description of a resource, similar to `kubectl describe` |

```python
print(k8s.describe("pod", "web-abc123", namespace="default"))
```

## Node operations

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.drain(node, force=False, ignore_daemonsets=False, timeout="")` | `dict` | Drain a node (evict pods). `force` continues past pods not backed by a controller; `ignore_daemonsets` leaves DaemonSet pods in place |
| `k8s.cordon(node, timeout="")` | `dict` | Mark a node unschedulable |
| `k8s.uncordon(node, timeout="")` | `dict` | Re-enable scheduling on a node |
| `k8s.taint(node, key, value="", effect="", timeout="")` | `dict` | Add a taint to a node. `effect`: `"NoSchedule"`, `"PreferNoSchedule"`, or `"NoExecute"` |
| `k8s.untaint(node, key, timeout="")` | `dict` | Remove a taint from a node by key |

### Example — roll a node

```python
k8s.cordon("node-01")
k8s.drain("node-01", ignore_daemonsets=True, timeout="5m")
# ... maintenance ...
k8s.uncordon("node-01")
```

## Metrics

Requires `metrics-server` running in the cluster.

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.top_nodes(timeout="")` | `list[dict]` | CPU/memory usage per node |
| `k8s.top_pods(namespace="", sort_by="", timeout="")` | `list[dict]` | CPU/memory usage per pod. `sort_by`: `"cpu"` or `"memory"` |

### Example

```python
for pod in k8s.top_pods(namespace="default", sort_by="cpu"):
    printf("%s  cpu=%s  mem=%s\n",
           pod["name"], pod["cpu"], pod["memory"])
```

## Context helpers

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.context()` | `string` | Current kubeconfig context name |
| `k8s.namespace_name()` | `string` | Default namespace for the current context |
| `k8s.version(timeout="")` | `dict` | Kubernetes server version info |
| `k8s.api_resources(timeout="")` | `list[dict]` | Available API resources |

```python
print("context:", k8s.context())
print("default namespace:", k8s.namespace_name())
print("server:", k8s.version()["gitVersion"])
```

## Controllers

`k8s.control()` runs a reconciliation loop over a resource kind. It's the starkite equivalent of writing a controller in `controller-runtime` — you supply handlers, the module owns the informer, work queue, retry policy, and optional leader election.

```python
k8s.control(kind, reconcile=..., on_create=..., on_update=..., on_delete=...,
            namespace="", labels="", field_selector="",
            resync="10m", workers=1, max_retries=5, backoff="5s",
            watch_owned=[], predicate=None,
            leader_election=False, leader_election_id="", leader_election_namespace="")
```

| Kwarg | Type | Default | Description |
|-------|------|---------|-------------|
| `kind` | string | **required** (positional) | Resource kind to watch |
| `reconcile` | callable | — | `fn(obj, meta) -> dict` — full reconcile handler. Receives the live object and metadata including event kind |
| `on_create` / `on_update` / `on_delete` | callable | — | Per-event handlers. At least one of `reconcile`/`on_create`/`on_update`/`on_delete` is required |
| `namespace` | string | cluster-wide | Scope the controller to a namespace |
| `labels` | string | — | Label selector (e.g., `"app=web"`) |
| `field_selector` | string | — | Field selector |
| `resync` | duration string | `"10m"` | Full-resync interval |
| `workers` | int | `1` | Number of concurrent work-queue workers |
| `max_retries` | int | `5` | Retry cap per event |
| `backoff` | duration string | `"5s"` | Base retry backoff |
| `watch_owned` | list[string] | — | Additional kinds to watch whose `ownerReferences` point at the primary kind; events trigger reconcile of the owner |
| `predicate` | callable | — | `fn(obj) -> bool` filter applied before enqueueing |
| `leader_election` | bool | `False` | Run as a leader-elected controller (only the elected replica processes events) |
| `leader_election_id` | string | — | Lease name for leader election |
| `leader_election_namespace` | string | — | Namespace for the leader-election Lease |

Blocks until interrupted (SIGINT/SIGTERM).

### Example — minimal reconciler

```python
def reconcile(obj, meta):
    printf("reconcile %s: phase=%s\n",
           obj["metadata"]["name"], obj["status"].get("phase", "-"))
    return {"requeue": False}

k8s.control("myapp", reconcile=reconcile,
            namespace="myapp-system", workers=2, leader_election=True,
            leader_election_id="myapp-lock",
            leader_election_namespace="myapp-system")
```

## Admission Webhooks

`k8s.webhook()` creates an HTTPS server that handles Kubernetes admission review requests. It blocks like `http.serve()` and `k8s.control()`.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `path` | string | **required** (first positional) | URL path (e.g., `/validate-myapp`) |
| `validate` | function | None | `fn(obj) -> {"allowed": bool, "message": str}` |
| `mutate` | function | None | `fn(obj) -> modified obj` |
| `port` | int | 9443 | HTTPS port |
| `tls_cert` | string | required | Path to TLS certificate |
| `tls_key` | string | required | Path to TLS private key |

### Validating Webhook

Rejects resources that don't meet criteria. Return `{"allowed": True}` to accept or `{"allowed": False, "message": "reason"}` to reject.

```python
def validate(obj):
    if obj.spec.replicas > 10:
        return {"allowed": False, "message": "max 10 replicas"}
    if not obj.metadata.labels.get("team"):
        return {"allowed": False, "message": "team label required"}
    return {"allowed": True}

k8s.webhook("/validate",
    validate = validate,
    port = 9443,
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

### Mutating Webhook

Modifies resources before they are persisted. The object is passed as a mutable AttrDict — modify it using bracket notation and return it. Changes are automatically converted to an RFC 6902 JSON patch.

```python
def mutate(obj):
    # Both dot-access (read) and bracket-access (read/write) work
    printf("Mutating: %s\n", obj.metadata.name)

    # Write via bracket notation
    obj["metadata"]["labels"]["managed-by"] = "starkite"
    obj["metadata"]["annotations"]["mutated"] = "true"
    return obj

k8s.webhook("/mutate",
    mutate = mutate,
    port = 9443,
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

### Combined Webhook

Both validate and mutate on the same server. Validation runs first — if rejected, mutation is skipped.

```python
k8s.webhook("/webhook",
    validate = validate_fn,
    mutate = mutate_fn,
    port = 9443,
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

### AttrDict Object Access

Objects passed to webhook handlers are AttrDicts with both dot-access and bracket-access:

```python
# Dot-access for reading (convenient)
name = obj.metadata.name
replicas = obj.spec.replicas
labels = obj.metadata.labels

# Bracket-access for reading and writing
obj["metadata"]["labels"]["key"] = "value"
obj["spec"]["replicas"] = 3
```

Nested maps share the same underlying data — mutations via bracket notation on a nested AttrDict propagate to the parent object automatically.

See the [webhooks guide](../guides/webhooks.md) for a full end-to-end workflow including `gen-webhook-artifacts`.

## Object constructors

The `k8s.obj` namespace provides typed constructors for building Kubernetes resources programmatically.

### `k8s.obj.crd()`

Constructs a CustomResourceDefinition manifest.

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `group` | `string` | *required* | API group for the CRD (e.g., `"example.io"`) |
| `version` | `string` | *required* | API version (e.g., `"v1"`, `"v1alpha1"`) |
| `kind` | `string` | *required* | Resource kind in PascalCase (e.g., `"MyApp"`) |
| `plural` | `string` | *required* | Plural name used in API paths (e.g., `"myapps"`) |
| `scope` | `string` | `"Namespaced"` | `"Namespaced"` or `"Cluster"` |
| `spec` | `dict` | `{}` | Schema fields for the `spec` section. Each key maps to `{"type": "<type>"}` with optional `"required"` and `"default"` |
| `status` | `dict` | `{}` | Schema fields for the `status` subresource, same format as `spec` |

**Spec and status schema format:**

Each field is a dict entry where the key is the field name and the value describes the type and constraints:

```python
{
    "fieldName": {"type": "string"},                          # simple field
    "replicas":  {"type": "integer", "default": 1},           # with default
    "image":     {"type": "string", "required": True},        # required field
    "ready":     {"type": "boolean"},                         # boolean field
}
```

Supported types: `"string"`, `"integer"`, `"boolean"`, `"number"`, `"array"`, `"object"`.

**Example — define and apply a CRD:**

```python
crd = k8s.obj.crd(
    group = "example.io",
    version = "v1",
    kind = "MyApp",
    plural = "myapps",
    scope = "Namespaced",
    spec = {
        "image": {"type": "string", "required": True},
        "replicas": {"type": "integer", "default": 1},
    },
    status = {
        "ready": {"type": "boolean"},
        "message": {"type": "string"},
    },
)

# Apply the CRD to the cluster
k8s.apply(crd)

# Print the generated YAML for review
print(k8s.yaml(crd))
```

### Utilities

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.yaml(manifest)` | `string` | Render a manifest dict as YAML |
| `k8s.config(kubeconfig="", context="", namespace="")` | `None` | Configure the default k8s client (kubeconfig path, context, default namespace). Usually inferred from env |

## Examples

### Get and list resources

```python
# Get a specific pod
pod = k8s.get("pod", "web-abc123", namespace="default")
print(pod["status"]["phase"])

# List pods by label
pods = k8s.list("pod", namespace="default", labels="app=web")
for p in pods:
    print(p["metadata"]["name"], p["status"]["phase"])
```

### Create from YAML

```python
manifest = yaml.decode(read_text("deployment.yaml"))
k8s.create(manifest, namespace="production")
```

### Apply a manifest

```python
k8s.apply({
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {"name": "app-config"},
    "data": {"key": "value"},
}, namespace="default")
```

### Deploy and expose

```python
k8s.deploy("web", "nginx:latest", replicas=3, port=80, namespace="default",
    labels={"app": "web", "tier": "frontend"},
    env={"ENV": "production"},
)
k8s.expose("deployment", "web", port=80, type="LoadBalancer", namespace="default")
```

### Scale and autoscale

```python
k8s.scale("deployment", "web", replicas=5, namespace="default")
k8s.autoscale("deployment", "web", min=2, max=10, cpu_percent=70, namespace="default")
```

### Rollout management

```python
# Restart a deployment
k8s.rollout("deployment", "web", action="restart", namespace="default")

# Check rollout status
status = k8s.rollout("deployment", "web", action="status", namespace="default")
print(status)

# Pause a rollout (e.g., during debugging)
k8s.rollout("deployment", "web", action="pause", namespace="default")
# Resume a paused rollout
k8s.rollout("deployment", "web", action="resume", namespace="default")
```

### Update container image

```python
k8s.set_image("deployment", "web", "nginx", "nginx:1.25", namespace="default")
```

### Set environment variables

```python
k8s.set_env("deployment", "web", {"LOG_LEVEL": "debug", "DB_HOST": "db-01"},
    namespace="default", container="web")
```

### Set resource limits

```python
k8s.set_resources("deployment", "web",
    requests={"cpu": "100m", "memory": "128Mi"},
    limits={"cpu": "500m", "memory": "512Mi"},
    namespace="default",
)
```

### Run a one-off job

```python
result = k8s.run("debug", "busybox", command=["sh", "-c", "nslookup kubernetes"],
    namespace="default", rm=True, timeout="1m")
```

### Delete and patch

```python
k8s.delete("pod", "web-abc123", namespace="default")

k8s.patch("deployment", "web", {"spec": {"replicas": 5}},
    namespace="default", type="merge")
```

### Labeling and annotating

```python
k8s.label("pod", "web-abc123", {"version": "v2", "canary": "true"}, namespace="default")
k8s.annotate("deployment", "web", {"deploy-note": "hotfix"}, namespace="default")
```

### Cluster info

```python
ver = k8s.version()
print("Kubernetes:", ver["gitVersion"])

resources = k8s.api_resources()
for r in resources:
    print(r["name"], r["kind"])
```

> **Note:**
All `k8s` functions that can fail support `try_` variants that return a `Result` instead of raising an error.
