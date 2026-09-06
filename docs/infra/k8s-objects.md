---
title: "Object representation"
description: "Understand how Starkite represents Kubernetes objects, reads fields, and handles mutations"
weight: 15
---

# Object representation

Starkite separates Kubernetes object workflows into two complementary phases:

1. **Object Construction (Authoring)**: Synthesizing manifests before submission using the `k8s.obj` constructor namespace with schema validation and boilerplate reduction.
2. **Object Representation & Traversal (Runtime)**: Inspecting, querying, and mutating live cluster resources using `AttrDict` with recursive dot notation and Starlark dictionary interfaces.

---

## Object Construction

When authoring new Kubernetes resources in Starlark, the `k8s.obj` namespace provides declarative constructors for standard workloads, configurations, networking, and pod sub-objects.

### Why Use `k8s.obj`
* **Schema Validation at Construction Time**: Validates parameter types, enforces required arguments (e.g., `ports` in `k8s.obj.service()`), and rejects unrecognized fields to prevent typos before submitting to the API server.
* **Boilerplate Reduction**: Automatically sets `apiVersion` and `kind`, and derives `spec.selector.matchLabels` and `spec.template.metadata.labels` from top-level labels.
* **Structured Sub-Objects**: Provides typed constructors for nested structures such as `k8s.obj.container()`, `k8s.obj.volume()`, `k8s.obj.probe()`, `k8s.obj.service_port()`, and `k8s.obj.env_var()`.

### Example: Building Workloads

```python
# Construct a Deployment with nested container and probe sub-objects
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
            readiness_probe = k8s.obj.probe(http_get={"path": "/healthz", "port": 80}, initial_delay_seconds=5),
        ),
    ],
)

# Construct a matching Service
svc = k8s.obj.service(
    name = "web",
    labels = {"app": "web"},
    selector = {"app": "web"},
    ports = [
        k8s.obj.service_port(port=80, target_port=80, name="http"),
    ],
)

# Apply both objects using Server-Side Apply
k8s.apply([dep, svc], namespace="production")
```

Constructors return a `KubeResource` value that can be passed directly to `k8s.apply()`, encoded to YAML via `k8s.yaml()`, or converted to a standard dictionary with `.to_dict()`.

### Example: Dynamic Resource Allocation (DRA)

Dynamic Resource Allocation (`resource.k8s.io/v1`) allocates accelerators, GPUs, and specialized hardware devices via scheduler-evaluated claims:

```python
# 1. Define cluster-level device class
gpu_class = k8s.obj.device_class(
    name = "gpu.nvidia.com",
    selectors = [
        {"cel": {"expression": "device.capacity.memory >= 40Gi"}},
    ],
)
k8s.apply(gpu_class)

# 2. Define resource claim requesting hardware allocation
claim = k8s.obj.resource_claim(
    name = "ml-gpu-claim",
    device_class = "gpu.nvidia.com",
    count = 1,
    device_tolerations = [
        {"key": "gpu.nvidia.com/mig", "operator": "Exists"},
    ],
)
k8s.apply(claim, namespace="production")

# 3. Bind claim to workload and container
workload = k8s.obj.deployment(
    name = "llm-inference",
    replicas = 2,
    resource_claims = [{"name": "gpu", "claim_name": "ml-gpu-claim"}],
    containers = [
        k8s.obj.container(
            name = "engine",
            image = "vllm/vllm-openai:latest",
            claims = [{"name": "gpu"}],
        ),
    ],
)
k8s.apply(workload, namespace="production")
```

### Example: Storage and Volume Management

Starkite provides typed constructors for storage primitives and ergonomic volume bindings:

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
    name = "app-storage",
    storage = "50Gi",
    storage_class_name = "fast-ssd",
    access_modes = ["ReadWriteOnce"],
)
k8s.apply(pvc, namespace="production")

