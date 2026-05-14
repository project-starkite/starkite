---
title: "Write a Kubernetes controller"
description: "Writing Kubernetes controllers in starkite"
weight: 20
---

# Write a Kubernetes controller

`k8s.control()` exposes controller-runtime patterns — reconcile loops, owner references, leader election, status subresources, owned-resource watches — as a Starlark API. A starkite controller is a `.star` script with reconciler functions for `on_create`, `on_update`, `on_delete`, and optionally `reconcile`.

## See also

- [`k8s` API reference](../../references/api/k8s.md)
- [`kite kube`](../../references/cli/kube.md) — generate controller and webhook artifacts from a script
- [Admission webhooks](webhooks.md)
