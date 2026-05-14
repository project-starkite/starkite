---
title: "Admission Webhooks"
description: "Building Kubernetes admission webhooks with starkite"
weight: 8
---

Kubernetes admission webhooks intercept API requests before resources are persisted. Starkite makes it simple to build validating and mutating webhooks with `k8s.webhook()`.

## Quick Start

### Validating Webhook

Reject deployments with too many replicas:

```python
#!/usr/bin/env kitecloud

def validate(obj):
    if obj.spec.replicas > 10:
        return {"allowed": False, "message": "max 10 replicas allowed"}
    return {"allowed": True}

k8s.webhook("/validate",
    validate = validate,
    port = 9443,
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

### Mutating Webhook

Inject labels into every deployment:

```python
#!/usr/bin/env kitecloud

def mutate(obj):
    obj["metadata"]["labels"]["managed-by"] = "starkite"
    obj["metadata"]["annotations"]["mutated-at"] = time.format(time.now(), time.RFC3339)
    return obj

k8s.webhook("/mutate",
    mutate = mutate,
    port = 9443,
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

## How It Works

`k8s.webhook()` starts an HTTPS server that:

1. Receives `AdmissionReview` requests from the Kubernetes API server
2. Extracts the resource object and converts it to an AttrDict (dot-access + bracket-access)
3. Calls your handler function with the object
4. For validation: returns `allowed: true/false` based on your handler's return value
5. For mutation: diffs the original and modified objects to generate an RFC 6902 JSON patch
6. Returns the `AdmissionReview` response to the API server

The function blocks until terminated (like `http.serve()` and `k8s.control()`).

## Object Access

Objects are passed as mutable AttrDicts. Use dot-notation for reading and bracket-notation for writing:

```python
def mutate(obj):
    # Read with dot-access
    name = obj.metadata.name
    ns = obj.metadata.namespace
    replicas = obj.spec.replicas

    # Read nested values
    labels = obj.metadata.labels
    image = obj.spec.containers[0].image  # list items are also AttrDicts

    # Write with bracket-access
    obj["metadata"]["labels"]["env"] = "production"
    obj["metadata"]["annotations"]["version"] = "v2"

    # Dot-access returns a nested AttrDict — bracket writes on it propagate
    labels = obj.metadata.labels
    labels["team"] = "platform"  # this modifies obj.metadata.labels

    return obj
```

## Handler Contracts

### Validate Handler

```python
def validate(obj):
    # obj: AttrDict — the resource being admitted
    # Return one of:
    #   {"allowed": True}
    #   {"allowed": False, "message": "reason for rejection"}
```

If the handler raises an error, the webhook returns `allowed: false` with the error message.

### Mutate Handler

```python
def mutate(obj):
    # obj: AttrDict — the resource being admitted (mutable)
    # Modify the object in place using bracket notation
    # Return the modified object
    # Changes are automatically converted to an RFC 6902 JSON patch
```

If the handler raises an error, the webhook returns `allowed: false` with the error message.

### Combined Validation and Mutation

When both `validate` and `mutate` are provided, validation runs first. If validation rejects the request, mutation is skipped:

```python
def validate(obj):
    if not obj.metadata.labels.get("team"):
        return {"allowed": False, "message": "team label required"}
    return {"allowed": True}

def mutate(obj):
    obj["metadata"]["labels"]["validated"] = "true"
    return obj

k8s.webhook("/webhook",
    validate = validate,
    mutate = mutate,
    port = 9443,
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

## TLS Certificates

Webhooks require TLS. For local testing, generate a self-signed certificate:

```bash
openssl req -x509 -newkey rsa:2048 \
    -keyout /tmp/key.pem -out /tmp/cert.pem \
    -days 1 -nodes -subj '/CN=localhost'

kitecloud run webhook.star \
    --var tls_cert=/tmp/cert.pem \
    --var tls_key=/tmp/key.pem
```

For production, use [cert-manager](https://cert-manager.io/) to manage certificates automatically.

## Deploying to Kubernetes

A webhook deployment requires:

1. **Deployment** — runs `kitecloud run webhook.star` with TLS certs mounted
2. **Service** — exposes the webhook pod (port 443 → 9443)
3. **TLS Secret** — certificate and private key
4. **WebhookConfiguration** — tells the API server which resources to intercept

### Generate Deployment Artifacts

Use `kitecloud kube gen-webhook-artifacts` to generate all manifests:

```bash
# Generate YAML manifests
kitecloud kube gen-webhook-artifacts \
    --webhook webhook.star \
    --name myapp-webhook \
    --image myregistry/myapp-webhook:v1 \
    --namespace myapp-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml

# Or generate a Starlark deployment script
kitecloud kube gen-webhook-artifacts \
    --webhook webhook.star \
    --name myapp-webhook \
    --image myregistry/myapp-webhook:v1 \
    --namespace myapp-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" \
    --output script > deploy-webhook.star
```

### The `--rule` Flag

Specifies which resources the webhook intercepts, using key=value pairs:

```bash
# Deployments in the apps group
--rule "group=apps resource=deployments operations=CREATE,UPDATE"

# Pods in the core group (omit group)
--rule "resource=pods operations=CREATE"

# Multiple rules
--rule "group=apps resource=deployments operations=CREATE,UPDATE" \
--rule "resource=pods operations=CREATE"
```

Keys:
- `group` — API group (`apps`, `batch`, omit for core, `*` for all)
- `version` — API version (`v1`, omit for all)
- `resource` — resource type (`deployments`, `pods`)
- `operations` — comma-separated: `CREATE`, `UPDATE`, `DELETE`, `CONNECT`, `*`

### Apply to Cluster

```bash
# Build and push image
docker build -t myregistry/myapp-webhook:v1 .
docker push myregistry/myapp-webhook:v1

# Apply manifests
kubectl apply -f deploy.yaml

# Update TLS secret with real certificates
kubectl create secret tls myapp-webhook-tls \
    --cert=tls.crt --key=tls.key \
    -n myapp-system --dry-run=client -o yaml | kubectl apply -f -
```

## Examples

See [`examples/cloud/webhook/`](https://github.com/project-starkite/starkite/tree/main/examples/cloud/webhook) for complete working examples:

- `validate-replicas.star` — reject deployments with too many replicas
- `mutate-labels.star` — inject default labels into deployments
