---
title: "Deploying resources"
description: "Deploy, scale, and autoscale an app — the fastest path from nothing to a running workload"
weight: 50
---

# Deploying resources

The shortest path from a container image to a running, scaling workload. The script reads variables for namespace, image, name, and replicas; deploys a Deployment + ClusterIP Service in one call; scales the Deployment up; attaches an HPA; then prints pod resource usage. No raw YAML.

This is the Tier 2 (kubectl-equivalent) path through the `k8s` module. Each call corresponds to a `kubectl` verb you'd otherwise type.

**Source:** [`examples/cloud/quick-deploy/quick-deploy.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/quick-deploy/quick-deploy.star)

## Script

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

## Run it

```bash
kite run ./examples/cloud/quick-deploy/quick-deploy.star
kite run ./examples/cloud/quick-deploy/quick-deploy.star --var image=myapp:v2 --var namespace=staging
kite run ./examples/cloud/quick-deploy/quick-deploy.star --dry-run
```

## What's happening

- **`k8s.config(namespace=...)`** binds a client to a namespace. Subsequent calls inherit it.
- **`k.deploy(name, image, ...)`** creates a Deployment plus an optional Service in a single call. Returns a dict with the created resource names.
- **`k.scale(kind, name, replicas)`** is the `kubectl scale` equivalent.
- **`k.autoscale(...)`** creates a HorizontalPodAutoscaler in one call.
- **`k.top_pods()`** returns the metrics-server view of running pods — `cpu_request`, `memory_request`, `status`, etc.

## See also

- [`k8s` reference](../references/api/k8s.md) — full Tier 1/2/3 API
- [Object representation](objects.md) — Tier 3 typed constructors for per-environment generation
- [Kubernetes examples](../examples/index.md#kubernetes) — rolling updates, multi-env manifests, full stacks
