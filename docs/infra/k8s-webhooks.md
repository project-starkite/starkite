---
title: "Admission webhooks"
description: "Build validating and mutating admission webhooks with Starlark"
weight: 20
---

# Admission webhooks

Kubernetes admission webhooks intercept requests to the API server before resources are persisted in etcd. You can use webhooks to validate incoming resource fields (rejecting non-compliant requests) or mutate resource structures (injecting defaults).

The `k8s.webhook()` function starts an HTTPS server that receives `AdmissionReview` payloads, dispatches them to your Starlark handlers, and returns structured responses to the API server.

---

## Validating Webhooks

A validating admission webhook is an HTTP callback invoked by the Kubernetes API server during the admission phase. It intercepts incoming API requests—such as creating or updating a resource—and inspects the resource payload. Based on custom rules defined in your Starlark handler, the webhook either admits the request to be stored in etcd or rejects it, returning a custom error message to the user.

To reject a request, return a dictionary with `"allowed": False` and a descriptive `"message"`. To approve the request, return `{"allowed": True}`. If a handler raises a Starlark runtime error, the webhook automatically rejects the request and returns the error text as the message.

### Example: Validating a Required Label

The following webhook intercepts incoming deployments and rejects them if they do not contain a required `app` metadata label:

```python
# validate-label.star
# Rejects deployments missing the 'app' label

def validate(obj):
    # Retrieve labels using dot notation
    labels = obj.metadata.labels
    if labels == None or labels.get("app") == None:
        return {
            "allowed": False,
            "message": "The 'app' label is required for all deployments",
        }
    
    # Admit the deployment
    return {"allowed": True}

# Start the webhook listener on port 9443
k8s.webhook(
    path = "/validate-label",
    validate = validate,
    port = 9443,
    tls_cert = var_str("tls_cert", "/certs/tls.crt"),
    tls_key = var_str("tls_key", "/certs/tls.key"),
)
```

### Advanced Example: Validating Replicas and Ownership

For more complex validation policies, you can inspect resource specs and multiple metadata fields. The following webhook enforces a maximum replica limit of 10 and requires a `team` label to track resource ownership:

```python
# validate-deployments.star
# Validates deployment replicas and ownership labels

def validate(obj):
    # Verify replica limits
    replicas = obj.spec.replicas
    if replicas != None and replicas > 10:
        return {
            "allowed": False,
            "message": "Maximum of 10 replicas allowed, got %d" % replicas,
        }

    # Verify required ownership label
    labels = obj.metadata.labels
    if labels == None or labels.get("team") == None:
        return {
            "allowed": False,
            "message": "The 'team' label is required for all deployments",
        }

    # Admit the resource
    return {"allowed": True}

# Start the webhook listener
k8s.webhook(
    path = "/validate",
    validate = validate,
    port = 9443,
    tls_cert = var_str("tls_cert", "/certs/tls.crt"),
    tls_key = var_str("tls_key", "/certs/tls.key"),
)
```

---

## Mutating Webhooks

A mutating admission webhook intercepts requests during the admission phase and modifies the resource payload before it is validated and persisted. Mutating webhooks are commonly used to inject default metadata, enforce sidecar containers, or automatically populate configuration parameters. 

In Starkite, your handler receives the incoming object as a mutable Starlark dictionary, modifies it, and returns the updated object. The runtime automatically calculates the RFC 6902 JSON patch representing these modifications and returns it to the API server.

### Example: Injecting Default Labels

The following mutating webhook intercepts deployments and automatically injects a `managed-by: starkite` label before the resource is admitted:

```python
# mutate-deployments.star
# Automatically injects labels on admission

def mutate(obj):
    # Ensure the labels dictionary exists
    if obj["metadata"].get("labels") == None:
        obj["metadata"]["labels"] = {}
        
    # Inject the management label
    obj["metadata"]["labels"]["managed-by"] = "starkite"
    
    # Return the mutated object
    return obj

# Start the mutating webhook listener
k8s.webhook(
    path = "/mutate",
    mutate = mutate,
    port = 9443,
    tls_cert = var_str("tls_cert", "/certs/tls.crt"),
    tls_key = var_str("tls_key", "/certs/tls.key"),
)
```

---

## Object Access Semantics

When writing webhook handlers, it is critical to understand how Starkite represents and accesses Kubernetes objects:
* **Read-Only Operations**: Use **dot notation** (e.g., `obj.spec.replicas`) for clean, null-safe field traversal (common in validating webhooks).
* **Write/Mutation Operations**: Use **bracket notation** (e.g., `obj["metadata"]["labels"]["env"] = "prod"`) to modify fields in-place (required in mutating webhooks).

For a complete reference on object structures and access behaviors, see the [Object representation](k8s-objects.md) guide.

---

## Deploying Webhooks

Because admission webhooks run in the critical path of Kubernetes API operations, they must meet strict operational requirements:
1. **HTTPS/TLS Enforced**: The Kubernetes API server *strictly* requires HTTPS. Your webhook server must load valid TLS certificates and private keys to handle connections.
2. **Network Reachability**: The cluster's API server must be able to reach your webhook port. When running in production, this is usually accomplished via a cluster `Service` exposing your deployment.

### Local Testing

To test a webhook locally before deploying it to the cluster, generate a self-signed TLS certificate and run the script using `kite run`:

```bash
# Generate temporary TLS certificates
openssl req -x509 -newkey rsa:2048 \
    -keyout /tmp/key.pem -out /tmp/cert.pem \
    -days 1 -nodes -subj '/CN=localhost'

# Run the validating webhook locally using the certificates
kite run ./validate-deployments.star \
    --permissions=allow-local \
    --var tls_cert=/tmp/cert.pem \
    --var tls_key=/tmp/key.pem
```

> [!NOTE]
> To receive requests from a remote cluster while running locally, you must expose your local port (e.g., `9443`) using a secure tunnel or port-forwarding tool that the API server can access.

### Running Webhooks in Production

To run your webhook continuously in a production cluster, you must package the script as a deployment and register it with the Kubernetes admission controller. The Starkite CLI automates this setup by generating all required Kubernetes resources (Namespace, ServiceAccount, Deployment, Service, Secret placeholder, and the corresponding `ValidatingWebhookConfiguration` or `MutatingWebhookConfiguration`).

To generate the manifests, use `kite kube gen-webhook-artifacts`:

```bash
# Generate all deployment, service, and webhook configuration manifests
kite kube gen-webhook-artifacts \
    --webhook ./validate-deployments.star \
    --name alice-webhook \
    --image myregistry/validate-deployments:v1 \
    --namespace alice-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml
```

Deploy the generated manifest stream directly to your cluster:

```bash
# Apply the manifests to run the webhook in-cluster
kubectl apply -f deploy.yaml
```

The webhook will start running in the `alice-system` namespace. The Kubernetes API server will now automatically route all deployment creation and update requests to your Starlark handler for validation.
