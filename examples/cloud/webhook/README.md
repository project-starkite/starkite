# Webhook Examples

Kubernetes admission webhook examples using `k8s.webhook()`.

## Prerequisites

- A running Kubernetes cluster (e.g., `kind create cluster`)
- TLS certificates (self-signed for testing, cert-manager for production)
- The `cloudkite` binary

## Examples

### validate-replicas.star

Rejects deployments with more than 10 replicas and requires a `team` label.

```bash
# Generate self-signed certs for testing
openssl req -x509 -newkey rsa:2048 -keyout /tmp/key.pem -out /tmp/cert.pem \
    -days 1 -nodes -subj '/CN=localhost'

# Run locally
cloudkite run validate-replicas.star \
    --var tls_cert=/tmp/cert.pem \
    --var tls_key=/tmp/key.pem
```

### mutate-labels.star

Injects a `managed-by: starkite` label into all deployments.

```bash
cloudkite run mutate-labels.star \
    --var tls_cert=/tmp/cert.pem \
    --var tls_key=/tmp/key.pem
```

## Generating deployment artifacts

```bash
# Generate YAML manifests
cloudkite kube gen-webhook-artifacts \
    --webhook validate-replicas.star \
    --name replicas-webhook \
    --image myregistry/replicas-webhook:v1 \
    --namespace webhooks \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml

# Generate Starlark deployment script
cloudkite kube gen-webhook-artifacts \
    --webhook validate-replicas.star \
    --name replicas-webhook \
    --image myregistry/replicas-webhook:v1 \
    --output script > deploy-webhook.star
```

## How k8s.webhook() works

`k8s.webhook()` blocks the script (like `http.serve()`) and serves an HTTPS endpoint that handles Kubernetes AdmissionReview requests.

```python
k8s.webhook("/path",
    validate = fn(obj),      # validation handler
    mutate = fn(obj),        # mutation handler
    port = 9443,             # HTTPS port
    tls_cert = "/certs/tls.crt",
    tls_key = "/certs/tls.key",
)
```

Objects are passed as AttrDicts with dot-access: `obj.metadata.name`, `obj.spec.replicas`.
