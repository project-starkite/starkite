---
title: "Webhooks"
description: "Validating and mutating admission webhooks built with k8s.webhook()"
weight: 70
---

# Webhooks

Kubernetes admission webhooks intercept API requests before resources are persisted. `k8s.webhook()` starts an HTTPS server that receives `AdmissionReview` requests, hands the resource to a Starlark handler, and returns the verdict — for validation, an `allowed: true/false` response; for mutation, an RFC 6902 JSON patch generated automatically by diffing the original and modified objects.

Two example scripts demonstrate the validate and mutate sides. Both block the script (like `http.serve()` and `k8s.control()`).

## Validating webhook

Reject Deployments with too many replicas or missing labels.

**Source:** [`examples/cloud/webhook/validate-replicas.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/webhook/validate-replicas.star)

```python
#!/usr/bin/env kite

def validate(obj):
    replicas = obj.spec.replicas
    if replicas != None and replicas > 10:
        return {"allowed": False, "message": "max 10 replicas allowed, got %d" % replicas}

    labels = obj.metadata.labels
    if labels == None or labels.get("team") == None:
        return {"allowed": False, "message": "team label is required"}

    return {"allowed": True}

tls_cert = var_str("tls_cert", "/certs/tls.crt")
tls_key  = var_str("tls_key",  "/certs/tls.key")

k8s.webhook("/validate",
    validate = validate,
    port     = 9443,
    tls_cert = tls_cert,
    tls_key  = tls_key,
)
```

The handler returns a dict. `{"allowed": True}` admits the request; `{"allowed": False, "message": "..."}` rejects with the message surfaced to the API client. Raising an error is treated as `allowed: False` with the error text as the message.

## Mutating webhook

Inject default labels into every Deployment.

**Source:** [`examples/cloud/webhook/mutate-labels.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/webhook/mutate-labels.star)

```python
#!/usr/bin/env kite

def mutate(obj):
    labels = obj["metadata"]["labels"]
    labels["managed-by"] = "starkite"
    return obj

tls_cert = var_str("tls_cert", "/certs/tls.crt")
tls_key  = var_str("tls_key",  "/certs/tls.key")

k8s.webhook("/mutate",
    mutate   = mutate,
    port     = 9443,
    tls_cert = tls_cert,
    tls_key  = tls_key,
)
```

The handler receives the resource as a mutable `AttrDict`, modifies it in place using bracket notation, and returns it. The webhook diffs the original and modified objects and emits an RFC 6902 JSON patch back to the API server.

## Object access

Objects passed to handlers are `AttrDict` values. Reads use dot-access; writes use bracket-access:

```python
def handler(obj):
    name = obj.metadata.name                    # read
    image = obj.spec.containers[0].image        # nested + list

    obj["metadata"]["labels"]["env"] = "prod"   # write
    obj["spec"]["replicas"] = 3
    return obj
```

Nested AttrDicts share state with the parent — `labels = obj.metadata.labels; labels["k"] = "v"` modifies `obj` in place.

## Run locally

Generate a self-signed cert for testing:

```bash
openssl req -x509 -newkey rsa:2048 \
    -keyout /tmp/key.pem -out /tmp/cert.pem \
    -days 1 -nodes -subj '/CN=localhost'

kite run ./examples/cloud/webhook/validate-replicas.star \
    --var tls_cert=/tmp/cert.pem --var tls_key=/tmp/key.pem
```

For production, use [cert-manager](https://cert-manager.io/) to issue and rotate certificates automatically.

## Generate deployment artifacts

`kite kube gen-webhook-artifacts` produces the full set of manifests (Namespace, ServiceAccount, Deployment with TLS volume, Service `443 → 9443`, TLS Secret placeholder, and a `ValidatingWebhookConfiguration` or `MutatingWebhookConfiguration`):

```bash
kite kube gen-webhook-artifacts \
    --webhook examples/cloud/webhook/validate-replicas.star \
    --name myapp-webhook \
    --image myregistry/myapp-webhook:v1 \
    --namespace myapp-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml

kubectl apply -f deploy.yaml
```

`--rule` keys: `group` (API group, omit for core, `*` for all), `version` (`v1`, omit for all), `resource` (resource type), `operations` (`CREATE`, `UPDATE`, `DELETE`, `CONNECT`, `*`). Repeat `--rule` for multiple match rules.

## Combining validation and mutation

When both `validate` and `mutate` are passed to the same `k8s.webhook()` call, validation runs first; mutation is skipped if validation rejects.

## See also

- [`k8s` reference](../references/api/k8s.md) — `webhook`, `control`, AttrDict semantics
- [`kite kube` reference](../references/cli/kube.md) — `gen-webhook-artifacts` flags
- [Controllers](controllers.md) — same blocking-server pattern, watch-driven instead of HTTP-driven
