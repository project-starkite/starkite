---
title: "Cloud commands (apply, deploy, drift)"
description: "Cloud edition commands for infrastructure operations"
weight: 26
---

!!! warning "Not yet implemented"
    The `apply`, `deploy`, and `drift` commands are reserved placeholders in the cloud edition. Invoking them currently returns `"<command>: not yet implemented"` and exits non-zero. Their intended behavior is documented below as a forward reference; do not rely on them in scripts until they ship.

!!! note "Cloud edition only"
    These commands exist only in the `kite-cloud` binary. Install via `kite edition use cloud` or run the `kite-cloud` binary directly.

The three commands are top-level shortcuts for common infrastructure workflows: applying manifests, deploying applications, and detecting drift from a desired state.

## `kite apply <manifest.star>`

Evaluate a starkite manifest and apply the resulting resources to your cloud environment (Kubernetes, cloud provider, etc.).

### Intended usage

```bash
kite apply infra.star
kite apply infra.star --var env=production
```

## `kite deploy <manifest.star>`

Orchestrate a full deployment: build, push, and rollout.

### Intended usage

```bash
kite deploy app.star
kite deploy app.star --var image_tag=v1.2.0
```

## `kite drift [manifest.star]`

Compare the desired state defined in a manifest against the actual state of deployed resources and report differences.

### Intended usage

```bash
kite drift infra.star
kite drift infra.star --output yaml
```

## In the meantime

Equivalent operations are available today via the [`k8s` module](../modules/k8s.md) directly:

- `k8s.apply(manifest)` applies a resource
- `k8s.deploy(name, image, ...)` creates a Deployment + Service
- `k8s.get(kind, name)` + user-written diff for drift detection

Once the top-level commands land, this page will be updated to reflect the real flags and behavior.
