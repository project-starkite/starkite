---
title: "Rolling update with health watch"
description: "Image update with watch-based health monitoring and automatic rollback"
weight: 20
---

# Rolling update with health watch

A realistic CI/CD deployment pattern: pre-flight check the current state, push the new image, watch pods for crash-loops or `ImagePullBackOff` during the rollout, and automatically roll back if anything goes wrong. The script combines Tier 2 (`set_image`, `rollout`) with Tier 1 (`watch`) — it's how you'd build a deployment-promote step in a CI pipeline.

**Source:** [`examples/cloud/rolling-update/rolling-update.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/rolling-update/rolling-update.star)

## Watch handler — the core pattern

```python
def make_watcher(errors, max_restarts):
    """Return a watch handler that tracks pod health during rollout."""
    def on_pod_event(event_type, pod):
        if event_type == "DELETED":
            return  # normal during rolling update

        for cs in pod["status"].get("containerStatuses", []):
            restarts = cs.get("restartCount", 0)
            reason = cs.get("state", {}).get("waiting", {}).get("reason", "")

            if restarts > max_restarts:
                errors.append("container %s in %s restarted %d times" % (
                    cs["name"], pod["metadata"]["name"], restarts))
                return False  # stop watching

            if reason in ("CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull"):
                errors.append("%s in %s: %s" % (
                    cs["name"], pod["metadata"]["name"], reason))
                return False
    return on_pod_event
```

## Driving the update

```python
k = k8s.config(namespace=ns)

# Pre-flight: skip if already on the target image; warn if degraded
preflight(k, name, new_image)

# Push the image
k.set_image("deployment", name, container, new_image)

# Watch pods labeled app=<name>; stop on first crash-loop signal or timeout
errors = []
k.watch("pods",
    labels="app=%s" % name,
    handler=make_watcher(errors, max_restarts),
    timeout=watch_timeout)

# Auto-rollback on any error captured by the watch handler
if errors:
    k.rollout("deployment", name, action="undo")
    k.wait_for("deployment", name, condition="available", timeout="5m")
    fail("rolling update failed — rolled back")

# Otherwise verify the rollout completed cleanly
status = k.rollout("deployment", name, action="status")
if not status["complete"]:
    k.wait_for("deployment", name, condition="available", timeout="5m")
```

## Run it

```bash
kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2
kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2 --var namespace=production
kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2 --var watch.timeout=300
```

## What's happening

- **`k.watch("pods", labels=..., handler=fn, timeout=...)`** runs a label-filtered watch. The handler is called per event with `(event_type, object)`. Returning `False` from the handler stops the watch early.
- **`k.set_image(kind, name, container, image)`** patches the container image on a workload — the `kubectl set image` equivalent.
- **`k.rollout(kind, name, action="undo")`** rolls back to the previous revision. `action="status"` returns rollout progress.
- **`k.wait_for(kind, name, condition=..., timeout=...)`** blocks until the resource meets the condition.

## See also

- [`k8s` reference](../../references/api/k8s.md) — `watch`, `wait_for`, `rollout`, `set_image`
- [Deploy](deploy.md) — the simpler one-shot deploy without rollback logic
