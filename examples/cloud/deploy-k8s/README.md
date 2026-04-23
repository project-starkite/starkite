# Deploy K8s

Generate Namespace + ConfigMap + Deployment + Service manifests via
`yaml.encode()` — a programmable alternative to Helm and Kustomize.

## What it demonstrates

- **Tier 1** (raw YAML generation): `yaml.encode()` for each manifest
- Environment-based configuration (dev/staging/prod) using `env()` variables
- Standard Kubernetes labeling conventions
- Structured resource limits per environment

## Variables

This example uses **environment variables** (`env()`) instead of `var()`:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ENVIRONMENT` | No | `dev` | Target environment (`dev`, `staging`, `prod`) |
| `APP_NAME` | No | `myapp` | Application name |
| `APP_VERSION` | No | `1.0.0` | Application version label |
| `APP_IMAGE` | No | `nginx` | Container image |
| `IMAGE_TAG` | No | `latest` | Image tag |
| `NAMESPACE` | No | `<app>-<env>` | Override the target namespace |

## Usage

```bash
# Generate YAML for dev (default)
kite run examples/cloud/deploy-k8s/deploy-k8s.star

# Pipe to kubectl
kite run examples/cloud/deploy-k8s/deploy-k8s.star | kubectl apply -f -

# Production config
ENVIRONMENT=prod kite run examples/cloud/deploy-k8s/deploy-k8s.star

# Save for GitOps
ENVIRONMENT=staging kite run examples/cloud/deploy-k8s/deploy-k8s.star > manifests/staging.yaml
```

## What it creates

- **Namespace** — named `<app>-<env>` (e.g. `myapp-dev`)
- **ConfigMap** — app settings (environment, log level, version)
- **Deployment** — with environment-specific replicas and resource limits, liveness/readiness probes
- **Service** — ClusterIP on port 80

## How it works

1. Reads environment variables to build a config dict with per-env profiles for replicas and resources
2. Helper functions generate standard Kubernetes labels and selector labels
3. Each manifest is built as a Starlark dict and encoded via `yaml.encode()`
4. All four manifests are printed as a multi-document YAML stream (`---` separators)
