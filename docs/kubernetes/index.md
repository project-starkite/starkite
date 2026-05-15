---
title: "Kubernetes"
description: "Build controllers, webhooks, and manifest workflows in starkite"
weight: 50
---

# Kubernetes

Starkite's cloud edition ships the `k8s` module — full Kubernetes resource management, controller-runtime, and admission webhooks — plus the `kite kube` subcommand for artifact generation. Install with `kitecmd edition use cloud`, or use the all-in-one `kite` binary.

<div class="grid cards" markdown>

-   :material-cog-sync:{ .lg .middle } __Controllers__

    ---

    Write reconcile loops, owner references, leader election, and status updates with `k8s.control()`.

    [:octicons-arrow-right-24: Read more](guides/controllers.md)

-   :material-shield-check:{ .lg .middle } __Admission webhooks__

    ---

    Validating and mutating webhooks with RFC 6902 patch generation via `k8s.webhook()`.

    [:octicons-arrow-right-24: Read more](guides/webhooks.md)

-   :material-api:{ .lg .middle } __`k8s` API reference__

    ---

    The full three-tier API — CRUD primitives, `kubectl`-equivalents, and typed constructors.

    [:octicons-arrow-right-24: Read more](../references/api/k8s.md)

-   :material-folder-open:{ .lg .middle } __Examples__

    ---

    Runnable `.star` files demonstrating working patterns — deployments, services, controllers, more.

    [:octicons-arrow-right-24: Browse](examples.md)

</div>

## Example operations

```python
pods = k8s.list("pod", namespace="default")
for pod in pods:
    print(pod["metadata"]["name"])

k8s.deploy("nginx", "nginx:latest", replicas=3, port=80)

manifest = yaml.encode({
    "apiVersion": "v1",
    "kind": "ConfigMap",
    "metadata": {"name": "my-config", "namespace": "default"},
    "data": {"key": "value"},
})
k8s.apply(manifest)

k8s.scale("deployment", "nginx", 5)
k8s.rollout("deployment", "nginx", action="restart")
```

Full API surface: [`k8s` reference](../references/api/k8s.md).

## Install the cloud edition

```bash
# From source — produces ./bin/kitecloud
make build-cloud

# Or via the edition manager (downloads from GitHub Releases)
kitecmd edition use cloud
```

If you already have the all-in-one `kite` binary, the cloud module is bundled in — no separate install needed. See [Editions](../concepts/editions.md) for the full edition model and switching commands.
