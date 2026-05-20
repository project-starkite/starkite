---
title: "Kubernetes"
description: "Build controllers, webhooks, and manifest workflows in starkite"
weight: 50
---

# Kubernetes

Starkite's cloud edition ships the `k8s` module — full Kubernetes resource management, controller-runtime, and admission webhooks — plus the `kite kube` subcommand for artifact generation. Install with `kitecmd edition use cloud`, or use the all-in-one `kite` binary.

<div class="grid cards" markdown>

-   :material-rocket-launch:{ .lg .middle } __Deploy & manage__

    ---

    Day-to-day patterns: deploy + scale, rolling updates with health watch, cluster health audits, per-environment manifest generation, full app stack composition.

    [:octicons-arrow-right-24: Read more](quickstarts/deploy.md)

-   :material-cog-sync:{ .lg .middle } __Controllers__

    ---

    Reconcile loops, label-filtered watches, and periodic resync with `k8s.control()`.

    [:octicons-arrow-right-24: Read more](quickstarts/controllers.md)

-   :material-shield-check:{ .lg .middle } __Admission webhooks__

    ---

    Validating and mutating webhooks with RFC 6902 patch generation via `k8s.webhook()`.

    [:octicons-arrow-right-24: Read more](quickstarts/webhooks.md)

-   :material-api:{ .lg .middle } __`k8s` API reference__

    ---

    The full three-tier API — CRUD primitives, `kubectl`-equivalents, and typed constructors.

    [:octicons-arrow-right-24: Read more](../references/api/k8s.md)

</div>

## Install the cloud edition

```bash
# From source — produces ./bin/kitecloud
make build-cloud

# Or via the edition manager (downloads from GitHub Releases)
kitecmd edition use cloud
```

If the all-in-one `kite` binary is already installed, the cloud module is bundled in — no separate install needed. See [Editions](../concepts/editions.md) for the full edition model and switching commands.
