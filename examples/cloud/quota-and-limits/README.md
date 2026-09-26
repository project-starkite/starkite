# Resource Quota and Limit Range

Define and enforce namespace governance policies using `k8s.obj.limit_range` and `k8s.obj.resource_quota`.

## What it demonstrates

- **`k8s.obj.limit_range`**: Defines default request/limit values, upper/lower bounds, and max limit-to-request ratios for containers and storage.
- **`k8s.obj.limit_range_item`**: Typed sub-object specifying constraints per resource type (`Container`, `Pod`, `PersistentVolumeClaim`).
- **`k8s.obj.resource_quota`** (alias `k8s.obj.quota`): Enforces cumulative compute, storage, and object count quotas across a namespace.
- **Multi-document YAML generation**: Output manifests for GitOps workflows, pipe into `kubectl`, or apply programmatically.

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `namespace` | No | `engineering` | Target namespace for the governance policies |
| `apply` | No | `false` | Apply manifests directly to the configured cluster |

## Usage

```bash
# Preview generated multi-document YAML manifests
kite run ./quota-and-limits.star

# Generate YAML for a specific namespace
kite run ./quota-and-limits.star --var namespace=production

# Pipe directly into kubectl
kite run ./quota-and-limits.star --var namespace=staging | kubectl apply -f -

# Apply directly using the starkite Kubernetes client
kite run ./quota-and-limits.star --var namespace=production --var apply=true

# Dry-run execution
kite run --dry-run ./quota-and-limits.star
```

## Generated Resources

- **`LimitRange` (`team-limits`)**:
  - Container default request: 100m CPU / 128Mi Memory
  - Container default limit: 500m CPU / 256Mi Memory
  - Container boundary limits: 50m to 2 CPU, 64Mi to 2Gi Memory
  - PVC storage boundary: 1Gi min, 50Gi max
- **`ResourceQuota` (`compute-quota`)**:
  - Total requests limit: 4 CPU / 8Gi Memory
  - Total limits cap: 8 CPU / 16Gi Memory
  - Pods cap: 20
  - PVCs cap: 10
  - LoadBalancers cap: 2
  - Scope: `NotTerminating`
