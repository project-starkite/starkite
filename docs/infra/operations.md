---
title: "Operating a cluster"
description: "Deploy, scale, retrieve, and pipe Kubernetes resources"
weight: 20
---

# Operating a cluster

Starkite's operational module gets container workloads running and inspected with the fewest moving parts. Each call you make corresponds to a `kubectl` verb you would otherwise type by hand, providing a direct, client-driven, or piped path for cluster operations.

---

## Deploying resources

You begin by reading the values that change between environments — namespace, image, name, replica count — so the same script deploys to staging or production with different flags rather than different code. From there you bind a client to a namespace and drive it through the deploy, scale, autoscale, and inspect sequence.

**Source:** [quick-deploy.star](https://github.com/project-starkite/starkite/blob/main/examples/cloud/quick-deploy/quick-deploy.star)

```python
#!/usr/bin/env kite

ns = var_str("namespace", "default")
image = var_str("image", "nginx:1.27")
name = var_str("app.name", "web")
replicas = var_int("replicas", 3)

k = k8s.config(namespace=ns)

# Deployment + ClusterIP Service in one call
result = k.deploy(name, image,
    replicas=replicas,
    port=80,
    labels={"team": "platform"})

printf("Deployment: %s\n", result["deployment"])
printf("Service:    %s (ClusterIP)\n", result["service"])

# Scale up
k.scale("deployment", name, replicas + 2)

# Attach HPA targeting 70% CPU
k.autoscale("deployment", name,
    min=replicas, max=(replicas + 2) * 2, cpu_percent=70)

# Inspect pod resource usage
for p in k.top_pods(sort_by="cpu", timeout="15s"):
    if name in p["name"]:
        printf("  %-40s %8s %12s %s\n",
            p["name"], p["cpu_request"], p["memory_request"], p["status"])
```

### Running the script

Run the script to deploy with defaults, or override any variable on the command line. The `--dry-run` flag walks the execution path without touching the cluster:

```bash
kite run ./quick-deploy.star
kite run ./quick-deploy.star --var image=myapp:v2 --var namespace=staging
kite run ./quick-deploy.star --dry-run
```

### Under the hood

Each call in the script maps to one cluster operation, trading fine-grained control for brevity:

- **`k8s.config(namespace=...)`** binds a client to a namespace, and every subsequent call on that client inherits it — so you set the target once instead of repeating it on each call.
- **`k.deploy(name, image, ...)`** creates a Deployment plus an optional Service in a single call, returning a dict of the created resource names.
- **`k.scale(kind, name, replicas)`** is the `kubectl scale` equivalent, setting the replica count directly.
- **`k.autoscale(...)`** creates a HorizontalPodAutoscaler in one call.
- **`k.top_pods()`** returns the metrics-server view of running pods.

Creating and mutating cluster resources requires the `allow-local` permission profile:

```bash
kite run ./quick-deploy.star --permissions=allow-local
```

---

## Retrieving objects

Most automation starts by reading what is already in the cluster — the deployment you are about to scale, the pods behind a service, or a job you are waiting on. The `k8s` module provides `get` to return a single named object, and `list` to retrieve a collection. Reading is covered by the `k8s.read` capability (granted by the `allow-local` profile).

### Get a single object

`k8s.get(kind, name, namespace="", timeout="")` fetches one resource by name as a dictionary matching the resource's JSON representation:

```python
dep = k8s.get("deployment", "web", namespace="default")
print(dep["spec"]["replicas"])
```

### List a collection

`k8s.list(kind, namespace="")` returns a list of dictionaries that you can iterate:

```python
pods = k8s.list("pods", namespace="default")
for pod in pods:
    print(pod["metadata"]["name"], pod["status"]["phase"])
```

### Filtering with selectors

Both `list` and `get` accept `labels` and `fields` selectors to filter server-side:

```python
# Label selector
web_pods = k8s.list("pods", namespace="default", labels="app=web")

# Field selector
running = k8s.list("pods", labels="app=web", fields="status.phase=Running")
```

### Waiting for conditions and watching changes

`wait_for` blocks until a resource reaches a specified condition or the timeout expires:

```python
k8s.wait_for("deployment", "web", condition="Available", timeout="2m")
```

For streaming changes, `watch` dispatches API events directly to a handler:

```python
def on_event(event_type, obj):
    printf("%s: %s\n", event_type, obj["metadata"]["name"])

k8s.watch("deployment", namespace="default", timeout="30s", handler=on_event)
```

---

## Object representation

Kubernetes objects cross the Starlark boundary as standard data structures: a resource manifest is a `dict`, and collections are lists of dicts.

### Reading fields

You read fields by indexing directly down through keys and arrays:

```python
dep = k8s.get("deployment", "web")
name     = dep["metadata"]["name"]
replicas = dep["spec"]["replicas"]
image    = dep["spec"]["template"]["spec"]["containers"][0]["image"]
```

### Building and applying objects

To construct resources, assemble a dictionary and pass it to `create` or `apply` (which performs a server-side apply):

```python
manifest = {
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {"name": "app-config"},
    "data": {"LOG_LEVEL": "info"},
}
k8s.apply(manifest, namespace="default")
```

You can also pass a raw YAML string directly, which is useful when reading manifests from disk:

```python
k8s.apply(path("deploy.yaml").read_text(), namespace="default")
```

### Typed constructors

For CustomResourceDefinitions (CRDs) or boilerplate-heavy manifests, the `k8s.obj` namespace provides typed constructors:

```python
crd = k8s.obj.crd(...)   # scaffolds a CustomResourceDefinition dict
```

---

## Piping to kubectl

Sometimes you want Starkite to build the manifest but leave cluster mutations to `kubectl`. The bridge between the two is standard output: your script writes a manifest to stdout, and you pipe it directly to `kubectl`.

### Emitting a manifest

Build the resource dictionary, encode it to YAML, and print it to stdout:

```python
# gen.star
manifest = {
    "apiVersion": "apps/v1",
    "kind": "Deployment",
    "metadata": {"name": "web"},
    "spec": {
        "replicas": var_int("replicas", 3),
        "selector": {"matchLabels": {"app": "web"}},
        "template": {
            "metadata": {"labels": {"app": "web"}},
            "spec": {"containers": [{"name": "web", "image": var_str("image", "nginx:latest")}]},
        },
    },
}
print(yaml.encode(manifest))
```

To apply or diff the output, pipe it to `kubectl -f -`:

```bash
kite run ./gen.star --var image=myapp:v2 | kubectl apply -f -
kite run ./gen.star --var image=myapp:v2 | kubectl diff -f -
```

### Handling multiple objects

For multiple resources, use `yaml.encode_all` to print a multi-document stream separated by `---`:

```python
print(yaml.encode_all([deployment, service, configmap]))
```

And pipe it to `kubectl` the same way:

```bash
kite run ./stack.star | kubectl apply -f -
```
