#!/usr/bin/env kite
# debug-pod.star - Interactive pod debugging toolkit
#
# Demonstrates Tier 1 pod operations: describe, logs, exec, and port-forward.
# Gathers diagnostic info from a pod in one shot — useful as a first response
# when something is misbehaving.
#
# Usage:
#   kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123
#   kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123 --var namespace=production
#   kite run examples/cloud/debug-pod/debug-pod.star --var pod.name=nginx-abc123 --var container=sidecar

def print_pod_info(info):
    """Print basic pod metadata."""
    resource = info["resource"]
    printf("  Phase:   %s\n", resource["status"].get("phase", "Unknown"))
    printf("  Node:    %s\n", resource["spec"].get("nodeName", "unassigned"))
    printf("  IP:      %s\n", resource["status"].get("podIP", "none"))
    printf("  Started: %s\n", resource["status"].get("startTime", "unknown"))

def print_conditions(info):
    """Print pod conditions."""
    conditions = info.get("conditions")
    if not conditions:
        printf("  (none)\n")
        return
    for c in conditions:
        marker = "+" if c["status"] == "True" else "-"
        printf("  %s %-25s %s\n", marker, c["type"], c.get("message", ""))

def print_containers(resource):
    """Print container statuses with waiting reasons."""
    for cs in resource["status"].get("containerStatuses", []):
        state_key = list(cs.get("state", {}).keys())
        state = state_key[0] if state_key else "unknown"
        printf("  %-20s state=%-12s ready=%-5s restarts=%d\n",
            cs["name"], state, str(cs.get("ready", False)), cs.get("restartCount", 0))

        if state == "waiting":
            waiting = cs["state"]["waiting"]
            printf("    reason: %s\n", waiting.get("reason", ""))
            if waiting.get("message"):
                printf("    message: %s\n", waiting["message"])

def print_events(info):
    """Print recent events."""
    events = info.get("events")
    if not events:
        printf("  (no events)\n")
        return
    for ev in events[-10:]:
        printf("  %-8s %-25s %s\n",
            ev.get("reason", ""), ev.get("type", ""), ev.get("message", ""))

def print_logs(k, pod_name, container, tail_lines):
    """Print recent logs, falling back to previous container logs."""
    if container:
        logs = k.logs(pod_name, container=container, tail=tail_lines)
    else:
        logs = k.logs(pod_name, tail=tail_lines)

    if logs:
        printf("%s\n", logs)
    else:
        printf("  (no logs)\n")
        printf("Checking previous container logs...\n")
        if container:
            prev = k.logs(pod_name, container=container, tail=tail_lines, previous=True)
        else:
            prev = k.logs(pod_name, tail=tail_lines, previous=True)
        if prev:
            printf("%s\n", prev)
        else:
            printf("  (no previous logs either)\n")

def run_diagnostics(k, pod_name, container):
    """Run quick diagnostic commands via exec."""
    commands = [
        ["id"],
        ["cat", "/etc/os-release"],
        ["df", "-h", "/"],
        ["printenv"],
    ]

    for cmd in commands:
        cmd_str = " ".join(cmd)
        if container:
            result = k.exec(pod_name, cmd, container=container)
        else:
            result = k.exec(pod_name, cmd)
        if result["code"] == 0:
            lines = result["stdout"].strip().split("\n")
            preview = "\n    ".join(lines[:5])
            if len(lines) > 5:
                preview += "\n    ... (%d more lines)" % (len(lines) - 5)
            printf("\n  $ %s\n    %s\n", cmd_str, preview)
        else:
            printf("\n  $ %s\n    (exit %d) %s\n", cmd_str, result["code"], result["stderr"].strip())

def main():
    pod_name = var_str("pod.name")
    ns = var_str("namespace", "default")
    container = var_str("container", "")
    tail_lines = var_int("tail.lines", 50)
    port = var_int("port", 0)  # 0 = skip port-forward

    k = k8s.config(namespace=ns)

    printf("Debugging pod/%s in namespace %s\n", pod_name, ns)
    printf("=" * 60 + "\n")

    # --- Describe --------------------------------------------------------------
    printf("\n[1/5] Pod info\n")
    info = k.describe("pod", pod_name)
    print_pod_info(info)

    # --- Conditions ------------------------------------------------------------
    printf("\n[2/5] Conditions\n")
    print_conditions(info)

    # --- Container statuses ----------------------------------------------------
    printf("\n[3/5] Containers\n")
    print_containers(info["resource"])

    # --- Events ----------------------------------------------------------------
    printf("\n[4/5] Recent events\n")
    print_events(info)

    # --- Logs ------------------------------------------------------------------
    printf("\n[5/5] Last %d log lines", tail_lines)
    if container:
        printf(" (container: %s)", container)
    printf("\n" + "-" * 60 + "\n")
    print_logs(k, pod_name, container, tail_lines)

    # --- Exec ------------------------------------------------------------------
    printf("-" * 60 + "\n")
    printf("Quick diagnostics via exec:\n")
    run_diagnostics(k, pod_name, container)

    # --- Port-forward (optional) -----------------------------------------------
    if port > 0:
        printf("\n" + "-" * 60 + "\n")
        printf("Port-forwarding to pod:%d...\n", port)
        pf = k.port_forward(pod_name, port, local_port=0)
        printf("Forwarding localhost:%d -> pod:%d\n", pf.local_port, port)

        resp = http.url("http://localhost:%d/" % pf.local_port).get()
        printf("  GET / -> %d\n", resp.status)

        pf.stop()

    printf("\nDebug session complete.\n")

main()
