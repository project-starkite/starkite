# Cloud Examples

Kubernetes scripting examples for starkite's cloud edition. Each script generates
YAML or interacts with a cluster programmatically — no Helm, no Kustomize, just
Starlark.

## Prerequisites

- **starkite** (cloud edition) — `kite version` should show `cloud` in the build tags
- **kubectl** — configured with cluster access
- A running Kubernetes cluster (kind, minikube, EKS, GKE, etc.)

## The Pipe Pattern

starkite generates Kubernetes YAML to stdout, which pipes directly into kubectl:

```bash
kite run script.star | kubectl apply -f -     # apply
kite run script.star | kubectl diff -f -       # preview changes
kite run script.star | kubectl apply --dry-run=server -f -  # validate
kite run script.star > manifests.yaml          # save for GitOps
```

This replaces `helm template`, `kustomize build`, and `envsubst` with a
programmable YAML generator. Variables are passed via `--var key=value` flags.

> **Note:** starkite has no stdin support — piping works only in the
> `kite -> stdout` direction.

## Examples

### Getting Started

| Example | Description |
|---------|-------------|
| [quick-deploy](quick-deploy/) | Deploy, scale, and inspect an app with zero YAML (Tier 2 abstractions) |
| [deploy-k8s](deploy-k8s/) | Generate Namespace + ConfigMap + Deployment + Service manifests via `yaml.encode()` |

### Application Patterns

| Example | Description |
|---------|-------------|
| [app-stack](app-stack/) | Full app stack using `k8s.obj` constructors with probes, resources, and volumes (Tier 3) |
| [multi-env](multi-env/) | Generate Deployment + Service for dev/staging/prod with per-env profiles |
| [rolling-update](rolling-update/) | Image update with watch-based health monitoring and auto-rollback |

### Helm Replacement

| Example | Description |
|---------|-------------|
| [wordpress-stack](wordpress-stack/) | WordPress + MySQL — script-driven deployment with zero piping |
| [redis-cluster](redis-cluster/) | Redis StatefulSet with `--var-file` for Helm-like values |
| [microservices](microservices/) | Multi-service app — composable functions replace subcharts |

### Cluster Operations

| Example | Description |
|---------|-------------|
| [cluster-health](cluster-health/) | Read-only cluster health audit — nodes, deployments, stuck pods |
| [namespace-stack](namespace-stack/) | Scaffold a namespace with ResourceQuota + LimitRange + RBAC |
| [debug-pod](debug-pod/) | Pod debugging toolkit — describe, logs, exec, port-forward |

### Batch

| Example | Description |
|---------|-------------|
| [cronjobs](cronjobs/) | Generate a batch of CronJobs from a flat config list |

## Quick Reference

```bash
# Scaffold a namespace with quotas and RBAC
kite run examples/cloud/namespace-stack/namespace-stack.star --var name=team-backend | kubectl apply -f -

# Deploy to all environments at once
kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 | kubectl apply -f -

# Deploy to just staging
kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 --var envs=staging | kubectl apply -f -

# Apply CronJobs to a specific namespace
kite run examples/cloud/cronjobs/cronjobs.star --var ns=batch --var image=myapp:v3 | kubectl apply -f -

# Full app stack with Tier 3 constructors
kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --var namespace=staging

# WordPress stack (imperative — applies directly to cluster)
kite run examples/cloud/wordpress-stack/wordpress-stack.star --var domain=blog.example.com

# Redis cluster with Helm-like values file
kite run examples/cloud/redis-cluster/redis-cluster.star --var-file examples/cloud/redis-cluster/values.yaml | kubectl apply -f -

# Microservices with --var overrides (replaces umbrella chart)
kite run examples/cloud/microservices/microservices.star --var image.tag=v2.1 --var domain=app.example.com | kubectl apply -f -

# Read-only cluster health check
kite run examples/cloud/cluster-health/cluster-health.star

# Debug a misbehaving pod
kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123
```
