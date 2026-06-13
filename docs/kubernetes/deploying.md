---
title: "Deploying resources"
description: "Deploy, scale, and autoscale an app — the fastest path from nothing to a running workload"
weight: 50
---

# Deploying resources

The goal here is to get a container image running as a scaling workload with the fewest moving parts. You start from an image and a handful of variables, and you end with a Deployment, a Service, an autoscaler, and a live view of pod resource usage — without writing a line of raw YAML. Each call you make corresponds to a `kubectl` verb you would otherwise type by hand, so this is the kubectl-equivalent path through the `k8s` module.

**Source:** [`examples/cloud/quick-deploy/quick-deploy.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/quick-deploy/quick-deploy.star)

## Script

You begin by reading the values that change between environments — namespace, image, name, replica count — so the same script deploys to staging or production with different flags rather than different code. From there you bind a client to a namespace and drive it through the deploy, scale, autoscale, and inspect sequence:

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

Read top to bottom, the script does four things in order: `k.deploy` creates the workload and its Service and returns the names it assigned, the two `printf` calls echo those names back so you can see what was created, `k.scale` raises the replica count, and `k.autoscale` hands ongoing scaling decisions to an HPA. The final loop pulls live metrics and prints only the pods belonging to this app.

## Run it

Run the script as written to deploy with the defaults, or override any variable on the command line to retarget it. The `--dry-run` flag walks the script without touching the cluster, which is the safe way to confirm what it would do before it does it:

```bash
kite run ./examples/cloud/quick-deploy/quick-deploy.star
kite run ./examples/cloud/quick-deploy/quick-deploy.star --var image=myapp:v2 --var namespace=staging
kite run ./examples/cloud/quick-deploy/quick-deploy.star --dry-run
```

The second form shows the payoff of reading variables up front: pointing the same workload at a `staging` namespace and a new image tag is a matter of two flags, with the script itself unchanged.

## What's happening

Each call in the script maps to one cluster operation, and the cost of the all-in-one calls is that they trade fine-grained control for brevity:

- **`k8s.config(namespace=...)`** binds a client to a namespace, and every subsequent call on that client inherits it — so you set the target once instead of repeating it on each call.
- **`k.deploy(name, image, ...)`** creates a Deployment plus an optional Service in a single call, returning a dict of the created resource names. That convenience is why it expects an image and a few options rather than a full manifest.
- **`k.scale(kind, name, replicas)`** is the `kubectl scale` equivalent, setting the replica count directly.
- **`k.autoscale(...)`** creates a HorizontalPodAutoscaler in one call, after which the cluster — not your script — adjusts the replica count.
- **`k.top_pods()`** returns the metrics-server view of running pods (`cpu_request`, `memory_request`, `status`, and more), so it requires a metrics server in the cluster to report anything.

## Permissions

Creating and mutating cluster resources is a privileged operation, so this script needs more than the default. With no profile it runs under `deny-all`, where `k8s` writes are blocked. The `allow-local` profile is the rung that grants `k8s` read, write, and config, so run the script there:

```bash
kite run ./examples/cloud/quick-deploy/quick-deploy.star --permissions=allow-local
```

See [Permission](../fundamentals/security/permission.md) for the full profile ladder.

## See also

- [`k8s` reference](../references/api/k8s.md) — full Tier 1/2/3 API
- [Object representation](objects.md) — Tier 3 typed constructors for per-environment generation
- [Kubernetes examples](../examples/index.md#kubernetes) — rolling updates, multi-env manifests, full stacks
