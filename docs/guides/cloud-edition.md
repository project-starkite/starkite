---
title: "Cloud Edition"
description: "Kubernetes and cloud modules"
weight: 5
---

The cloud edition of starkite adds Kubernetes (`k8s` module) and the `kite kube` artifact-generation subcommand on top of the base edition. It ships as the standalone `cloudkite` binary, and is also bundled into the all-in-one `kite` binary.

## Installation

```bash
# Build from source — produces ./bin/cloudkite
make build-cloud

# Or install via the edition manager (downloads from GitHub Releases)
basekite edition use cloud
```

If you have the all-in-one `kite` binary installed, you already have the cloud module — no separate install needed.

## Kubernetes Module

The `k8s` module provides full Kubernetes resource management:

```python
# List pods in a namespace
pods = k8s.list("pod", namespace="default")
for pod in pods:
    print(pod["metadata"]["name"])

# Create a deployment
k8s.deploy("nginx", "nginx:latest", replicas=3, port=80)

# Apply a manifest
manifest = yaml.encode({
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {"name": "my-config", "namespace": "default"},
    "data": {"key": "value"},
})
k8s.apply(manifest)

# Scale a deployment
k8s.scale("deployment", "nginx", 5)

# Rolling restart
k8s.rollout("deployment", "nginx", action="restart")
```

## Available Kubernetes Functions

| Function | Description |
|----------|-------------|
| `k8s.get(kind, name)` | Get a single resource |
| `k8s.list(kind)` | List resources |
| `k8s.create(manifest)` | Create a resource |
| `k8s.apply(manifest)` | Server-side apply |
| `k8s.delete(kind, name)` | Delete a resource |
| `k8s.patch(kind, name, patch)` | Patch a resource |
| `k8s.label(kind, name, labels)` | Set labels |
| `k8s.annotate(kind, name, annotations)` | Set annotations |
| `k8s.deploy(name, image, ...)` | Create Deployment + optional Service |
| `k8s.run(name, image, ...)` | Run a Pod |
| `k8s.expose(kind, name, port)` | Create a Service |
| `k8s.scale(kind, name, replicas)` | Scale a workload |
| `k8s.rollout(kind, name, action)` | Manage rollouts |
| `k8s.set_image(kind, name, container, image)` | Update container image |
| `k8s.set_env(kind, name, env)` | Update environment variables |
| `k8s.set_resources(kind, name, ...)` | Update resource limits |
| `k8s.autoscale(kind, name, ...)` | Create HPA |
| `k8s.version()` | Get server version |
| `k8s.api_resources()` | List API resources |

## Editions Management

```bash
basekite edition status        # List installed editions
basekite edition use cloud     # Install cloud edition (downloads cloudkite)
basekite edition use base      # Switch back to base
basekite edition remove cloud  # Uninstall cloud edition
```

When cloud is active, `basekite` transparently execs into `cloudkite` for every command.
