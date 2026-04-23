# Rolling Update

Image update with watch-based health monitoring and automatic rollback — a
realistic CI/CD deployment pattern.

## What it demonstrates

- **Tier 2**: `k.set_image()`, `k.rollout()`
- **Tier 1**: `k.get()`, `k.watch()`, `k.wait_for()`
- Pre-flight validation (current vs. target image, deployment health)
- Real-time pod watching during rollout with crash-loop detection
- Automatic rollback on failure

## Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `image` | **Yes** | — | New container image (e.g. `myapp:v2`) |
| `namespace` | No | `default` | Target namespace |
| `deploy.name` | No | `web` | Deployment name |
| `container` | No | same as deploy name | Container name within the pod |
| `watch.timeout` | No | `180` | Seconds to watch pods after image update |
| `max.restarts` | No | `2` | Max container restarts before triggering rollback |

## Usage

```bash
# Update a deployment's image
kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2

# Update in production with extended watch
kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2 --var namespace=production

# Custom timeout and restart tolerance
kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2 --var watch.timeout=300 --var max.restarts=3
```

## What it creates

No new resources — operates on an **existing** Deployment.

## How it works

1. **Pre-flight**: fetches the current Deployment, checks that the target image differs, and warns if the deployment is already degraded
2. **Update**: calls `k.set_image()` to patch the container image
3. **Watch**: sets up a pod watcher that monitors for CrashLoopBackOff, ImagePullBackOff, and excessive restarts during the rollout window
4. **Evaluate**: if errors were detected, prints them and initiates `k.rollout("undo")` to roll back, then waits for the rollback to complete
5. **Verify**: on success, checks `k.rollout("status")` to confirm all replicas are ready
