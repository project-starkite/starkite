# Controller Examples

These examples demonstrate building Kubernetes controllers with starkite's `k8s.control()` function.

## Prerequisites

- A running Kubernetes cluster (e.g., `kind create cluster`)
- The `kite-cloud` binary (`go build -o kite-cloud ./cmd/cloud/starkite/`)

## Examples

### resource.star

Defines a `MyApp` CustomResourceDefinition using `k8s.obj.crd()`. This file is loaded by other scripts to share the CRD definition.

```bash
kite-cloud run resource.star
```

This prints the generated CRD YAML, useful for review or piping to `kubectl apply -f -`.

### deploy-controller.star

Deploys a complete controller stack programmatically: CRD, namespace, RBAC, and a controller Deployment. No YAML files needed — everything is expressed in Starlark.

```bash
# Deploy with defaults
kite-cloud run deploy-controller.star

# Override image and replicas
kite-cloud run deploy-controller.star --var image=myregistry/controller:v1.2.0 --var replicas=3
```

The script loads the CRD from `resource.star`, then creates all supporting resources (namespace, ServiceAccount, ClusterRole, ClusterRoleBinding) and the controller Deployment in sequence.

### Generating artifacts with `kite-cloud kube gen-controller-artifacts`

For teams that prefer static YAML, `kite-cloud` can generate manifests from a Starlark definition:

```bash
# Generate YAML manifests to stdout
kite-cloud kube gen-controller-artifacts \
    --controller controller.star \
    --resource resource.star \
    --image myregistry/myapp-controller:v1 \
    --namespace myapp-system > deploy.yaml

# Generate a Starlark deploy script
kite-cloud kube gen-controller-artifacts \
    --controller controller.star \
    --resource resource.star \
    --image myregistry/myapp-controller:v1 \
    --output script > deploy-controller.star

# Also generate a Dockerfile
kite-cloud kube gen-controller-artifacts \
    --controller controller.star \
    --image myregistry/myapp-controller:v1 \
    --dockerfile Dockerfile > deploy.yaml
```

**YAML output mode** (default) produces standard Kubernetes manifests suitable for `kubectl apply -f` or inclusion in Helm charts and kustomize bases.

**Script output mode** (`--output script`) produces a `.star` deployment script using `k8s.obj.*` constructors that you can customize further.

**Dockerfile** is generated only when `--dockerfile <name>` is provided (opt-in).

### configmap-sync.star

Watches ConfigMaps in a namespace and logs create/update/delete events.

```bash
kite-cloud run configmap-sync.star
kite-cloud run configmap-sync.star --var namespace=my-ns
```

Then in another terminal:
```bash
kubectl create configmap test --from-literal=key=value
kubectl delete configmap test
```

### deployment-scaler.star

Enforces a maximum replica count on deployments labeled `enforce-max-replicas=true`.

```bash
kite-cloud run deployment-scaler.star
kite-cloud run deployment-scaler.star --var max_replicas=5
```

Then in another terminal:
```bash
kubectl create deployment nginx --image=nginx --replicas=10
kubectl label deployment nginx enforce-max-replicas=true
# The controller will scale it down to 3 (or your configured max)
```

## How k8s.control() works

`k8s.control()` blocks the script (like `http.serve()`) and handles:

- **Automatic watch reconnection** with exponential backoff
- **Event deduplication** by resource namespace/name
- **Rate-limited retries** on handler errors
- **Graceful shutdown** on SIGTERM/SIGINT

You provide event handlers and the controller does the rest:

```python
k8s.control("resource-kind",
    on_create = fn(obj),           # called on ADDED events
    on_update = fn(old, new),      # called on MODIFIED events
    on_delete = fn(obj),           # called on DELETED events
    reconcile = fn(event, obj),    # catch-all handler
    namespace = "default",         # restrict to namespace
    labels = "app=myapp",          # label selector filter
    resync = "5m",                 # periodic re-reconcile
    workers = 1,                   # max concurrent handlers
)
```
