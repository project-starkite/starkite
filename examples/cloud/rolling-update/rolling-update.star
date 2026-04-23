#!/usr/bin/env kite
# rolling-update.star - Image update with watch-based health monitoring
#
# Demonstrates Tier 2 (set_image, rollout) combined with Tier 1 (watch)
# for a realistic CI/CD deployment: update the image, watch pods for
# crash loops during the rollout, auto-rollback on failure.
#
# Usage:
#   kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2
#   kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2 --var namespace=production
#   kite run examples/cloud/rolling-update/rolling-update.star --var image=myapp:v2 --var watch.timeout=300

def preflight(k, name, new_image):
    """Check current state before updating."""
    printf("Pre-flight: checking deployment/%s...\n", name)

    dep = k.get("deployment", name)
    current_image = dep["spec"]["template"]["spec"]["containers"][0]["image"]

    if current_image == new_image:
        printf("Already running %s — nothing to do.\n", new_image)
        fail("image unchanged")

    printf("  Current image: %s\n", current_image)
    printf("  Target image:  %s\n", new_image)

    desired = dep["spec"].get("replicas", 1)
    available = dep["status"].get("availableReplicas", 0)
    if available < desired:
        printf("  WARNING: deployment is already degraded (%d/%d available)\n", available, desired)

def make_watcher(errors, max_restarts):
    """Return a watch handler that tracks pod health during rollout."""
    # Handler receives (event_type, object) as two separate args
    def on_pod_event(event_type, pod):
        pod_name = pod["metadata"]["name"]
        phase = pod["status"].get("phase", "Unknown")

        if event_type == "DELETED":
            return  # normal during rolling update

        # Check for crash-looping containers
        for cs in pod["status"].get("containerStatuses", []):
            restarts = cs.get("restartCount", 0)
            waiting = cs.get("state", {}).get("waiting", {})
            reason = waiting.get("reason", "")

            if restarts > max_restarts:
                errors.append("container %s in %s restarted %d times" % (
                    cs["name"], pod_name, restarts))
                return False  # stop watching

            if reason in ("CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull"):
                errors.append("%s in %s: %s" % (cs["name"], pod_name, reason))
                return False  # stop watching

        if event_type == "MODIFIED" and phase == "Running":
            printf("  %s: running\n", pod_name)

    return on_pod_event

def main():
    ns = var_str("namespace", "default")
    name = var_str("deploy.name", "web")
    new_image = var_str("image")
    container = var_str("container", "") or name
    watch_timeout = var_str("watch.timeout", "3m")
    max_restarts = var_int("max.restarts", 2)

    k = k8s.config(namespace=ns)

    # --- Pre-flight checks -----------------------------------------------------
    preflight(k, name, new_image)

    # --- Update image ----------------------------------------------------------
    printf("\nUpdating %s -> %s...\n", container, new_image)
    k.set_image("deployment", name, container, new_image)

    # --- Watch for problems during rollout -------------------------------------
    printf("Watching pods for %s (max %d restarts allowed)...\n", watch_timeout, max_restarts)

    errors = []
    k.watch("pods", labels="app=%s" % name, handler=make_watcher(errors, max_restarts), timeout=watch_timeout)

    # --- Evaluate result -------------------------------------------------------
    if errors:
        printf("\nRollout FAILED:\n")
        for e in errors:
            printf("  - %s\n", e)

        printf("\nRolling back to previous revision...\n")
        k.rollout("deployment", name, action="undo")
        k.wait_for("deployment", name, condition="available", timeout="5m")
        printf("Rollback complete. Deployment is back on the previous version.\n")
        fail("rolling update failed — rolled back")

    # --- Verify rollout completed ----------------------------------------------
    printf("\nVerifying rollout status...\n")
    status = k.rollout("deployment", name, action="status")

    if status["complete"]:
        printf("Rollout complete: %d/%d replicas ready.\n",
            status["ready"], status["replicas"])
    else:
        printf("Rollout still in progress: %d/%d ready.\n",
            status["ready"], status["replicas"])
        printf("Waiting for completion...\n")
        k.wait_for("deployment", name, condition="available", timeout="5m")

    printf("\nSuccessfully updated %s to %s.\n", name, new_image)

main()
