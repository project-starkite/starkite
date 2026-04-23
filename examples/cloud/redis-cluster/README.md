# Redis Cluster

Replaces: `helm install redis bitnami/redis -f values.yaml`

Redis StatefulSet with `--var-file` for Helm-like values files. Ships with a
bundled `values.yaml` showing tunable defaults. Pipe pattern — generates YAML
to stdout.

## What it demonstrates

- **`--var-file=values.yaml`** — Helm-like values files with `--var` overrides
- `k8s.obj.stateful_set()` with `service_name` and `volume_claim_templates`
- `k8s.obj.service()` with `cluster_ip="None"` for headless Service
- `k8s.obj.config_map()` with generated redis.conf
- `k8s.obj.secret()` with `string_data` (conditional — only when password set)
- Pure YAML generation via `k8s.yaml()`

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `image` | No | `redis:7-alpine` | Redis image |
| `replicas` | No | `3` | StatefulSet replicas |
| `storage` | No | `5Gi` | PVC size per replica |
| `storage.class` | No | *(empty)* | StorageClass |
| `maxmemory` | No | `256mb` | Redis maxmemory setting |
| `maxmemory.policy` | No | `allkeys-lru` | Eviction policy |
| `password` | No | *(empty = no auth)* | Redis password |
| `namespace` | No | `default` | Target namespace |

## Usage

```bash
# Generate with built-in defaults
kite run examples/cloud/redis-cluster/redis-cluster.star | kubectl apply -f -

# Use the bundled values.yaml (same as defaults, but demonstrates --var-file)
kite run examples/cloud/redis-cluster/redis-cluster.star \
  --var-file examples/cloud/redis-cluster/values.yaml | kubectl apply -f -

# Override specific values — --var takes precedence over --var-file
kite run examples/cloud/redis-cluster/redis-cluster.star \
  --var-file examples/cloud/redis-cluster/values.yaml \
  --var replicas=5 --var password=secret | kubectl apply -f -

# Dry-run validation
kite run examples/cloud/redis-cluster/redis-cluster.star | kubectl apply --dry-run=client -f -

# Save to file for GitOps
kite run examples/cloud/redis-cluster/redis-cluster.star > redis.yaml
```

## What it creates

- **ConfigMap** — `redis-config` with generated redis.conf (maxmemory, eviction policy, AOF)
- **Secret** — `redis-auth` (conditional — only when `password` is set)
- **Headless Service** — `redis-headless` (clusterIP: None) for StatefulSet pod DNS
- **Client Service** — `redis` for application connections on port 6379
- **StatefulSet** — `redis` with `volumeClaimTemplates` for per-replica persistent storage

## How it works

1. Reads variables from `--var-file` and/or `--var` flags (var overrides var-file)
2. Generates redis.conf from `maxmemory`, `maxmemory.policy`, and `password` variables
3. Builds ConfigMap, optional Secret, headless Service, client Service, and StatefulSet
4. Outputs all resources as multi-document YAML via `k8s.yaml()`
