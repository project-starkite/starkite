---
title: "Object representation"
description: "How Kubernetes objects map to Starlark values"
weight: 40
---

# Object representation

Most of what you do against a cluster comes down to reading a resource and constructing one, and Starkite lets you do both without learning a new object type. A Kubernetes object crosses the boundary as a plain Starlark value with the same shape as the resource's JSON — a manifest read with `k8s.get` is a `dict`, and a list from `k8s.list` is a list of dicts. You work with the data structures the language already gives you.

## Reading fields

Because a fetched object is just a `dict`, you read its fields the way you read any nested `dict` or list — by indexing down through the keys, walking into lists with a numeric index where the manifest holds an array:

```python
dep = k8s.get("deployment", "web")
name     = dep["metadata"]["name"]
replicas = dep["spec"]["replicas"]
image    = dep["spec"]["template"]["spec"]["containers"][0]["image"]
```

Each step mirrors the manifest exactly, so `dep["spec"]["template"]["spec"]["containers"][0]["image"]` reaches the first container's image the same way the YAML nests it. There is no accessor to memorize; if you can navigate the manifest, you can navigate the value.

## Building objects

Construction runs the same idea in reverse. You assemble a `dict` shaped like the manifest and hand it to `create` or `apply`:

```python
manifest = {
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {"name": "app-config"},
    "data": {"LOG_LEVEL": "info"},
}
k8s.apply(manifest, namespace="default")
```

That dict is the whole object — `apply` sends it to the cluster as a server-side apply and returns the resulting resource as another `dict` you can read back.

When the manifest already exists as text, you skip the dict entirely. Both `create` and `apply` also accept a YAML string, so a manifest read from a file goes straight through:

```python
k8s.apply(path("deploy.yaml").read_text(), namespace="default")
```

This is the path to reach for when a teammate maintains the YAML by hand: you apply exactly what is on disk, with no round-trip through Starlark.

## Typed constructors

Hand-building a deeply nested manifest is error-prone for objects with a lot of required boilerplate, and a CustomResourceDefinition is the worst offender. For that case the `k8s.obj` namespace gives you a typed constructor that fills in the structure and validates it for you:

```python
crd = k8s.obj.crd(...)   # scaffold a CustomResourceDefinition
```

`k8s.obj.crd` returns an ordinary manifest dict, so you apply it like any other object and render it to YAML for review with `k8s.yaml(crd)`. See [object constructors](../references/api/k8s.md#object-constructors) in the API reference for its full parameter set.

The payoff of objects being ordinary dicts shows up everywhere else, too: the [json](../core-modules/json-yaml.md) and [yaml](../core-modules/json-yaml.md) modules encode, diff, and persist them with no conversion, because there is nothing to convert.
