---
title: "Controlling a cluster"
description: "Build Kubernetes controllers and admission webhooks with Starlark"
weight: 30
---

# Controlling a cluster

To build active automation on a cluster, Starkite supports two programmability models: **Reconcile Loops (Controllers)** that react to cluster events in the background, and **Admission Webhooks** that intercept and inspect or mutate requests before they are persisted.

---

## Reconcile loops (Controllers)

A controller is how you keep a cluster converging on the state you want long after a one-shot script would have exited. You describe what a resource should look like, and the controller watches that resource and corrects it whenever reality drifts. In Starkite you write one with `k8s.control()`: a `.star` script that registers a handler and then hands control to the runtime, which drives a watch-based reconcile loop for as long as the process lives.

That shape is blocking. `k8s.control()` does not return — like `http.serve()`, it parks the script and becomes the program. Events arrive from the Kubernetes API server, deduplicate per resource, and dispatch to your handler; the runtime owns the parts you would otherwise rewrite by hand — watch reconnection, exponential backoff, rate-limited retries, and clean shutdown on SIGTERM or SIGINT.

The example below watches Deployments labeled `enforce-max-replicas=true`, and whenever one exceeds the configured maximum it scales it back down.

**Source:** [deployment-scaler.star](https://github.com/project-starkite/starkite/blob/main/examples/cloud/controller/deployment-scaler.star)

### Script

```python
#!/usr/bin/env kite
# deployment-scaler.star — enforces max replicas on labeled deployments

max_replicas = var_int("max_replicas", 3)

def reconcile(event, obj):
    if event == "DELETED":
        printf("[DELETED] %s/%s\n", obj.metadata.namespace, obj.metadata.name)
        return

    replicas = obj.spec.replicas
    if replicas == None:
        replicas = 1

    if replicas > max_replicas:
        printf("[SCALE DOWN] %s/%s from %d to %d replicas\n",
            obj.metadata.namespace, obj.metadata.name, replicas, max_replicas)
        k8s.apply({
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": obj.metadata.name,
                "namespace": obj.metadata.namespace,
            },
            "spec": {"replicas": max_replicas},
        })
    else:
        printf("[OK] %s/%s has %d replicas (max: %d)\n",
            obj.metadata.namespace, obj.metadata.name, replicas, max_replicas)

printf("Enforcing max %d replicas on deployments with label enforce-max-replicas=true...\n", max_replicas)
k8s.control("deployments",
    reconcile = reconcile,
    labels = "enforce-max-replicas=true",
    resync = "1m",
)
```

### Running the controller

Start the controller first; by default it enforces three, and `--var` lets you raise that:

```bash
kite run ./deployment-scaler.star
kite run ./deployment-scaler.star --var max_replicas=5
```

Now give it a violation to find. In a second shell, create a Deployment and apply the label:

```bash
kubectl create deployment nginx --image=nginx --replicas=10
kubectl label deployment nginx enforce-max-replicas=true
# the controller scales it down to max_replicas
```

### Handler signatures

`k8s.control()` accepts four handler kwargs:

| Handler | Signature | Called on |
|---|---|---|
| `on_create` | `fn(obj)` | `ADDED` events |
| `on_update` | `fn(old, new)` | `MODIFIED` events |
| `on_delete` | `fn(obj)` | `DELETED` events |
| `reconcile` | `fn(event, obj)` | catch-all (any event type as a string) |

### Control configuration

- **`labels = "k=v"`** narrows the watch to resources matching the selector.
- **`resync = "1m"`** re-fires `reconcile` on every known resource periodically.
- **`workers = N`** caps concurrent handler executions.
- **`namespace = "ns"`** restricts the watch to a single namespace.

### Generating deployment artifacts

For production, `kite kube gen-controller-artifacts` generates the Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding, and Deployment manifests:

```bash
kite kube gen-controller-artifacts \
    --controller ./deployment-scaler.star \
    --image myregistry/myapp-controller:v1 \
    --namespace myapp-system > deploy.yaml

kubectl apply -f deploy.yaml
```

---

## Admission Webhooks

Sometimes you need the cluster to enforce a rule before a resource is ever stored — reject a Deployment that asks for too many replicas, or stamp a default label onto everything that comes through. That is an admission webhook: Kubernetes pauses each API request just before it persists the resource and asks your server for a verdict. With `k8s.webhook()` you write that server as a Starlark handler instead of a Go service.

Like `k8s.control()`, the webhook server call is blocking. It starts an HTTPS server that receives `AdmissionReview` requests, hands the resource to your handler, and turns what the handler returns into a response the API server understands.

### Validating webhook

A validating handler inspects the resource and decides whether to admit it:

**Source:** [validate-replicas.star](https://github.com/project-starkite/starkite/blob/main/examples/cloud/webhook/validate-replicas.star)

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

The verdict is the dict the handler returns. `{"allowed": True}` admits the request; `{"allowed": False, "message": "..."}` rejects it. If the handler raises an error instead of returning, the webhook treats that as `allowed: False` with the error text as the message.

### Mutating webhook

When you need to change a resource rather than judge it, switch to a mutating handler. It receives the object, edits it, and returns it — here, injecting a default label onto every Deployment.

**Source:** [mutate-labels.star](https://github.com/project-starkite/starkite/blob/main/examples/cloud/webhook/mutate-labels.star)

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

The handler receives the resource as a mutable `AttrDict`, edits it in place with bracket notation, and returns it; the webhook diffs the returned object against the original and emits the RFC 6902 JSON patch back to the API server.

### Object access

Objects passed to handlers are `AttrDict` values, and they expose two access styles: dot-access for reading and bracket-access for writing:

```python
def handler(obj):
    name = obj.metadata.name                    # read
    image = obj.spec.containers[0].image        # nested + list

    obj["metadata"]["labels"]["env"] = "prod"   # write
    obj["spec"]["replicas"] = 3
    return obj
```

### Running the webhook locally

Before you deploy, you can run a webhook locally using throwaway self-signed TLS certificates (HTTPS is required by the API server):

```bash
openssl req -x509 -newkey rsa:2048 \
    -keyout /tmp/key.pem -out /tmp/cert.pem \
    -days 1 -nodes -subj '/CN=localhost'

kite run ./validate-replicas.star \
    --var tls_cert=/tmp/cert.pem --var tls_key=/tmp/key.pem
```

### Generating webhook deployment manifests

`kite kube gen-webhook-artifacts` produces the Namespace, ServiceAccount, Deployment, Service, Secret placeholder, and Webhook configuration:

```bash
kite kube gen-webhook-artifacts \
    --webhook ./validate-replicas.star \
    --name myapp-webhook \
    --image myregistry/myapp-webhook:v1 \
    --namespace myapp-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml

kubectl apply -f deploy.yaml
```

Pass `validate` and `mutate` to the same `k8s.webhook()` call to run both handlers on a single server.

---

## See also

- [`k8s` reference](../references/api/k8s.md) — `control`, `webhook`, AttrDict semantics, leader election
- [`kite kube` reference](../references/cli/kube.md) — `gen-controller-artifacts` and `gen-webhook-artifacts` flags
- More examples: [`examples/cloud/controller/`](https://github.com/project-starkite/starkite/tree/main/examples/cloud/controller) and [`examples/cloud/webhook/`](https://github.com/project-starkite/starkite/tree/main/examples/cloud/webhook)
