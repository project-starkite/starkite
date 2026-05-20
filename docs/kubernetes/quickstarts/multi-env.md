---
title: "Multi-environment manifests"
description: "Generate Deployment + Service manifests for dev / staging / prod from one script"
weight: 40
---

# Multi-environment manifests

Generate per-environment Kubernetes manifests from a single starkite script — different replica counts, resource profiles, and namespaces for dev, staging, and prod, all defined as Starlark dicts. The script writes YAML to stdout, ready to pipe to `kubectl apply`, `kubectl diff`, or `kubectl apply --dry-run=server`.

This is the Tier 3 (typed constructors) path: every Kubernetes object is built with `k8s.obj.*` rather than raw dicts, so missing fields and type mismatches surface at script runtime instead of silently shipping broken manifests.

**Source:** [`examples/cloud/multi-env/multi-env.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/multi-env/multi-env.star)

## Per-environment profile dict

```python
profiles = {
    "dev":     {"replicas": 1, "cpu": "100m", "mem": "128Mi"},
    "staging": {"replicas": 2, "cpu": "200m", "mem": "256Mi"},
    "prod":    {"replicas": 5, "cpu": "500m", "mem": "512Mi"},
}
```

## Build per-environment Deployment + Service

```python
image = var_str("image")
app = var_str("app", "web")
envs = var_str("envs", "dev,staging,prod").split(",")

for e in envs:
    p = profiles.get(e, profiles["dev"])

    dep = k8s.obj.deployment(
        name=app,
        labels={"app": app, "env": e},
        replicas=p["replicas"],
        containers=[
            k8s.obj.container(
                name=app,
                image=image,
                ports=[k8s.obj.container_port(container_port=8080, name="http")],
                resources=k8s.obj.resource_requirements(
                    requests={"cpu": p["cpu"], "memory": p["mem"]},
                ),
                env=[
                    k8s.obj.env_var(name="ENV", value=e),
                    k8s.obj.env_var(name="APP_NAME", value=app),
                ],
            ),
        ],
    )

    svc = k8s.obj.service(
        name=app,
        selector={"app": app, "env": e},
        ports=[k8s.obj.service_port(name="http", port=80, target_port=8080)],
    )

    printf("# --- %s-%s ---\n", app, e)
    print(k8s.yaml([dep, svc]))
```

## Run it

```bash
# Generate manifests for all three environments and pipe to kubectl
kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 | kubectl apply -f -

# Generate only staging + prod
kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 --var envs=staging,prod | kubectl apply -f -

# Server-side dry-run, then diff
kite run examples/cloud/multi-env/multi-env.star --var image=nginx:latest | kubectl apply --dry-run=server -f -
kite run examples/cloud/multi-env/multi-env.star --var image=nginx:latest | kubectl diff -f -
```

## What's happening

- **`k8s.obj.deployment(...)` / `k8s.obj.service(...)` / `k8s.obj.container(...)`** are typed constructors. Each takes the same kwargs the Kubernetes API uses, validates them at construction, and returns a `KubeResource` value.
- **`k8s.yaml(resources)`** serializes one or many `KubeResource` values to multi-document YAML — no need to wire `yaml.encode()` manually.
- **No cluster contact**: the script generates manifests only. Pair it with `kubectl` for the apply step, or call `k.apply([...])` from inside the script if direct application fits the workflow.
- **Variable injection** drives the targeting: `--var image=...` and `--var envs=...` parameterize the same script across deploy contexts.

## See also

- [`k8s` reference](../../references/api/k8s.md) — full `k8s.obj.*` constructor catalog
- [App stack](app-stack.md) — typed constructors composed with `k.apply()` to deploy directly instead of piping through kubectl
