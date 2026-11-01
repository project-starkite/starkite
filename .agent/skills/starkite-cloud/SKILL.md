---
name: starkite-cloud
description: >-
  Automate cloud operations, infrastructure lifecycles, and data workflows
  using Starkite. Use when authoring or debugging Starkite automation scripts
  targeting cloud modules (http, k8s, concur, sql, ssh, containers).
---

# Starkite Cloud Operations Skill

This skill teaches autonomous agents how to author idiomatic, sandboxed, and robust Starkite automation scripts.

## Tier 1: Core Language Invariants & Execution Rules

Autonomous agents generating Starkite scripts MUST adhere to four load-bearing execution rules:

1. **Automatic Entrypoint Execution (`main()`)**:
   - Starkite scripts define entry logic inside `def main():` or `def main(ctx):`.
   - **NEVER** call `main()` at the top-level. The runtime automatically discovers and invokes `main()`.
   - Calling `main()` manually causes duplicate execution and corrupts exit status codes.

2. **Error Handling via `try_` Result Pattern**:
   - Built-in functions have safe `try_` variants (e.g. `fs.try_read_text`, `os.try_exec`, `http.try_get`).
   - Safe functions return a `Result` struct with three fields: `res.ok` (bool), `res.value` (any), and `res.err` (string).
   - **ALWAYS** check `res.ok` before reading `res.value`. If `res.ok` is False, handle `res.err` gracefully.
   - Direct functions (e.g. `fs.read_text`) raise uncatchable runtime failures on error.

