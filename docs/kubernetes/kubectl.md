---
title: "Piping to kubectl"
description: "Generate manifests in a script and apply them with kubectl"
weight: 20
---

# Piping to kubectl

A starkite script can generate Kubernetes manifests and hand them to `kubectl` over stdout — useful when a workflow standardizes on `kubectl apply` for the actual cluster mutation while using starkite for templating and logic.

## Emit a manifest to stdout

Build the object as a dict, encode it to YAML, and print it:

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

Pipe it straight into `kubectl`:

```bash
kite run gen.star --var image=myapp:v2 | kubectl apply -f -
kite run gen.star --var image=myapp:v2 | kubectl diff -f -
```

## Multiple objects

Emit a multi-document stream with `yaml.encode_all`:

```python
print(yaml.encode_all([deployment, service, configmap]))
```

```bash
kite run stack.star | kubectl apply -f -
```

This keeps cluster mutation in `kubectl` (and its RBAC, dry-run, and diff tooling) while starkite handles variable injection and conditional logic. To apply directly from the script instead, see [Deploying resources](deploying.md).
