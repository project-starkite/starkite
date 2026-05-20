---
title: "Full app stack with constructors"
description: "Compose a ConfigMap + Deployment + Service with probes, resources, and volumes"
weight: 50
---

# Full app stack with constructors

A complete application stack assembled with `k8s.obj.*` constructors and applied in one call. Demonstrates the Tier 3 (typed constructors) + Tier 1 (`apply`, `wait_for`) composition: every resource is built with validated constructors that catch shape errors at script runtime, then applied as a multi-document set with a single `k.apply()`.

The script builds a ConfigMap (app settings), a Deployment (with HTTP + metrics ports, readiness + liveness probes, resource requests + limits, env vars, env-from-ConfigMap, and tmp + config volume mounts), and a ClusterIP Service exposing both ports — then applies them and waits for the deployment to become available.

**Source:** [`examples/cloud/app-stack/app-stack.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/app-stack/app-stack.star)

## ConfigMap

```python
def build_config_map(name, ns):
    return k8s.obj.config_map(
        name=name + "-config",
        data={
            "APP_ENV": ns,
            "LOG_LEVEL": "debug" if ns == "default" else "info",
            "LOG_FORMAT": "json",
            "MAX_CONNECTIONS": "100",
        },
    )
```

## Deployment with probes, resources, env-from, volumes

```python
def build_deployment(name, image, replicas):
    container = k8s.obj.container(
        name=name,
        image=image,
        ports=[
            k8s.obj.container_port(container_port=80, name="http"),
            k8s.obj.container_port(container_port=9090, name="metrics"),
        ],
        env=[
            k8s.obj.env_var(name="APP_NAME", value=name),
            k8s.obj.env_var(name="POD_NAME", value_from={
                "fieldRef": {"fieldPath": "metadata.name"},
            }),
        ],
        env_from=[
            k8s.obj.env_from(config_map_ref={"name": name + "-config"}),
        ],
        resources=k8s.obj.resource_requirements(
            requests={"cpu": "100m", "memory": "128Mi"},
            limits={"cpu": "500m", "memory": "512Mi"},
        ),
        readiness_probe=k8s.obj.probe(
            http_get={"path": "/", "port": 80},
            initial_delay_seconds=5,
            period_seconds=10,
        ),
        liveness_probe=k8s.obj.probe(
            http_get={"path": "/", "port": 80},
            initial_delay_seconds=15,
            period_seconds=20,
            timeout_seconds=3,
        ),
        volume_mounts=[
            k8s.obj.volume_mount(name="tmp", mount_path="/tmp"),
            k8s.obj.volume_mount(name="config", mount_path="/etc/app", read_only=True),
        ],
    )

    return k8s.obj.deployment(
        name=name,
        labels={"app": name},
        replicas=replicas,
        containers=[container],
        volumes=[
            k8s.obj.volume(name="tmp", empty_dir={}),
            k8s.obj.volume(name="config", config_map={"name": name + "-config"}),
        ],
    )
```

## Apply + wait

```python
ns = var_str("namespace", "default")
image = var_str("image")
name = var_str("app.name", "myapp")
replicas = var_int("replicas", 2)

k = k8s.config(namespace=ns)

cm = build_config_map(name, ns)
dep = build_deployment(name, image, replicas)
svc = build_service(name)

# Preview the YAML
printf("%s\n", k8s.yaml([cm, dep, svc]))

# Apply all three at once
k.apply([cm, dep, svc])

# Wait for the rollout
result = k.wait_for("deployment", name, condition="available", timeout="5m")
if not result["ready"]:
    fail("deployment failed: " + result["message"])
```

## Run it

```bash
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --var namespace=staging
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --dry-run
```

## What's happening

- **`k8s.obj.*`** constructors validate every field at construction time. A typo in `readiness_probe` or `volume_mount` surfaces as a Starlark error before any cluster contact.
- **`k.apply([cm, dep, svc])`** accepts a list of `KubeResource` values and runs server-side apply on each — ConfigMap, Deployment, Service — in one call.
- **`k.wait_for("deployment", name, condition="available")`** blocks until the deployment is rolled out; returns the final observed resource so the caller can inspect `.message` on failure.
- **Composition**: pure functions like `build_deployment` make the assembly testable and reusable across multiple stacks.

## See also

- [`k8s` reference](../../references/api/k8s.md) — full `k8s.obj.*` constructor catalog
- [Multi-environment manifests](multi-env.md) — same constructor approach but pipes YAML to kubectl instead of applying directly
