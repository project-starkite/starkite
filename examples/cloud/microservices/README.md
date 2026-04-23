# Microservices

Replaces: `helm install myapp ./umbrella-chart` (umbrella chart with subcharts)

Multi-service application with composable functions that replace Helm subcharts.
Frontend + API + Worker deployed from a shared service profile dict. Cross-service
wiring is just code (`"http://api:%d" % port`). Pipe pattern — generates YAML to stdout.

## What it demonstrates

- **`--var` driven** piping — composable functions replace subcharts
- `for name in services:` iteration over a service profile dict
- Cross-service wiring via code (not template expressions)
- Shared ConfigMap replacing Helm global values
- Conditional Ingress with path-based routing (`/api` -> api, `/` -> frontend)
- Pure YAML generation via `k8s.yaml()`

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `image.tag` | No | `latest` | Image tag for all services |
| `image.registry` | No | `myapp` | Image registry/org |
| `domain` | No | *(empty = no Ingress)* | Ingress hostname |
| `log.level` | No | `info` | Shared log level |
| `namespace` | No | `default` | Target namespace |

## Usage

```bash
# Generate with defaults
kite run examples/cloud/microservices/microservices.star | kubectl apply -f -

# Set image tag and enable Ingress
kite run examples/cloud/microservices/microservices.star \
  --var image.tag=v2.1 --var domain=app.example.com | kubectl apply -f -

# Custom registry and log level
kite run examples/cloud/microservices/microservices.star \
  --var image.registry=ghcr.io/myorg --var log.level=debug | kubectl apply -f -

# Dry-run validation
kite run examples/cloud/microservices/microservices.star | kubectl apply --dry-run=client -f -

# Save to file for GitOps
kite run examples/cloud/microservices/microservices.star > manifests.yaml
```

## What it creates

- **ConfigMap** — `shared-config` with environment, log format, API URL (replaces Helm globals)
- **Deployment** x3 — `frontend` (2 replicas), `api` (3 replicas), `worker` (2 replicas)
- **Service** x3 — one per component, exposing respective ports
- **Ingress** — conditional, routes `/api` to api service and `/` to frontend

## Service profiles

| Service | Port | Replicas | Health endpoint |
|---------|------|----------|-----------------|
| frontend | 80 | 2 | `/` |
| api | 8080 | 3 | `/healthz` |
| worker | 9090 | 2 | `/healthz` |

## How it works

1. Defines a `services` dict with per-component profiles (port, replicas, health path)
2. `make_deployment()` builds a Deployment for any service — replaces a Helm subchart
3. `make_service()` builds a matching Service
4. Iterates `for name in services:` to generate all Deployment + Service pairs
5. Cross-service URLs are computed in code: `"http://api:%d" % api_port`
6. Builds a shared ConfigMap (replaces Helm global values)
7. Outputs all resources as multi-document YAML via `k8s.yaml()`
