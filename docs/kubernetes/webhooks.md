---
title: "Webhooks"
description: "Validating and mutating admission webhooks built with k8s.webhook()"
weight: 70
---

# Webhooks

Sometimes you need the cluster to enforce a rule before a resource is ever stored — reject a Deployment that asks for too many replicas, or stamp a default label onto everything that comes through. That is an admission webhook: Kubernetes pauses each API request just before it persists the resource and asks your server for a verdict. With `k8s.webhook()` you write that server as a Starlark handler instead of a Go service.

The mechanism is the same whichever side you write. `k8s.webhook()` starts an HTTPS server that receives `AdmissionReview` requests, hands the resource to your handler, and turns what the handler returns into a response the API server understands. A validating handler returns a verdict — `allowed: true` or `allowed: false` — and a mutating handler returns a modified object, which the webhook diffs against the original to produce the RFC 6902 JSON patch the API server applies. Like `http.serve()` and `k8s.control()`, the call blocks: it runs the server until interrupted.

## Validating webhook

Start with the side that says yes or no. A validating handler inspects the resource and decides whether to admit it — here, rejecting Deployments that ask for too many replicas or omit a required label.

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

The verdict is the dict the handler returns. `{"allowed": True}` admits the request; `{"allowed": False, "message": "..."}` rejects it, and the message surfaces to the API client as the reason. If the handler raises an error instead of returning, the webhook treats that as `allowed: False` with the error text as the message — a failed assertion or an unexpected `None` becomes a rejection rather than a crash.

## Mutating webhook

When you need to change a resource rather than judge it, switch to a mutating handler. It receives the object, edits it, and returns it — here, injecting a default label onto every Deployment.

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

You never construct the patch yourself. The handler receives the resource as a mutable `AttrDict`, edits it in place with bracket notation, and returns it; the webhook diffs the returned object against the original it sent in and emits the RFC 6902 JSON patch back to the API server. You describe the desired end state, and the wire format is derived for you.

## Object access

Both handlers work on the same kind of value, so it pays to know how it reads and writes. Objects passed to handlers are `AttrDict` values, and they expose two access styles: dot-access for reading and bracket-access for writing.

```python
def handler(obj):
    name = obj.metadata.name                    # read
    image = obj.spec.containers[0].image        # nested + list

    obj["metadata"]["labels"]["env"] = "prod"   # write
    obj["spec"]["replicas"] = 3
    return obj
```

Dot-access reaches through nested maps and list indices, so you read deep paths without quoting every key. The catch worth knowing is that nested AttrDicts share state with their parent: bind `labels = obj.metadata.labels`, write `labels["k"] = "v"`, and you have modified `obj` itself. That is what makes in-place mutation work, but it also means a handle you took for reading can change the object you return.

## Run locally

Before you deploy anything, you can run a webhook on your own machine to check the logic. The one prerequisite is TLS — the API server only talks to webhooks over HTTPS — so generate a throwaway certificate and point the script at it:

```bash
openssl req -x509 -newkey rsa:2048 \
    -keyout /tmp/key.pem -out /tmp/cert.pem \
    -days 1 -nodes -subj '/CN=localhost'

kite run ./examples/cloud/webhook/validate-replicas.star \
    --var tls_cert=/tmp/cert.pem --var tls_key=/tmp/key.pem
```

That self-signed pair is fine for a local check, but it expires in a day and no cluster will trust it in production. For a real deployment, let [cert-manager](https://cert-manager.io/) issue and rotate the certificate so you are not minting and distributing keys by hand.

## Generate deployment artifacts

A running script is only half of a deployed webhook — the cluster also needs the Deployment, Service, Secret, and the configuration that tells the API server to call you. Rather than hand-write that stack, `kite kube gen-webhook-artifacts` produces the full set of manifests (Namespace, ServiceAccount, Deployment with TLS volume, Service `443 → 9443`, TLS Secret placeholder, and a `ValidatingWebhookConfiguration` or `MutatingWebhookConfiguration`):

```bash
kite kube gen-webhook-artifacts \
    --webhook examples/cloud/webhook/validate-replicas.star \
    --name myapp-webhook \
    --image myregistry/myapp-webhook:v1 \
    --namespace myapp-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml

kubectl apply -f deploy.yaml
```

The `--rule` flag is what scopes the webhook to the requests it should see. Its keys are `group` (API group, omit for core, `*` for all), `version` (`v1`, omit for all), `resource` (resource type), and `operations` (`CREATE`, `UPDATE`, `DELETE`, `CONNECT`, `*`). Repeat `--rule` to match more than one kind of request — a webhook with no matching rule is never called, so this is where you decide what it guards.

## Combining validation and mutation

You do not need two servers to do both jobs. Pass `validate` and `mutate` to the same `k8s.webhook()` call and one server runs both: validation runs first, and if it rejects the request, mutation is skipped. The resource is never modified on its way to being denied.

## See also

- [`k8s` reference](../references/api/k8s.md) — `webhook`, `control`, AttrDict semantics
- [`kite kube` reference](../references/cli/kube.md) — `gen-webhook-artifacts` flags
- [Controllers](controllers.md) — same blocking-server pattern, watch-driven instead of HTTP-driven
