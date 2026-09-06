---
title: "k8s"
description: "Kubernetes resource management (Cloud edition)"
weight: 27
keywords: [k8s, kubernetes, cluster, pod, deployment, service, controller, webhook, config, apply]
---

!!! note "Cloud functionality"
    The `k8s` module is available in `kite` (all-in-one) and `kitecloud`. It is **not** available in `kitecmd` or `kiteai`. See [Infrastructure](../../infra/k8s-connect.md).

The `k8s` module provides full Kubernetes resource management — CRUD, high-level workloads, watches, logs, exec, port-forward, node operations, metrics, controllers, admission webhooks, and typed object constructors.

All functions that perform I/O accept a `timeout` kwarg (duration string, e.g., `"30s"`, `"5m"`). Most take an optional `namespace` kwarg; when omitted, the client's default namespace is used.

## Quick reference

| Category | Functions |
|----------|-----------|
| [CRUD](#crud) | `get`, `list`, `create`, `apply`, `delete`, `patch`, `label`, `annotate`, `status`, `event` |
| [Conditions & Finalizers](#conditions-and-finalizers) | `k8s.condition.*`, `k8s.finalizer.*`, `k8s.is_deleting` |
| [Watch & wait](#watch-and-wait) | `watch`, `wait_for` |
| [High-level workloads](#high-level-workloads) | `deploy`, `run`, `expose`, `scale`, `autoscale`, `rollout`, `set_image`, `set_env`, `set_resources` |
| [Logs, exec, port-forward, copy](#logs-exec-port-forward-copy) | `logs`, `logs_follow`, `exec`, `port_forward`, `cp` |
| [Describe](#describe) | `describe` |
| [Node operations](#node-operations) | `drain`, `cordon`, `uncordon`, `taint`, `untaint` |
| [Metrics](#metrics) | `top_nodes`, `top_pods` |
| [Context helpers](#context-helpers) | `context`, `namespace_name`, `version`, `api_resources` |
| [Controllers](#controllers) | `control` |
| [Webhooks](#admission-webhooks) | `webhook` |
| [Object constructors & utils](#object-constructors) | `k8s.obj.*`, `k8s.yaml`, `k8s.config` |

## CRUD

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.get(kind, name, namespace="", timeout="")` | `AttrDict` | Get a single resource |
| `k8s.list(kind, namespace="", labels="", fields="", timeout="")` | `list[AttrDict]` | List resources with optional label/field selectors |
| `k8s.create(manifest, namespace="", dry_run=False, timeout="")` | `AttrDict` | Create a resource from a manifest (dict, AttrDict, or YAML string) |
| `k8s.apply(manifest, namespace="", field_manager="starkite", dry_run=False, force=False, timeout="")` | `AttrDict` | Apply a resource (server-side apply) |
| `k8s.delete(kind, name, namespace="", propagation="Background", timeout="")` | `None` | Delete a resource |
| `k8s.patch(kind, name, patch, namespace="", type="merge", timeout="")` | `AttrDict` | Patch a resource. `type`: `"merge"`, `"strategic"`, or `"json"` |
| `k8s.label(kind, name, labels, namespace="", timeout="")` | `AttrDict` | Set labels on a resource |
| `k8s.annotate(kind, name, annotations, namespace="", timeout="")` | `AttrDict` | Set annotations on a resource |
| `k8s.status(obj, status, namespace="", timeout="")` | `AttrDict` | Update the status subresource of a resource. Pass the resource as `obj` and the new status dict as `status` |
| `k8s.event(obj, reason, message, type="Normal", namespace="", timeout="")` | `AttrDict` | Emit a Kubernetes event attached to `obj`. `type` can be `"Normal"` or `"Warning"` |
| `k8s.claims(namespace="", labels="", timeout="")` | `list[AttrDict]` | List `resource.k8s.io/v1` `ResourceClaim` objects for Dynamic Resource Allocation (DRA) |
| `k8s.pvcs(namespace="", labels="", timeout="")` | `list[AttrDict]` | List `PersistentVolumeClaim` objects |
| `k8s.pvs(labels="", timeout="")` | `list[AttrDict]` | List cluster-scoped `PersistentVolume` objects |
| `k8s.storage_classes(labels="", timeout="")` | `list[AttrDict]` | List cluster-scoped `StorageClass` objects |

### Example — status subresource update

```python
# Update the .status of a custom resource
obj = k8s.get("myapp", "demo", namespace="default")
k8s.status(obj, {"ready": True, "message": "initialized"}, namespace="default")
```

### Example — emitting a Kubernetes event

```python
deploy = k8s.get("deployment", "web", namespace="default")
k8s.event(deploy, reason="DriftCorrected", message="Replicas scaled down to policy limit", type="Normal")
```

## Conditions and Finalizers

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.condition.get(obj, type)` | `AttrDict` or `None` | Query a condition by `type` (e.g., `"Ready"`) from `obj.status.conditions` |
| `k8s.condition.set(obj, type, status, reason="", message="", timeout="")` | `AttrDict` | Set or update a condition in `obj.status.conditions` and persist it via status subresource |
| `k8s.finalizer.has(obj, name)` | `bool` | Check if `name` is present in `obj.metadata.finalizers` |
| `k8s.finalizer.add(obj, name, timeout="")` | `AttrDict` | Add a finalizer string to `obj.metadata.finalizers` via merge patch |
| `k8s.finalizer.remove(obj, name, timeout="")` | `AttrDict` | Remove a finalizer string from `obj.metadata.finalizers` via merge patch |
| `k8s.is_deleting(obj)` | `bool` | Check if `obj.metadata.deletionTimestamp` is set |


## Watch and wait

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.watch(kind, namespace="", labels="", timeout="", handler=None)` | `list[AttrDict]` or `None` | Watch a resource kind. If `handler` is supplied, call it per event (`handler(event_type, obj)`) and return `None`; otherwise collect events and return a list of `{"type": ..., "object": ...}` AttrDicts. `timeout` caps wall-clock duration |
| `k8s.wait_for(kind, name, condition="", namespace="", timeout="")` | `AttrDict` | Block until the named resource meets the given condition (e.g., `"Available"`, `"Ready"`, `"Complete"`) or the timeout expires. Returns `{"ready": bool, "resource": AttrDict, "message": str}` |

### Example — watch deployments in a namespace

```python
def on_event(event_type, obj):
    printf("%s: %s\n", event_type, obj.metadata.name)

k8s.watch("deployment", namespace="default", timeout="30s", handler=on_event)
```

### Example — wait for rollout

```python
result = k8s.wait_for("deployment", "web", condition="Available",
                      namespace="default", timeout="5m")
if result.ready:
    print("Deployment is ready")
```

## High-level workloads

### Deploy and run

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.deploy(name, image, replicas=1, port=0, namespace="", labels=None, env=None, timeout="")` | `AttrDict` | Create a Deployment (returns `{"deployment": str, "service": str}`) |
| `k8s.run(name, image, command=None, namespace="", restart="Never", rm=False, timeout="3m")` | `AttrDict` | Run a one-off Pod (like `kubectl run`) |
| `k8s.expose(kind, name, port, target_port=0, type="ClusterIP", namespace="", timeout="")` | `AttrDict` | Expose a resource as a Service |

### Scale and rollout

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.scale(kind, name, replicas, namespace="", timeout="")` | `AttrDict` | Scale a resource to the given replica count |
| `k8s.autoscale(kind, name, min=1, max=10, cpu_percent=80, namespace="", timeout="")` | `AttrDict` | Create a HorizontalPodAutoscaler |
| `k8s.rollout(kind, name, action="status", namespace="", timeout="")` | `AttrDict` | Manage rollouts. `action`: `"status"`, `"restart"`, `"pause"`, `"resume"` |

### Configuration

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.set_image(kind, name, container, image, namespace="", timeout="")` | `AttrDict` | Update the container image of a resource |
| `k8s.set_env(kind, name, env, namespace="", container="", timeout="")` | `AttrDict` | Set environment variables on a resource |
| `k8s.set_resources(kind, name, requests=None, limits=None, namespace="", container="", timeout="")` | `AttrDict` | Set resource requests and limits |

## Logs, exec, port-forward, copy

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.logs(name, namespace="", container="", tail=0, since="", previous=False, timeout="")` | `string` | Fetch pod logs. `tail` caps line count; `since` is a duration string (e.g., `"10m"`); `previous=True` reads the previous container instance |
| `k8s.logs_follow(name, handler, namespace="", container="", tail=0, timeout="")` | `None` | Stream logs, calling `handler(line)` per line. Blocks until the pod ends or `timeout` elapses |
| `k8s.exec(name, command, namespace="", container="", timeout="")` | `AttrDict` | Run a command in a pod. `command` may be a string (executed via `/bin/sh -c`) or a list (argv). Returns `{"stdout", "stderr", "code"}` |
| `k8s.port_forward(name, port, local_port=0, namespace="")` | `PortForwardHandle` | Forward a local port to a pod port. Blocks until interrupted. `local_port=0` picks a free port |
| `k8s.cp(pod, src, dst, namespace="", container="", timeout="")` | `AttrDict` | Copy files to/from a pod. Use `pod:path` as `src` to download, `pod:path` as `dst` to upload |

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
print(result.stdout)
```

### Example — copy a file out of a pod

```python
k8s.cp("web-abc123", "web-abc123:/var/log/app.log", "./local-app.log",
       namespace="default")
```

## Describe

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.describe(kind, name, namespace="", timeout="")` | `AttrDict` | Return detailed description of a resource, with `.resource`, `.conditions`, and `.events` |

```python
info = k8s.describe("pod", "web-abc123", namespace="default")
print("Pod Phase:", info.resource.status.phase)
```

## Node operations

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.drain(node, force=False, ignore_daemonsets=False, timeout="")` | `AttrDict` | Drain a node (evict pods). `force` continues past pods not backed by a controller; `ignore_daemonsets` leaves DaemonSet pods in place |
| `k8s.cordon(node, timeout="")` | `AttrDict` | Mark a node unschedulable |
| `k8s.uncordon(node, timeout="")` | `AttrDict` | Re-enable scheduling on a node |
| `k8s.taint(node, key, value="", effect="", timeout="")` | `AttrDict` | Add a taint to a node. `effect`: `"NoSchedule"`, `"PreferNoSchedule"`, or `"NoExecute"` |
| `k8s.untaint(node, key, timeout="")` | `AttrDict` | Remove a taint from a node by key |

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
| `k8s.top_nodes(timeout="")` | `list[AttrDict]` | CPU/memory capacity and allocatable per node |
| `k8s.top_pods(namespace="", sort_by="", timeout="")` | `list[AttrDict]` | Resource requests and status per pod |

### Example

```python
for pod in k8s.top_pods(namespace="default", sort_by="cpu"):
    printf("%s  cpu=%s  mem=%s\n",
           pod.name, pod.cpu_request, pod.memory_request)
```

## Context helpers

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.context()` | `string` | Current kubeconfig context name |
| `k8s.namespace_name()` | `string` | Default namespace for the current context |
| `k8s.version(timeout="")` | `AttrDict` | Kubernetes server version info (`.major`, `.minor`, `.git_version`, `.platform`) |
| `k8s.api_resources(timeout="")` | `list[AttrDict]` | Available API resources |

```python
print("context:", k8s.context())
print("default namespace:", k8s.namespace_name())
print("server version:", k8s.version().git_version)
```

## Controllers

`k8s.control()` runs an active reconciliation loop over a resource kind. It is the Starkite equivalent of writing a controller in `controller-runtime` — you supply handlers, while the runtime manages the informers, workqueue, self-echo loop suppression, status conditions, event emission, child resource lifecycle, health endpoints, and distributed leader election.

```python
k8s.control(
    kind,
    reconcile = None,
    finalize = None,
    on_create = None,
    on_update = None,
    on_delete = None,
    namespace = "",
    labels = "",
    field_selector = "",
    resync = "",
    poll = "",
    finalizer = "",
    health_port = 0,
    workers = 1,
    max_retries = 5,
    backoff = "5s",
    generation_changed = True,
    watch_owned = [],
    watch_related = [],
    predicate = None,
    leader_election = False,
    leader_election_id = "",
    leader_election_namespace = "",
    identity = "",
)
```

| Kwarg | Type | Default | Description |
|-------|------|---------|-------------|
| `kind` | string | **required** (positional) | Resource kind to watch |
| `reconcile` | callable | — | Primary reconcile handler: `fn(obj)` or `fn(event, obj)`. Can return `None`, a requeue duration string (e.g., `"10s"`), or a `list[KubeResource]` of desired child resources |
| `finalize` | callable | — | Declarative teardown handler: `fn(obj)`. Executed when `metadata.deletionTimestamp` is set before the finalizer is stripped |
| `finalizer` | string | auto | Custom finalizer string. Injected on creation and removed after `finalize()` completes cleanly |
| `on_create` / `on_update` / `on_delete` | callable | — | Granular per-event handlers (optional alternative to `reconcile`) |
| `namespace` | string | default | Scope the controller to a namespace (or cluster-wide for cluster-scoped resources) |
| `labels` | string | — | Label selector (e.g., `"app=web"`) |
| `field_selector` | string | — | Field selector (e.g., `"metadata.name=my-resource"`) |
| `resync` | duration string | — | Informer resync interval (e.g., `"10m"`) |
| `poll` | duration string | — | Periodic background reconciliation interval (e.g., `"30s"`) |
| `health_port` | int | `0` | Port for embedded HTTP health server exposing `/healthz` and `/readyz` |
| `generation_changed` | bool | `True` | Filter out metadata/status-only updates that did not change `.metadata.generation` |
| `workers` | int | `1` | Number of concurrent workqueue workers |
| `max_retries` | int | `5` | Retry cap per item on error before dropping |
| `backoff` | duration string | `"5s"` | Base retry backoff |
| `watch_owned` | list[string] | — | Owned resource kinds to watch whose `ownerReferences` point to the primary kind |
| `watch_related` | list | — | Secondary resource mappings: `[{"kind": "secrets", "map_func": fn(sec) -> ["ns/name"]}]` or `[("secrets", fn)]` |
| `predicate` | callable | — | `fn(obj) -> bool` filter applied before enqueueing |
| `leader_election` | bool | `False` | Run under distributed `coordination.k8s.io/v1` `Lease` locking |
| `leader_election_id` | string | `<kind>-controller` | Name of the `Lease` resource |
| `leader_election_namespace` | string | controller ns | Namespace for the `Lease` resource |
| `identity` | string | `<host>_<pid>` | Replica identity string for the lease lock candidate |

Blocks until interrupted (SIGINT/SIGTERM).

### Substrate Automation Features

When using `reconcile = reconcile_fn`:
* **Functional Child Returns**: When `reconcile(obj)` returns a list of child resources (e.g., `return [child_dep, child_svc]`), the runtime automatically:
  1. Injects `ownerReferences` pointing to the parent resource.
  2. Spawns informers (auto-watch) on child kinds so child modifications trigger parent re-reconciliation.
  3. Applies child resources via Server-Side Apply (`fieldManager="starkite"`).
  4. Prunes orphaned child resources when removed from the returned list.
* **Status Conditions & Events**: The runtime automatically updates `.status.conditions` (`Type="Ready", Status="True", Reason="Reconciled"`) and emits a `Normal Reconciled` Kubernetes event upon successful reconciliation.
* **Loop Immunity**: Built-in self-echo suppression prevents updates or patches made by the controller from triggering infinite reconciliation loops.
* **Dynamic Readiness Probes**: Under leader election, `/readyz` responds with `200 OK` on the active leader and `503 Service Unavailable` on standby replicas until failover occurs.

### Example — Operator with Child Reconciliation and Finalizer

```python
def reconcile(site):
    name = site.metadata.name
    ns = site.metadata.namespace
    replicas = site.spec.get("replicas", 1)

    # Return desired child workloads; substrate manages ownership, SSA apply, and pruning
    child_dep = k8s.obj.deployment(
        name = name,
        namespace = ns,
        replicas = replicas,
        containers = [k8s.obj.container(name="web", image="nginx:alpine")],
    )
    return [child_dep]

def finalize(site):
    print("Executing teardown for %s" % site.metadata.name)
    return None

k8s.control(
    "staticsites",
    reconcile = reconcile,
    finalize = finalize,
    finalizer = "tutorial.starkite.io/finalizer",
    health_port = 8081,
    poll = "30s",
    workers = 2,
)
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

See the [webhooks guide](../../infra/k8s-webhooks.md) for a full end-to-end workflow including `gen-webhook-artifacts`.

## Object constructors

The `k8s.obj` namespace provides declarative constructors for building validated Kubernetes resource manifests and sub-objects programmatically.

### Resource constructors

| Constructor | Returns | Description |
|-------------|---------|-------------|
| `k8s.obj.deployment(name, replicas=1, containers=[], resource_claims=[], labels={}, annotations={}, selector={}, template=None)` | `KubeResource` | Construct a Deployment manifest |
| `k8s.obj.service(name, ports=[], selector={}, type="ClusterIP", labels={}, annotations={})` | `KubeResource` | Construct a Service manifest |
| `k8s.obj.config_map(name, data={}, binary_data={}, labels={}, annotations={})` | `KubeResource` | Construct a ConfigMap manifest |
| `k8s.obj.secret(name, data={}, string_data={}, type="Opaque", labels={}, annotations={})` | `KubeResource` | Construct a Secret manifest |
| `k8s.obj.pod(name, containers=[], resource_claims=[], restart_policy="Always", labels={}, annotations={})` | `KubeResource` | Construct a Pod manifest |
| `k8s.obj.job(name, containers=[], resource_claims=[], completions=1, parallelism=1, labels={}, annotations={})` | `KubeResource` | Construct a Job manifest |
| `k8s.obj.cron_job(name, schedule, job_template=None, containers=[], resource_claims=[], labels={}, annotations={})` | `KubeResource` | Construct a CronJob manifest |
| `k8s.obj.stateful_set(name, service_name="", replicas=1, containers=[], resource_claims=[], labels={}, annotations={})` | `KubeResource` | Construct a StatefulSet manifest |
| `k8s.obj.daemon_set(name, containers=[], resource_claims=[], labels={}, annotations={})` | `KubeResource` | Construct a DaemonSet manifest |
| `k8s.obj.ingress(name, rules=[], tls=[], ingress_class_name="", labels={}, annotations={})` | `KubeResource` | Construct an Ingress manifest |
| `k8s.obj.persistent_volume(name, capacity={}, storage="", access_modes=[], storage_class_name="", reclaim_policy="", host_path={}, nfs={}, csi={}, labels={}, annotations={})` | `KubeResource` | Construct a PersistentVolume manifest (cluster-scoped) |
| `k8s.obj.persistent_volume_claim(name, access_modes=[], storage="", storage_class_name="", volume_name="", selector={}, data_source={}, data_source_ref={}, labels={}, annotations={})` | `KubeResource` | Construct a PersistentVolumeClaim manifest |
| `k8s.obj.storage_class(name, provisioner="", volume_binding_mode="", reclaim_policy="", allow_volume_expansion=False, parameters={}, mount_options=[], labels={}, annotations={})` | `KubeResource` | Construct a StorageClass manifest (cluster-scoped) |
| `k8s.obj.namespace(name, labels={}, annotations={})` | `KubeResource` | Construct a Namespace manifest |
| `k8s.obj.service_account(name, labels={}, annotations={})` | `KubeResource` | Construct a ServiceAccount manifest |
| `k8s.obj.device_class(name, selectors=[], config=[], suitable_nodes={}, labels={}, annotations={})` | `KubeResource` | Construct a `resource.k8s.io/v1` `DeviceClass` (cluster-scoped) |
| `k8s.obj.resource_claim(name, device_class="", count=1, allocation_mode="", device_tolerations=[], selectors=[], devices={}, labels={}, annotations={})` | `KubeResource` | Construct a `resource.k8s.io/v1` `ResourceClaim` |
| `k8s.obj.resource_claim_template(name, spec=None, claim_metadata={}, labels={}, annotations={})` | `KubeResource` | Construct a `resource.k8s.io/v1` `ResourceClaimTemplate` |
| `k8s.obj.resource_slice(name, node_name="", driver="", pool={}, devices={}, labels={}, annotations={})` | `KubeResource` | Construct a `resource.k8s.io/v1` `ResourceSlice` |
| `k8s.obj.crd(group, version, kind, plural, scope="Namespaced", spec={}, status={})` | `CRDResource` | Construct a CustomResourceDefinition manifest |

### Sub-object constructors

| Constructor | Returns | Description |
|-------------|---------|-------------|
| `k8s.obj.container(name, image, ports=[], env=[], command=[], args=[], volume_mounts=[], resources=None, claims=[], liveness_probe=None, readiness_probe=None)` | `KubeResource` | Container specification |
| `k8s.obj.container_port(container_port, name="", protocol="TCP", host_port=0)` | `KubeResource` | Container port definition |
| `k8s.obj.service_port(port, target_port=0, name="", protocol="TCP", node_port=0)` | `KubeResource` | Service port definition |
| `k8s.obj.env_var(name, value="", value_from={})` | `KubeResource` | Environment variable definition |
| `k8s.obj.env_from(config_map_ref={}, secret_ref={}, prefix="")` | `KubeResource` | Environment variable source from ConfigMap or Secret |
| `k8s.obj.resource_requirements(requests={}, limits={}, claims=[])` | `KubeResource` | Resource requests, limits, and claim bindings |
| `k8s.obj.probe(http_get={}, tcp_socket={}, exec={}, initial_delay_seconds=0, period_seconds=10, timeout_seconds=1, failure_threshold=3)` | `KubeResource` | Health probe configuration |
| `k8s.obj.volume(name, pvc=None, claim_name="", config_map=None, secret=None, empty_dir=None, ephemeral=None, host_path={}, nfs={}, csi={}, projected={}, downward_api={})` | `KubeResource` | Pod volume definition with ergonomic shortcuts for PVCs, ConfigMaps, Secrets, EmptyDir, and Ephemeral volumes |
| `k8s.obj.volume_mount(name, mount_path, sub_path="", sub_path_expr="", read_only=False, mount_propagation="", recursive_read_only="")` | `KubeResource` | Container volume mount specification |

### Example — construct and apply workload

```python
# Construct Deployment and Service with typed constructors
dep = k8s.obj.deployment(
    name = "web",
    replicas = 3,
    labels = {"app": "web", "tier": "frontend"},
    containers = [
        k8s.obj.container(
            name = "nginx",
            image = "nginx:1.27",
            ports = [k8s.obj.container_port(container_port=80, name="http")],
            env = [
                k8s.obj.env_var(name="PORT", value="80"),
                k8s.obj.env_var(name="ENV", value="production"),
            ],
            readiness_probe = k8s.obj.probe(http_get={"path": "/", "port": 80}, initial_delay_seconds=5),
        ),
    ],
)

svc = k8s.obj.service(
    name = "web",
    labels = {"app": "web"},
    selector = {"app": "web"},
    ports = [
        k8s.obj.service_port(port=80, target_port=80, name="http"),
    ],
)

# Apply directly using Server-Side Apply
k8s.apply([dep, svc], namespace="default")
```

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

### Example — Dynamic Resource Allocation (DRA)

Dynamic Resource Allocation (`resource.k8s.io/v1`) allocates accelerators, GPUs, and hardware devices dynamically via scheduler-evaluated claims:

```python
# 1. Define cluster-level device class
gpu_class = k8s.obj.device_class(
    name = "gpu.nvidia.com",
    selectors = [
        {"cel": {"expression": "device.capacity.memory >= 40Gi"}}
    ],
)
k8s.apply(gpu_class)

# 2. Define resource claim requesting hardware allocation
claim = k8s.obj.resource_claim(
    name = "ml-gpu-claim",
    device_class = "gpu.nvidia.com",
    count = 1,
    device_tolerations = [
        {"key": "gpu.nvidia.com/mig", "operator": "Exists"}
    ],
)
k8s.apply(claim)

# 3. Attach claim to deployment and container
workload = k8s.obj.deployment(
    name = "llm-inference",
    replicas = 2,
    resource_claims = [{"name": "gpu", "claim_name": "ml-gpu-claim"}],
    containers = [
        k8s.obj.container(
            name = "engine",
            image = "vllm/vllm-openai:latest",
            claims = [{"name": "gpu"}],
        )
    ],
)
k8s.apply(workload)

# 4. Inspect claim allocation status
claims = k8s.claims(namespace="default")
for c in claims:
    print(c.metadata.name, c.status.get("allocation"))
```

### Example — Persistent Volumes and Workload Storage

Starkite supports typed constructors and ergonomic workload binding for Kubernetes storage primitives:

```python
# 1. Define cluster-level StorageClass
sc = k8s.obj.storage_class(
    name = "fast-ssd",
    provisioner = "kubernetes.io/no-provisioner",
    volume_binding_mode = "WaitForFirstConsumer",
    allow_volume_expansion = True,
)
k8s.apply(sc)

# 2. Define PersistentVolumeClaim
pvc = k8s.obj.persistent_volume_claim(
    name = "db-data",
    storage = "20Gi",
    storage_class_name = "fast-ssd",
    access_modes = ["ReadWriteOnce"],
)
k8s.apply(pvc, namespace="default")

# 3. Mount volume in a Deployment using object shortcut
workload = k8s.obj.deployment(
    name = "postgres",
    replicas = 1,
    volumes = [
        k8s.obj.volume(name="data", pvc=pvc),  # direct KubeResource reference
        k8s.obj.volume(name="scratch", empty_dir=True),  # bool shortcut
    ],
    containers = [
        k8s.obj.container(
            name = "db",
            image = "postgres:16",
            volume_mounts = [
                k8s.obj.volume_mount(name="data", mount_path="/var/lib/postgresql/data"),
                k8s.obj.volume_mount(name="scratch", mount_path="/tmp"),
            ],
        ),
    ],
)
k8s.apply(workload, namespace="default")

# 4. Inspect storage resources
pvcs = k8s.pvcs(namespace="default")
for p in pvcs:
    print(p.metadata.name, p.status.phase, p.spec.resources.requests.storage)
```

### Utilities

| Function | Returns | Description |
|----------|---------|-------------|
| `k8s.yaml(manifest)` | `string` | Render a manifest dict or KubeResource as YAML |
| `k8s.config(kubeconfig="", context="", namespace="")` | `Client` | Return a configured Kubernetes client bound to the given kubeconfig path, context, and namespace |

## Examples

### Get and list resources

```python
# Get a specific pod
pod = k8s.get("pod", "web-abc123", namespace="default")
print(pod.status.phase)

# List pods by label
pods = k8s.list("pod", namespace="default", labels="app=web")
for p in pods:
    print(p.metadata.name, p.status.phase)
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
result = k8s.deploy("web", "nginx:latest", replicas=3, port=80, namespace="default",
    labels={"app": "web", "tier": "frontend"},
    env={"ENV": "production"},
)
printf("Deployed: %s, Service: %s\n", result.deployment, result.service)

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
printf("Complete: %s, Ready: %d/%d\n", status.complete, status.ready, status.replicas)

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
print("Kubernetes:", ver.git_version)

resources = k8s.api_resources()
for r in resources:
    print(r.name, r.kind)
```

> **Note:**
All `k8s` functions that can fail support `try_` variants that return a `Result` instead of raising an error.
