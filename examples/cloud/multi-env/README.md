# Multi-Env

Generate Deployment + Service manifests for dev, staging, and prod in one shot —
each with per-environment replica counts and resource profiles.

## What it demonstrates

- **Tier 3** (`k8s.obj.*` constructors) for typed manifest generation
- Per-environment profiles (replicas, CPU, memory)
- Selective environment targeting via the `envs` variable
- Pure YAML generation — pipe to kubectl to apply

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `image` | **Yes** | — | Container image (e.g. `myapp:v2.1`) |
| `app` | No | `web` | Application name |
| `envs` | No | `dev,staging,prod` | Comma-separated list of environments to generate |

## Usage

```bash
# Generate manifests for all environments
kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 | kubectl apply -f -

# Just staging and prod
kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 --var envs=staging,prod | kubectl apply -f -

# Dry-run validation
kite run examples/cloud/multi-env/multi-env.star --var image=nginx:latest | kubectl apply --dry-run=server -f -

# Preview changes
kite run examples/cloud/multi-env/multi-env.star --var image=nginx:latest | kubectl diff -f -
```

## What it creates

Per environment:

- **Deployment** — with environment-specific replicas and resource requests
- **Service** — ClusterIP on port 80 targeting port 8080

Environment profiles:

| Environment | Replicas | CPU Request | Memory Request |
|-------------|----------|-------------|----------------|
| dev | 1 | 100m | 128Mi |
| staging | 2 | 200m | 256Mi |
| prod | 5 | 500m | 512Mi |

## How it works

1. Defines a `profiles` dict with per-environment resource profiles
2. Parses the `envs` variable to determine which environments to generate
3. For each environment, builds a Deployment and Service using `k8s.obj.*` constructors
4. Each resource is namespaced to `<app>-<env>` (e.g. `web-staging`)
5. Outputs all manifests as a multi-document YAML stream with `k8s.yaml()`
