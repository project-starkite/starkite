# Kubernetes Object Traversal & AttrDict Operations

Demonstrates how starkite represents Kubernetes resources as `AttrDict` objects supporting recursive dot notation, dictionary indexing (`starlark.Mapping`), dictionary iteration (`starlark.IterableMapping`), in-place mutation, and JSON/YAML serialization.

## What it demonstrates

- **Recursive dot notation**: `pod.metadata.name`, `pod.status.phase`, `pod.spec.containers[0].image`
- **Mapping indexing**: `pod["metadata"]["name"]`, `pod.get("kind")`
- **IterableMapping & methods**: `len(pod)`, `pod.keys()`, `pod.values()`, `for k, v in pod.metadata.labels.items():`
- **In-place mutation**: `pod["metadata"]["labels"]["starkite.io/audited"] = "true"`
- **Serialization & conversion**: `json.encode()`, `yaml.encode()`, and `pod.to_dict()`

## Usage

```bash
# Inspect first pod in kube-system
kite run examples/cloud/object-traversal/object-traversal.star

# Inspect pods in a specific namespace
kite run examples/cloud/object-traversal/object-traversal.star --var namespace=default
```
