---
title: "Declarative manifests"
description: "Construct, apply, and generate Kubernetes YAML configs using Starlark"
weight: 16
---

# Declarative manifests

For declarative configuration management, Starkite allows you to construct Kubernetes manifests dynamically as Starlark dictionaries or read them from external files, and apply them directly using Server-Side Apply.

## Building and Applying Manifests

Starkite translates Starlark dictionaries into JSON payloads to interact with the Kubernetes API. You can construct standard manifests and apply them using `k8s.apply()` (which performs a Server-Side Apply):

```python
def apply_application_config():
    # Define the ConfigMap manifest as a dictionary
    manifest = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": "alice-app-config"},
        "data": {
            "LOG_LEVEL": "info",
            "DB_HOST": "db.alice.local",
        },
    }
    
    # Apply the configuration
    k8s.apply(manifest, namespace="staging")
    print("Configuration manifest applied.")
```

## Applying External Manifest Files

You can read raw YAML or JSON manifests from disk using the `fs` module and apply them directly to the cluster:

```python
def deploy_manifest_file():
    # Read the manifest file from the local filesystem
    file_path = "manifests/service.yaml"
    manifest_yaml = fs.path(file_path).read_text()
    
    # Apply the manifest to the cluster
    k8s.apply(manifest_yaml, namespace="staging")
    print("Manifest from file applied.")
```

---

## Generating and Piping Manifests

When you want to validate manifests locally, run dry-runs, or delegate cluster mutations to external tools, Starkite can act as a template engine. Your script writes the generated manifests to standard output, which you then pipe to `kubectl`.

### Emitting a Manifest

Construct the resource in Starlark, encode it using the `yaml` module, and write it to standard output:

```python
def main():
    image_tag = var_str("image", "nginx:1.27")
    
    manifest = {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {"name": "alice-web"},
        "spec": {
            "replicas": 3,
            "selector": {"matchLabels": {"app": "alice-web"}},
            "template": {
                "metadata": {"labels": {"app": "alice-web"}},
                "spec": {
                    "containers": [
                        {"name": "web", "image": image_tag}
                    ]
                },
            },
        },
    }
    
    # Encode the dictionary to a YAML string and print to stdout
    print(yaml.encode(manifest))
```

### Integrating with kubectl

Pipe the stdout of the Starkite script directly into `kubectl`:

```bash
# Apply the generated manifest directly
kite run ./gen.star --var image=myapp:v2 | kubectl apply -f -

# Preview changes before applying
kite run ./gen.star --var image=myapp:v2 | kubectl diff -f -
```

### Emitting Multiple Documents

To output multiple resources in a single stream, pass a list of dictionaries to `yaml.encode_all()`. This separates the documents using the standard `---` delimiter:

```python
def export_stack():
    cm = {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": "alice-cfg"},
        "data": {"ENV": "staging"},
    }
    dep = {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {"name": "alice-web"},
    }
    
    # Emits both documents separated by '---'
    print(yaml.encode_all([cm, dep]))
```

---

## Composing Overlays with `k8s.overlay()`

Multi-environment deployments often share a common base set of resources while varying target namespaces, resource name prefixes, container images, replica counts, and security hardening rules across staging and production. Calling `k8s.overlay()` transforms an entire list of manifests offline inside Starkite's default `deny-all` sandbox, without requiring cluster connectivity or maintaining separate patch YAML directories.

`k8s.overlay()` accepts a list of `KubeResource` values, dictionaries, or multi-document YAML strings and returns transformed dictionaries ready for `k8s.yaml()`, `k8s.validate()`, `k8s.diff()`, or `k8s.apply()`:

* **`namespace`**: Sets `metadata.namespace` on namespaced resources while skipping cluster-scoped kinds (`Namespace`, `ClusterRole`, `CustomResourceDefinition`, and similar).
* **`name_prefix` / `name_suffix`**: Renames `metadata.name` on non-`Namespace` resources and rewrites matching core cross-references across sibling manifests (`configMapKeyRef`, `secretKeyRef`, `envFrom`, `volumes`, `serviceAccountName`, `scaleTargetRef`, `roleRef`, and `subjects`).
* **`labels` / `annotations`**: Merges metadata labels and annotations onto every resource without rewriting pod selectors.
* **`images`**: Rewrites matching container image names, tags, or digests across `Pod`, `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`, and `ReplicaSet`.
* **`patches`**: Applies targeted dictionary deep-merges (`{"kind": ..., "name": ..., "patch": {...}}`) or Starlark transform functions (`fn(obj) -> obj | None`, where returning `None` drops the resource).
* **`extra`**: Appends additional environment-specific resources (such as `Namespace`, `PodDisruptionBudget`, or `HorizontalPodAutoscaler`) so they participate in the same namespace, label, and name-prefix transforms.

```python
base_cm = k8s.obj.config_map(name = "the-map", data = {"ENV": "base"})
base_dep = k8s.obj.deployment(
    name = "the-deployment",
    replicas = 1,
    labels = {"app": "hello"},
    containers = [{
        "name": "web",
        "image": "monopole/hello:1",
        "env": [{"name": "APP_ENV", "valueFrom": {"configMapKeyRef": {"name": "the-map", "key": "ENV"}}}],
    }],
)

def harden_containers(obj):
    if obj.get("kind") != "Deployment":
        return obj
    for c in obj["spec"]["template"]["spec"]["containers"]:
        c["securityContext"] = {"runAsNonRoot": True, "readOnlyRootFilesystem": True}
    return obj

manifests = k8s.overlay(
    [base_cm, base_dep],
    namespace = "hello-prod",
    name_prefix = "prod-",
    labels = {"environment": "production"},
    images = {"monopole/hello": "registry.example.internal/monopole/hello:2"},
    patches = [
        {"kind": "Deployment", "name": "the-deployment", "patch": {"spec": {"replicas": 3}}},
        harden_containers,
    ],
    extra = [
        k8s.obj.namespace(name = "hello-prod"),
        k8s.obj.pod_disruption_budget(name = "the-pdb", min_available = 1, selector = {"matchLabels": {"app": "hello"}}),
    ],
)

print(k8s.yaml(manifests))
```

For safe error inspection returning a `Result` instead of raising on invalid input or patch errors, use `k8s.try_overlay()`.
