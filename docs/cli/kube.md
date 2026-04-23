---
title: "kite kube"
description: "Kubernetes-specific commands (cloud edition)"
weight: 25
---

!!! note "Cloud edition only"
    `kite kube` is available only in the `kite-cloud` edition. Install via `kite edition use cloud` or run the `kite-cloud` binary directly.

Generate Kubernetes deployment artifacts for starkite controllers and admission webhooks. Two subcommands: `gen-controller-artifacts` builds manifests for long-running controllers; `gen-webhook-artifacts` builds manifests for validating/mutating webhooks including TLS plumbing.

## Subcommands

| Subcommand | Purpose |
|------------|---------|
| `kite kube gen-controller-artifacts` | Generate controller deployment manifests |
| `kite kube gen-webhook-artifacts` | Generate webhook deployment manifests |

---

## `kite kube gen-controller-artifacts`

Evaluate a Starlark generator script that builds manifests via `k8s.obj.*` constructors. The default built-in generator produces:

- CustomResourceDefinition (when `--resource` is provided)
- Namespace, ServiceAccount, ClusterRole, ClusterRoleBinding
- Deployment

Output can be YAML (for `kubectl apply`) or a generated Starlark deployment script.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--controller <path>` | `controller.star` | Path to controller script |
| `--resource <path>` | (none) | Path to resource definition script (for CRD generation) |
| `--image <ref>` | **required** | Container image for the controller |
| `--namespace <ns>` | `default` | Namespace for the controller deployment |
| `--replicas <n>` | `1` | Number of controller replicas |
| `--output <format>` | `yaml` | Output format: `yaml` or `script` |
| `--dockerfile <name>` | (none) | Also generate a Dockerfile with the given name |
| `--generator <path>` | (none) | Custom generator script, overrides the built-in |

### Examples

```bash
# Generate YAML manifests (default)
kite-cloud kube gen-controller-artifacts \
    --controller controller.star \
    --resource resource.star \
    --image myregistry/myapp-controller:v1 \
    --namespace myapp-system > deploy.yaml

# Generate a Starlark deployment script
kite-cloud kube gen-controller-artifacts \
    --controller controller.star \
    --image myregistry/myapp-controller:v1 \
    --output script > deploy-controller.star

# Use a custom generator
kite-cloud kube gen-controller-artifacts \
    --generator my-generator.star \
    --image myregistry/myapp-controller:v1
```

---

## `kite kube gen-webhook-artifacts`

Produce full admission-webhook manifests: Namespace, ServiceAccount, Deployment (with TLS volume), Service (`443 → 9443`), TLS Secret placeholder, and a `ValidatingWebhookConfiguration` or `MutatingWebhookConfiguration`.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--webhook <path>` | `webhook.star` | Path to webhook script |
| `--name <string>` | **required** | Webhook name |
| `--image <ref>` | **required** | Container image |
| `--namespace <ns>` | `default` | Namespace for deployment |
| `--rule <string>` | (none, repeatable) | Admission rule, e.g. `"group=apps resource=deployments operations=CREATE,UPDATE"` |
| `--output <format>` | `yaml` | Output format: `yaml` or `script` |
| `--dockerfile <name>` | (none) | Generate a Dockerfile with the given name |
| `--generator <path>` | (none) | Custom generator script |

### Examples

```bash
kite-cloud kube gen-webhook-artifacts \
    --webhook webhook.star \
    --name myapp-webhook \
    --image myregistry/myapp-webhook:v1 \
    --namespace myapp-system \
    --rule "group=apps resource=deployments operations=CREATE,UPDATE" > deploy.yaml
```

## Related

- [Building webhooks guide](../guides/webhooks.md)
- [k8s module reference](../modules/k8s.md)
