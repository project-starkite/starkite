# Cluster Health

Read-only cluster health audit — scans nodes, deployments, and pods for common
problems. Touches nothing, safe to run against production.

## What it demonstrates

- **Read-only operations**: `k.list()`, `k.top_nodes()`, `k.version()`, `k.context()`
- Node condition checks (Ready, MemoryPressure, DiskPressure, PIDPressure)
- Deployment replica health across namespaces
- Crash-looping and stuck pod detection
- Exit-code-based pass/fail for CI integration

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `k8s.context` | No | current context | Kubernetes context to target |
| `skip.system` | No | `true` | Skip `kube-*` namespaces when checking deployments and pods |

## Usage

```bash
# Check current cluster
kite run examples/cloud/cluster-health/cluster-health.star

# Check a specific context
kite run examples/cloud/cluster-health/cluster-health.star --var k8s.context=prod-cluster

# Include system namespaces
kite run examples/cloud/cluster-health/cluster-health.star --var skip.system=false
```

## What it creates

Nothing — this is a read-only audit script.

## How it works

1. Connects to the cluster and prints the context name and server version
2. **Node check**: lists all nodes, reports Ready/NotReady status and any pressure conditions
3. **Node resources**: calls `k.top_nodes()` to show CPU and memory capacity vs. allocatable
4. **Deployment check**: iterates all namespaces, counts healthy vs. under-replicated deployments
5. **Pod check**: scans for pods in Pending/Unknown phase or containers with >5 restarts
6. **Summary**: prints all issues found and calls `fail()` if any exist (non-zero exit code)
