# Namespace Stack

Scaffold a complete namespace with ResourceQuota, LimitRange, and RBAC — pure
YAML generation via `yaml.encode()`, no cluster access needed.

## What it demonstrates

- **Tier 1** (raw YAML generation): `yaml.encode()` for each manifest
- Complete namespace bootstrapping pattern
- Resource quotas and default container limits
- RBAC RoleBinding for team-scoped admin access

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `name` | **Yes** | — | Namespace name (also used as team/group name) |
| `cpu` | No | `8` | CPU limit for the ResourceQuota |
| `mem` | No | `16Gi` | Memory limit for the ResourceQuota |

## Usage

```bash
# Scaffold and apply a namespace
kite run examples/cloud/namespace-stack/namespace-stack.star --var name=team-backend | kubectl apply -f -

# Preview changes
kite run examples/cloud/namespace-stack/namespace-stack.star --var name=team-backend | kubectl diff -f -

# Custom quotas
kite run examples/cloud/namespace-stack/namespace-stack.star --var name=staging --var cpu=4 --var mem=8Gi > ns.yaml
```

## What it creates

- **Namespace** — with `team` and `managed-by` labels
- **ResourceQuota** — caps total CPU, memory, and pod count (50) in the namespace
- **LimitRange** — sets default container requests (100m CPU, 128Mi) and limits (200m CPU, 256Mi)
- **RoleBinding** — grants the `admin` ClusterRole to the team Group (matched by namespace name)

## How it works

1. Reads the `name`, `cpu`, and `mem` variables
2. Builds four manifest dicts (Namespace, ResourceQuota, LimitRange, RoleBinding)
3. Encodes each with `yaml.encode()` and prints them as a multi-document YAML stream
4. Output is designed to be piped into `kubectl apply -f -`
