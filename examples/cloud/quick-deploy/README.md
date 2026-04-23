# Quick Deploy

Deploy, scale, and inspect an app with zero YAML — the fastest path from
nothing to a running Kubernetes app.

## What it demonstrates

- **Tier 2** (kubectl abstractions): `k.deploy()`, `k.scale()`, `k.autoscale()`, `k.top_pods()`
- One-call deployment that creates both a Deployment and a ClusterIP Service
- Scaling and HPA setup in a single script
- Pod resource usage inspection

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `namespace` | No | `default` | Target namespace |
| `image` | No | `nginx:1.27` | Container image |
| `app.name` | No | `web` | Application name for the Deployment and Service |
| `replicas` | No | `3` | Initial replica count |

## Usage

```bash
# Deploy with defaults
kite run examples/cloud/quick-deploy/quick-deploy.star

# Custom image
kite run examples/cloud/quick-deploy/quick-deploy.star --var image=myapp:v2

# Deploy to a specific namespace
kite run examples/cloud/quick-deploy/quick-deploy.star --var namespace=staging

# Dry-run mode
kite run examples/cloud/quick-deploy/quick-deploy.star --dry-run
```

## What it creates

- **Deployment** — runs the specified image with the given replica count
- **Service** — ClusterIP service exposing port 80
- **HPA** — HorizontalPodAutoscaler targeting 70% CPU utilization

## How it works

1. Calls `k.deploy()` to create the Deployment + Service in one shot
2. Scales up by 2 replicas via `k.scale()`
3. Sets up an HPA with `k.autoscale()` for automatic scaling
4. Lists pod resource usage with `k.top_pods()`, filtered to the app's pods
