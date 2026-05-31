---
title: "Object representation"
description: "How Kubernetes objects map to Starlark values"
weight: 40
---

# Object representation

Kubernetes objects cross the starkite boundary as plain Starlark values — the same shape as the resource's JSON. A manifest read with `k8s.get` is a `dict`; a list from `k8s.list` is a list of dicts. There is no special object type to learn.

## Reading fields

Access nested fields with normal dict/list indexing:

```python
dep = k8s.get("deployment", "web")
name     = dep["metadata"]["name"]
replicas = dep["spec"]["replicas"]
image    = dep["spec"]["template"]["spec"]["containers"][0]["image"]
```

## Building objects

Construct a manifest as a dict and pass it to `create`/`apply`:

```python
manifest = {
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {"name": "app-config"},
    "data": {"LOG_LEVEL": "info"},
}
k8s.apply(manifest, namespace="default")
```

`create`/`apply` also accept a YAML string, so a manifest read from a file works directly:

```python
k8s.apply(path("deploy.yaml").read_text(), namespace="default")
```

## Typed constructors

For common objects, the `k8s.obj` helpers and `k8s.yaml` / `k8s.config` builders reduce boilerplate and validate structure. CRDs have a dedicated constructor:

```python
crd = k8s.obj.crd(...)   # scaffold a CustomResourceDefinition
```

See [object constructors](../references/api/k8s.md#object-constructors) in the API reference. Because objects are ordinary dicts, the [json](../core-modules/json-yaml.md) and [yaml](../core-modules/json-yaml.md) modules encode, diff, and persist them with no conversion.
