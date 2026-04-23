# App Stack

Build and apply a full application stack using `k8s.obj` constructors — typed,
validated Kubernetes objects with probes, resource limits, volumes, and envFrom.

## What it demonstrates

- **Tier 3** (`k8s.obj.*` constructors) composed with **Tier 1** (`k.apply()`, `k.wait_for()`)
- `k8s.obj.container()` with ports, env, envFrom, resources, probes, and volume mounts
- `k8s.obj.pod_spec()` with volumes
- `k8s.obj.deployment()`, `k8s.obj.service()`, `k8s.obj.config_map()`
- `k8s.yaml()` for preview before apply

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `image` | **Yes** | — | Container image (e.g. `myapp:v1`) |
| `namespace` | No | `default` | Target namespace |
| `app.name` | No | `myapp` | Application name |
| `replicas` | No | `2` | Replica count |

## Usage

```bash
# Deploy to default namespace
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1

# Deploy to staging
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --var namespace=staging

# Dry-run — preview YAML without applying
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --dry-run
```

## What it creates

- **ConfigMap** — `<name>-config` with app environment settings (log level, format, max connections)
- **Deployment** — with readiness/liveness probes, CPU/memory limits, emptyDir + configMap volumes
- **Service** — ClusterIP exposing HTTP (80) and metrics (9090) ports

## How it works

1. `build_config_map()` creates a ConfigMap with environment-aware settings
2. `build_deployment()` assembles a container with probes, resources, env vars (including downward API for pod name), envFrom (ConfigMap), and volume mounts — then wraps it in a pod spec with volumes and a Deployment
3. `build_service()` creates a ClusterIP Service with HTTP and metrics ports
4. All three objects are previewed via `k8s.yaml()`, then applied with `k.apply()`
5. `k.wait_for()` blocks until the Deployment reaches the `available` condition