# 3. Bind volumes to pod with object shortcuts
pod = k8s.obj.pod(
    name = "data-processor",
    volumes = [
        k8s.obj.volume(name="data", pvc=pvc),             # KubeResource or string PVC name
        k8s.obj.volume(name="scratch", empty_dir=True),    # boolean emptyDir shortcut
        k8s.obj.volume(name="config", config_map="app-cm"),# string ConfigMap name shortcut
    ],
    containers = [
        k8s.obj.container(
            name = "worker",
            image = "alpine:latest",
            command = ["sleep", "3600"],
            volume_mounts = [
                k8s.obj.volume_mount(name="data", mount_path="/var/data"),
                k8s.obj.volume_mount(name="scratch", mount_path="/tmp"),
                k8s.obj.volume_mount(name="config", mount_path="/etc/app", read_only=True),
            ],
        ),
    ],
)
k8s.apply(pod, namespace="production")
```

### Example: Hardened Pod Isolation and Container Resize Policies

Starkite provides declarative configuration for user namespaces (rootless isolation) and in-place container resize policies:

```python
# Construct a hardened pod with rootless User Namespaces and resize policy
pod = k8s.obj.pod(
    name = "hardened-worker",
    host_users = False,  # Maps container root UID 0 to an unprivileged host UID (GA 1.36)
    security_context = k8s.obj.security_context(
        run_as_non_root = True,
        apparmor_profile = {"type": "RuntimeDefault"},
        seccomp_profile = {"type": "RuntimeDefault"},
    ),
    containers = [
        k8s.obj.container(
            name = "worker",
            image = "alpine:latest",
            command = ["sleep", "3600"],
            resize_policy = [
                {"resource_name": "cpu", "restart_policy": "NotRequired"},
                {"resource_name": "memory", "restart_policy": "NotRequired"},
            ],
        ),
    ],
)
k8s.apply(pod, namespace="production")
```

---

## Runtime Representation & Dot Notation

Once a resource is created in the cluster or retrieved from the Kubernetes API, the API server populates runtime metadata (such as `.metadata.uid`, `.metadata.generation`, `.metadata.creationTimestamp`) and lifecycle status (`.status.phase`, `.status.conditions`).

Starkite returns all live cluster query results (`k8s.get()`, `k8s.list()`, `k8s.watch()`, `k8s.control()`, `k8s.deploy()`, `k8s.wait_for()`) as `AttrDict` instances.

### Dot-Notation Traversal

`AttrDict` enables direct attribute navigation across arbitrary nesting levels:

```python
def inspect_live_deployment():
    # Query live state from the cluster
    dep = k8s.get("deployment", "web", namespace="production")

    # Traverse nested fields via dot notation
    name = dep.metadata.name
    generation = dep.metadata.generation
    desired_replicas = dep.spec.replicas
    available_replicas = dep.status.get("availableReplicas", 0)

    # Access nested list items and sub-fields
    image = dep.spec.template.spec.containers[0].image

    print("Deployment:", name, "Replicas:", available_replicas, "/", desired_replicas)
    print("Container Image:", image)
```

### Key Behaviors of Dot Notation
* **Recursive Wrapping**: Accessing a child dictionary (e.g., `dep.spec.template`) automatically preserves dot notation throughout the entire object graph.
* **Null Safety**: Missing fields return `None` or allow fallback via `.get(key, default)` without raising `KeyError` exceptions.
* **Read-Only Traversal**: Field assignment via dot notation (`dep.spec.replicas = 5`) is rejected at runtime; modifications require dictionary bracket indexing.

---

## Dictionary Operations

`AttrDict` provides full compatibility with standard dictionary indexing and helper methods:

```python
def check_labels(pod):
    # Bracket indexing
    kind = pod["kind"]
    pod_name = pod["metadata"]["name"]

    # Safe lookup with defaults
    labels = pod.metadata.get("labels", {})
    tier = labels.get("tier", "standard")
```

---

## Dictionary Iteration

`AttrDict` supports standard dictionary iteration and inspection methods:

```python
def inspect_resource(obj):
    # Count fields
    print("Field count:", len(obj))
    print("Top-level keys:", list(obj.keys()))

    # Iterate keys
    for key in obj:
        print("Key:", key)

    # Iterate key-value pairs
    labels = obj.metadata.get("labels", {})
    for key, value in labels.items():
        print("  Label:", key, "=", value)
```

---

## In-Place Mutation

When modifying objects within mutating admission webhooks or controller reconcile loops, update fields in-place using bracket indexing (`[]`):

```python
def mutate_admission_payload(obj):
    # Read using dot notation
    print("Inspecting incoming deployment:", obj.metadata.name)

    # Mutate in-place using bracket indexing
    if not obj.metadata.get("labels"):
        obj["metadata"]["labels"] = {}

    obj["metadata"]["labels"]["managed-by"] = "starkite"
    obj["spec"]["replicas"] = 2

    # Modified data propagates to the parent object for RFC 6902 patch calculation
    return obj
```

---

## Summary of Operations

| Operation | Syntax | Returns / Behavior | Common Use Cases |
|:---|:---|:---|:---|
| **Authoring** | `k8s.obj.deployment(name="app", ...)` | `KubeResource` | Building declarative manifests with schema checking |
| **Dot Traversal** | `pod.metadata.name`, `pod.status.phase` | Value or `None` | Inspection, controllers, validating webhooks |
| **Bracket Index** | `pod["metadata"]["name"]` | Value | Standard dictionary compatibility |
| **Safe Fetch** | `pod.get("spec", {})` | Value or default | Reading optional or conditional fields |
| **Item Iteration** | `for k, v in pod.metadata.labels.items():` | Key-value tuples | Label inspection, auditing, filtering |
| **In-Place Mutation** | `pod["metadata"]["labels"]["env"] = "prod"` | Mutates in-place | Mutating webhooks, controller drift corrections |
| **Serialization** | `json.encode(pod)`, `k8s.apply(pod)` | Encoded string / API call | Direct API apply, manifest export |
| **Native Dict** | `pod.to_dict()` | Standard Starlark `dict` | Conversion to plain dictionary |