3. **Kernel Sandbox Isolation Constraints**:
   - Scripts execute within OS-level sandboxes (Landlock LSM on Linux, Seatbelt on macOS, AppContainers on Windows).
   - By default, file writes and network access outside the script directory are restricted.
   - Always use `fs.path` for path normalization across operating systems (automatically converts `/` and `\`).
   - Scripts requiring network or write access must use appropriate CLI flags (`--sandbox-net-allow`, `--sandbox-path-write`).

4. **Auto-Loaded Standard Library Modules**:
   - Standard modules (`fs`, `http`, `os`, `json`, `yaml`, `concur`, `time`, `log`, `csv`, `zip`, `gzip`, `semver`, `crypto`, `math`, `strings`) are predeclared globals.
   - **DO NOT** write `import fs` or `load("fs", ...)` for built-in modules.
   - Use `load("//path/to/script.star", "symbol")` strictly for relative or installed user modules.

## Tier 2: Scripting Idioms & Production Recipes

### 4. Concurrent Fan-Out & Rate-Limited Pipelines
```python
def check_endpoint(target):
    res = http.try_get("https://" + target + "/healthz")
    return {"target": target, "alive": res.ok and res.value.status_code == 200}

endpoints = ["node1.internal", "node2.internal", "node3.internal"]
# Concurrently check endpoints with worker pool limit of 2
results = concur.each(endpoints, check_endpoint, limit=2)
for r in results:
    log.info("Endpoint %s alive=%s", r["target"], r["alive"])
```

### 5. Kubernetes Cluster Triage & Resource Inspection
```python
client = k8s.client()
pods_res = client.try_list("pods", namespace="default")
if pods_res.ok:
    for pod in pods_res.value:
        phase = pod.status.phase
        if phase != "Running":
            log.warn("Pod %s is in state: %s", pod.metadata.name, phase)
else:
    log.error("Failed to query cluster: %s", pods_res.err)
```

## Tier 3: Complete Module & Member Reference Tables

Reference documentation for all functions, methods, and return types within this skill's scope.

### Module `concur`

concur provides concurrent execution: map, each, exec

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `each` | `concur.each(items, func, workers=0, timeout="", on_error="abort")` | `None` | Apply `func` to each item concurrently (no return values) | `concur.try_each` |
| `exec` | `concur.exec(*fns, timeout="", on_error="abort")` | `tuple` | Execute multiple functions concurrently, returning their results | `concur.try_exec` |
| `map` | `concur.map(items, func, workers=0, timeout="", on_error="abort")` | `list` | Apply `func` to each item concurrently, returning results in order | `concur.try_map` |

### Module `containers`

containers provides container engine management: config() returns a client for daemon operations

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `config` | `containers.config()` | `None` | - | `containers.try_config` |
| `delete` | `containers.delete()` | `None` | - | `containers.try_delete` |
| `exec` | `containers.exec()` | `None` | - | `containers.try_exec` |
| `image_build` | `containers.image_build()` | `None` | - | `containers.try_image_build` |
| `image_inspect` | `containers.image_inspect()` | `None` | - | `containers.try_image_inspect` |
| `image_list` | `containers.image_list()` | `None` | - | `containers.try_image_list` |
| `image_pull` | `containers.image_pull()` | `None` | - | `containers.try_image_pull` |
| `image_remove` | `containers.image_remove()` | `None` | - | `containers.try_image_remove` |
| `inspect` | `containers.inspect()` | `None` | - | `containers.try_inspect` |
| `list` | `containers.list()` | `None` | - | `containers.try_list` |
| `remove` | `containers.remove()` | `None` | - | `containers.try_remove` |
| `run` | `containers.run()` | `None` | - | `containers.try_run` |
| `stop` | `containers.stop()` | `None` | - | `containers.try_stop` |

### Module `http`

http provides HTTP client and server: get, post, put, patch, delete, url, config, server, serve

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `config` | `http.config(timeout="30s", headers={})` | `None` | Set default client configuration | `http.try_config` |
| `delete` | `http.delete(url, headers={}, timeout="")` | `http.response` | Send DELETE request directly to a URL | `http.try_delete` |
| `get` | `http.get(url, headers={}, timeout="")` | `http.response` | Send GET request directly to a URL | `http.try_get` |
| `patch` | `http.patch(url, body=None, headers={}, timeout="")` | `http.response` | Send PATCH request directly to a URL | `http.try_patch` |
| `post` | `http.post(url, body=None, headers={}, timeout="")` | `http.response` | Send POST request directly to a URL | `http.try_post` |
| `put` | `http.put(url, body=None, headers={}, timeout="")` | `http.response` | Send PUT request directly to a URL | `http.try_put` |
| `serve` | `http.serve(handler_or_routes, port=0, host="", tls_cert="", tls_key="")` | `None` | Quick-start a server with a handler or route dict | `http.try_serve` |
| `server` | `http.server(port=0, host="", tls_cert="", tls_key="", read_timeout="", write_timeout="", idle_timeout="", shutdown_timeout="", max_header_bytes=0, max_body_bytes=0)` | `http.server` | Create an HTTP server. Timeout kwargs take duration strings (e.g. `"30s"`); `max_*_bytes` take integer byte sizes | `http.try_server` |
| `url` | `http.url(url_string)` | `http.url` | Create a URL object for making requests | `http.try_url` |

#### Exported Type `http.server` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `handle` | `srv.handle(pattern, handler)` | `None` | Register a route handler |
| `port` | `srv.port()` | `int` | Get the port the server is listening on |
| `serve` | `srv.serve(port=0, host="", tls_cert="", tls_key="")` | `None` | Start server and block |
| `shutdown` | `srv.shutdown()` | `None` | Gracefully shut down the server |
| `start` | `srv.start(port=0, host="", tls_cert="", tls_key="")` | `None` | Start server in background |
| `try_handle` | `srv.try_handle(pattern, handler)` | `Result` | Register a route handler Returns a Result instead of raising. |
| `try_port` | `srv.try_port()` | `Result` | Get the port the server is listening on Returns a Result instead of raising. |
| `try_serve` | `srv.try_serve(port=0, host="", tls_cert="", tls_key="")` | `Result` | Start server and block Returns a Result instead of raising. |
| `try_shutdown` | `srv.try_shutdown()` | `Result` | Gracefully shut down the server Returns a Result instead of raising. |
| `try_start` | `srv.try_start(port=0, host="", tls_cert="", tls_key="")` | `Result` | Start server in background Returns a Result instead of raising. |
| `try_use` | `srv.try_use(middleware)` | `Result` | Add middleware function Returns a Result instead of raising. |
| `use` | `srv.use(middleware)` | `None` | Add middleware function |

#### Exported Type `http.url` Methods

| Method | Signature | Returns | Description |
|---|---|---|---|
| `delete` | `url.delete(headers={}, timeout="")` | `http.response` | Send DELETE request |
| `get` | `url.get(headers={}, timeout="")` | `http.response` | Send GET request |
| `patch` | `url.patch(body, headers={}, timeout="")` | `http.response` | Send PATCH request |
| `post` | `url.post(body, headers={}, timeout="")` | `http.response` | Send POST request |
| `put` | `url.put(body, headers={}, timeout="")` | `http.response` | Send PUT request |
| `try_delete` | `url.try_delete(headers={}, timeout="")` | `Result` | Send DELETE request Returns a Result instead of raising. |
| `try_get` | `url.try_get(headers={}, timeout="")` | `Result` | Send GET request Returns a Result instead of raising. |
| `try_patch` | `url.try_patch(body, headers={}, timeout="")` | `Result` | Send PATCH request Returns a Result instead of raising. |
| `try_post` | `url.try_post(body, headers={}, timeout="")` | `Result` | Send POST request Returns a Result instead of raising. |
| `try_put` | `url.try_put(body, headers={}, timeout="")` | `Result` | Send PUT request Returns a Result instead of raising. |

### Module `k8s`

k8s provides 3-tier Kubernetes operations: CRUD (get, list, apply, delete, patch, label, annotate), watch (watch, wait_for), I/O (logs, exec, port_forward), high-level (deploy, scale, rollout, drain), and k8s.obj constructors

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `annotate` | `k8s.annotate(kind, name, annotations, namespace="", timeout="")` | `AttrDict` | Set annotations on a resource | `k8s.try_annotate` |
| `api_resources` | `k8s.api_resources(timeout="")` | `list[AttrDict]` | Available API resources | `k8s.try_api_resources` |
| `apply` | `k8s.apply(manifest, namespace="", field_manager="starkite", dry_run=False, force=False, prune=False, prune_labels={}, timeout="")` | `AttrDict` | Apply a resource (server-side apply). When `prune=True`, prunes omitted resources managed by `field_manager` matching `prune_labels` | `k8s.try_apply` |
| `autoscale` | `k8s.autoscale(kind, name, min=1, max=10, cpu_percent=80, namespace="", timeout="")` | `AttrDict` | Create a HorizontalPodAutoscaler | `k8s.try_autoscale` |
| `claims` | `k8s.claims(namespace="", labels="", timeout="")` | `list[AttrDict]` | List `resource.k8s.io/v1` `ResourceClaim` objects for Dynamic Resource Allocation (DRA) | `k8s.try_claims` |
| `condition` | `k8s.condition()` | `None` | - | - |
| `config` | `k8s.config(kubeconfig="", context="", namespace="")` | `Client` | Return a configured Kubernetes client bound to the given kubeconfig path, context, and namespace | `k8s.try_config` |
| `context` | `k8s.context()` | `string` | Current kubeconfig context name | `k8s.try_context` |
| `control` | `k8s.control()` | `None` | - | `k8s.try_control` |
| `cordon` | `k8s.cordon(node, timeout="")` | `AttrDict` | Mark a node unschedulable | `k8s.try_cordon` |
| `cp` | `k8s.cp(pod, src, dst, namespace="", container="", timeout="")` | `AttrDict` | Copy files to/from a pod. Use `pod:path` as `src` to download, `pod:path` as `dst` to upload | `k8s.try_cp` |
| `create` | `k8s.create(manifest, namespace="", dry_run=False, timeout="")` | `AttrDict` | Create a resource from a manifest (dict, AttrDict, or YAML string) | `k8s.try_create` |
| `debug` | `k8s.debug(name, image="nicolaka/netshoot", target_container="", command=None, namespace="", timeout="3m")` | `AttrDict` | Attach an ephemeral container to a running pod for zero-shell diagnostics. Returns `{"stdout", "stderr", "code", "container", "pod"}` | `k8s.try_debug` |
| `delete` | `k8s.delete(kind, name, namespace="", propagation="Background", timeout="")` | `None` | Delete a resource | `k8s.try_delete` |
| `deploy` | `k8s.deploy(name, image, replicas=1, port=0, namespace="", labels=None, env=None, timeout="")` | `AttrDict` | Create a Deployment (returns `{"deployment": str, "service": str}`) | `k8s.try_deploy` |
| `describe` | `k8s.describe(kind, name, namespace="", timeout="")` | `AttrDict` | Return detailed description of a resource, with `.resource`, `.conditions`, and `.events` | `k8s.try_describe` |
| `diff` | `k8s.diff(manifest, namespace="", field_manager="starkite", timeout="")` | `AttrDict` | Server-side apply dry-run diff against live cluster state (`has_drift`, `drifted_fields`, `conflicts`, `diff`, `live`, `applied`) | `k8s.try_diff` |
| `drain` | `k8s.drain(node, force=False, ignore_daemonsets=False, timeout="")` | `AttrDict` | Drain a node (evict pods). `force` continues past pods not backed by a controller; `ignore_daemonsets` leaves DaemonSet pods in place | `k8s.try_drain` |
| `event` | `k8s.event(obj, reason, message, type="Normal", namespace="", timeout="")` | `AttrDict` | Emit a Kubernetes event attached to `obj`. `type` can be `"Normal"` or `"Warning"` | `k8s.try_event` |
| `events` | `k8s.events(target=None, kind="", name="", namespace="", type="", reason="", since="", limit=0, timeout="")` | `list[AttrDict]` | Query diagnostic event streams with dual-API normalization (`events.k8s.io/v1` and `core/v1`). Filter by target object, kind, name, type (`"Warning"`/`"Normal"`), reason substring, or relative duration (`since="30m"`). Returns items sorted newest first with relative `age` | `k8s.try_events` |
| `evict` | `k8s.evict(name, namespace="", dry_run=False, timeout="")` | `AttrDict` | Evict a Pod via `/eviction` subresource, honoring `PodDisruptionBudgets` | `k8s.try_evict` |
| `exec` | `k8s.exec(name, command, namespace="", container="", timeout="")` | `AttrDict` | Run a command in a pod. `command` may be a string (executed via `/bin/sh -c`) or a list (argv). Returns `{"stdout", "stderr", "code"}` | `k8s.try_exec` |
| `expose` | `k8s.expose(kind, name, port, target_port=0, type="ClusterIP", namespace="", timeout="")` | `AttrDict` | Expose a resource as a Service | `k8s.try_expose` |
| `finalizer` | `finalizer` | `string` | Custom finalizer string. Injected on creation and removed after `finalize()` completes cleanly | `k8s.try_finalizer` |
| `get` | `k8s.get(kind, name, namespace="", timeout="")` | `AttrDict` | Get a single resource | `k8s.try_get` |
| `is_deleting` | `k8s.is_deleting(obj)` | `bool` | Check if `obj.metadata.deletionTimestamp` is set | `k8s.try_is_deleting` |
| `label` | `k8s.label(kind, name, labels, namespace="", timeout="")` | `AttrDict` | Set labels on a resource | `k8s.try_label` |
| `list` | `k8s.list(kind, namespace="", labels="", fields="", timeout="")` | `list[AttrDict]` | List resources with optional label/field selectors | `k8s.try_list` |
| `logs` | `k8s.logs(name, namespace="", container="", tail=0, since="", previous=False, timeout="")` | `string` | Fetch pod logs. `tail` caps line count; `since` is a duration string (e.g., `"10m"`); `previous=True` reads the previous container instance | `k8s.try_logs` |
| `logs_follow` | `k8s.logs_follow(name, handler, namespace="", container="", tail=0, timeout="")` | `None` | Stream logs, calling `handler(line)` per line. Blocks until the pod ends or `timeout` elapses | `k8s.try_logs_follow` |
| `namespace_name` | `k8s.namespace_name()` | `string` | Default namespace for the current context | `k8s.try_namespace_name` |
| `obj` | `k8s.obj()` | `None` | - | - |
| `patch` | `k8s.patch(kind, name, patch, namespace="", type="merge", timeout="")` | `AttrDict` | Patch a resource. `type`: `"merge"`, `"strategic"`, or `"json"` | `k8s.try_patch` |
| `port_forward` | `k8s.port_forward(name, port, local_port=0, namespace="")` | `PortForwardHandle` | Forward a local port to a pod port. Blocks until interrupted. `local_port=0` picks a free port | `k8s.try_port_forward` |
| `pvcs` | `k8s.pvcs(namespace="", labels="", timeout="")` | `list[AttrDict]` | List `PersistentVolumeClaim` objects | `k8s.try_pvcs` |
| `pvs` | `k8s.pvs(labels="", timeout="")` | `list[AttrDict]` | List cluster-scoped `PersistentVolume` objects | `k8s.try_pvs` |
| `resize` | `k8s.resize(name, container, cpu=None, memory=None, requests=None, limits=None, namespace="", timeout="30s")` | `AttrDict` | Resize CPU and memory allocations of a running container in-place without pod restart | `k8s.try_resize` |
| `rollout` | `k8s.rollout(kind, name, action="status", namespace="", timeout="")` | `AttrDict` | Manage rollouts. `action`: `"status"`, `"restart"`, `"pause"`, `"resume"` | `k8s.try_rollout` |
| `route` | `k8s.route(name, gateway, service, port, prefix="/", host="", namespace="", timeout="")` | `AttrDict` | Create or update an HTTPRoute binding a Gateway to a Service backend (returns `{"route": str}`) | `k8s.try_route` |
| `run` | `k8s.run(name, image, command=None, namespace="", restart="Never", rm=False, timeout="3m")` | `AttrDict` | Run a one-off Pod (like `kubectl run`) | `k8s.try_run` |
| `scale` | `k8s.scale(kind, name, replicas, namespace="", timeout="")` | `AttrDict` | Scale a resource to the given replica count | `k8s.try_scale` |
| `set_env` | `k8s.set_env(kind, name, env, namespace="", container="", timeout="")` | `AttrDict` | Set environment variables on a resource | `k8s.try_set_env` |
| `set_image` | `k8s.set_image(kind, name, container, image, namespace="", timeout="")` | `AttrDict` | Update the container image of a resource | `k8s.try_set_image` |
| `set_resources` | `k8s.set_resources(kind, name, requests=None, limits=None, namespace="", container="", timeout="")` | `AttrDict` | Set resource requests and limits | `k8s.try_set_resources` |
| `status` | `k8s.status(obj, status, namespace="", timeout="")` | `AttrDict` | Update the status subresource of a resource. Pass the resource as `obj` and the new status dict as `status` | `k8s.try_status` |
| `storage_classes` | `k8s.storage_classes(labels="", timeout="")` | `list[AttrDict]` | List cluster-scoped `StorageClass` objects | `k8s.try_storage_classes` |
| `taint` | `k8s.taint(node, key, value="", effect="", timeout="")` | `AttrDict` | Add a taint to a node. `effect`: `"NoSchedule"`, `"PreferNoSchedule"`, or `"NoExecute"` | `k8s.try_taint` |
| `top_nodes` | `k8s.top_nodes(timeout="")` | `list[AttrDict]` | CPU/memory capacity and allocatable per node | `k8s.try_top_nodes` |
| `top_pods` | `k8s.top_pods(namespace="", sort_by="", timeout="")` | `list[AttrDict]` | Resource requests and status per pod | `k8s.try_top_pods` |
| `uncordon` | `k8s.uncordon(node, timeout="")` | `AttrDict` | Re-enable scheduling on a node | `k8s.try_uncordon` |
| `untaint` | `k8s.untaint(node, key, timeout="")` | `AttrDict` | Remove a taint from a node by key | `k8s.try_untaint` |
| `validate` | `k8s.validate(manifest, policy=None, expression="", strict=False)` | `AttrDict` | Evaluate CEL validation policies or expressions against a manifest client-side prior to cluster submission. Returns `{"valid": bool, "violations": list[str], "policy": str}` | `k8s.try_validate` |
| `version` | `k8s.version(timeout="")` | `AttrDict` | Kubernetes server version info (`.major`, `.minor`, `.git_version`, `.platform`) | `k8s.try_version` |
| `wait_for` | `k8s.wait_for(kind, name, condition="", namespace="", timeout="")` | `AttrDict` | Block until the named resource meets the given condition (e.g., `"Available"`, `"Ready"`, `"Complete"`) or the timeout expires. Returns `{"ready": bool, "resource": AttrDict, "message": str}` | `k8s.try_wait_for` |
| `watch` | `k8s.watch(kind, namespace="", labels="", timeout="", handler=None)` | `list[AttrDict]` or `None` | Watch a resource kind. If `handler` is supplied, call it per event (`handler(event_type, obj)`) and return `None`; otherwise collect events and return a list of `{"type": ..., "object": ...}` AttrDicts. `timeout` caps wall-clock duration | `k8s.try_watch` |
| `webhook` | `k8s.webhook()` | `None` | - | `k8s.try_webhook` |
| `yaml` | `k8s.yaml(manifest)` | `string` | Render a manifest dict or KubeResource as YAML | `k8s.try_yaml` |

### Module `sql`

sql provides database connectivity: open() returns a connection for queries, execs, and transactions

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `open` | `sql.open()` | `None` | - | `sql.try_open` |
| `stmt` | `sql.stmt()` | `None` | - | `sql.try_stmt` |

### Module `ssh`

ssh provides SSH remote execution: config() returns a client for remote commands

| Member | Signature | Returns | Description | Safe Variant |
|---|---|---|---|---|
| `add_known_host` | `client.add_known_host(key, host=None, ...)` | `None` | Append host key entry for target host to known_hosts | `ssh.try_add_known_host` |
| `check_authorized_key` | `client.check_authorized_key(key, ...)` | `None` | Probe remote hosts for public key authorization without logging in (alias: `client.key_check`) | `ssh.try_check_authorized_key` |
| `config` | `ssh.config()` | `None` | - | `ssh.try_config` |
| `copy_id` | `client.copy_id(key, ...)` | `None` | Install public key into target hosts' `authorized_keys` | `ssh.try_copy_id` |
| `exec` | `client.exec(cmd, ...)` | `None` | Execute single command or pipeline across target hosts | `ssh.try_exec` |
| `find_known_hosts` | `client.find_known_hosts(host=None, ...)` | `None` | Query entries matching target hosts in known_hosts file | `ssh.try_find_known_hosts` |
| `key_check` | `ssh.key_check()` | `None` | - | `ssh.try_key_check` |
| `keygen` | `ssh.keygen()` | `None` | - | `ssh.try_keygen` |
| `keyscan` | `ssh.keyscan()` | `None` | - | `ssh.try_keyscan` |
| `remove_known_host` | `client.remove_known_host(host=None, ...)` | `None` | Evict stale host key entries matching target hosts | `ssh.try_remove_known_host` |
| `scan_host_keys` | `client.scan_host_keys(...)` | `None` | Discover and inspect public host keys across target hosts (alias: `client.keyscan`) | `ssh.try_scan_host_keys` |

