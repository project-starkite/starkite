---
title: "Controllers"
description: "Writing Kubernetes controllers in starkite"
weight: 20
---

# Controllers

`k8s.control()` exposes the controller-runtime patterns — reconcile loops, owner references, leader election, status subresources, owned-resource watches — as a Starlark API. A starkite controller is a single `.star` script with reconciler functions for `on_create`, `on_update`, `on_delete`, and (optionally) `reconcile`.

!!! info "Coming soon"
    A worked end-to-end example (CRD definition, controller script, kustomize manifests via `kite kube gen-controller-artifacts`) is in progress.

## See also

- [`k8s` API reference](../../references/api/k8s.md)
- [`kite kube`](../../references/cli/kube.md) — generate controller and webhook artifacts from your script
- [Admission webhooks](webhooks.md)
