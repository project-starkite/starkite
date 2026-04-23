#!/usr/bin/env kite
# cluster-health.star - Read-only cluster health audit
#
# Scans a cluster for common problems: unhealthy nodes, under-replicated
# deployments, stuck pods, and resource pressure. Touches nothing —
# safe to run against production.
#
# Usage:
#   kite run examples/cloud/cluster-health/cluster-health.star
#   kite run examples/cloud/cluster-health/cluster-health.star --var k8s.context=prod-cluster
#   kite run examples/cloud/cluster-health/cluster-health.star --var skip.system=false

def check_nodes(k, issues):
    """Check node health and resource pressure."""
    printf("Nodes:\n")
    nodes = k.list("nodes")
    for node in nodes:
        name = node["metadata"]["name"]
        conditions = {c["type"]: c["status"] for c in node["status"].get("conditions", [])}

        ready = conditions.get("Ready", "Unknown")
        pressure = []
        if conditions.get("MemoryPressure") == "True":
            pressure.append("MemoryPressure")
        if conditions.get("DiskPressure") == "True":
            pressure.append("DiskPressure")
        if conditions.get("PIDPressure") == "True":
            pressure.append("PIDPressure")

        status = "Ready" if ready == "True" else "NotReady"
        if pressure:
            status = status + " (" + ", ".join(pressure) + ")"
            issues.append("Node %s has pressure: %s" % (name, ", ".join(pressure)))
        if ready != "True":
            issues.append("Node %s is %s" % (name, status))

        marker = " " if ready == "True" and not pressure else "!"
        printf("  %s %-30s %s\n", marker, name, status)

    return nodes

def check_node_resources(k):
    """Show node capacity and allocatable resources."""
    printf("\nNode resources:\n")
    printf("  %-25s %10s %10s %10s %10s\n", "NODE", "CPU CAP", "CPU ALLOC", "MEM CAP", "MEM ALLOC")
    printf("  %-25s %10s %10s %10s %10s\n", "----", "-------", "---------", "-------", "---------")

    top = k.top_nodes()
    for n in top:
        cap = n.get("capacity", {})
        alloc = n.get("allocatable", {})
        printf("  %-25s %10s %10s %10s %10s\n",
            n["name"],
            cap.get("cpu", "?"),
            alloc.get("cpu", "?"),
            cap.get("memory", "?"),
            alloc.get("memory", "?"))

def check_deployments(k, namespaces, skip_system, issues):
    """Check deployment health across namespaces."""
    printf("\nDeployments:\n")
    total_deps = 0
    healthy_deps = 0

    for ns in namespaces:
        ns_name = ns["metadata"]["name"]
        if skip_system and ns_name.startswith("kube-"):
            continue

        deps = k.list("deployments", namespace=ns_name)
        for dep in deps:
            total_deps += 1
            dep_name = dep["metadata"]["name"]
            desired = dep["spec"].get("replicas", 1)
            available = dep["status"].get("availableReplicas", 0)
            ready = dep["status"].get("readyReplicas", 0)

            if available >= desired:
                healthy_deps += 1
            else:
                issues.append("%s/%s: %d/%d available" % (ns_name, dep_name, available, desired))
                printf("  ! %-20s %-20s %d/%d ready\n", ns_name, dep_name, ready, desired)

    printf("  %d/%d deployments healthy\n", healthy_deps, total_deps)
    return total_deps

def check_pods(k, namespaces, skip_system, issues):
    """Check for stuck or crash-looping pods."""
    printf("\nProblem pods:\n")
    problem_count = 0

    for ns in namespaces:
        ns_name = ns["metadata"]["name"]
        if skip_system and ns_name.startswith("kube-"):
            continue

        pods = k.list("pods", namespace=ns_name)
        for pod in pods:
            phase = pod["status"].get("phase", "Unknown")
            pod_name = pod["metadata"]["name"]

            if phase in ("Pending", "Unknown"):
                problem_count += 1
                issues.append("Pod %s/%s is %s" % (ns_name, pod_name, phase))
                printf("  ! %-20s %-40s %s\n", ns_name, pod_name, phase)
                continue

            # Check for crash-looping containers
            for cs in pod["status"].get("containerStatuses", []):
                restarts = cs.get("restartCount", 0)
                if restarts > 5:
                    problem_count += 1
                    issues.append("Pod %s/%s container %s restarted %d times" % (
                        ns_name, pod_name, cs["name"], restarts))
                    printf("  ! %-20s %-40s %d restarts (%s)\n",
                        ns_name, pod_name, restarts, cs["name"])

    if problem_count == 0:
        printf("  (none)\n")

def main():
    ctx = var_str("k8s.context", "")
    skip_system = var_bool("skip.system", True)

    if ctx:
        k = k8s.config(context=ctx, timeout="30s")
    else:
        k = k8s.config(timeout="30s")

    issues = []

    printf("Cluster: %s\n", k.context())
    info = k.version()
    printf("Version: %s (platform: %s)\n\n", info["git_version"], info["platform"])

    nodes = check_nodes(k, issues)
    check_node_resources(k)

    namespaces = k.list("namespaces")
    total_deps = check_deployments(k, namespaces, skip_system, issues)
    check_pods(k, namespaces, skip_system, issues)

    # --- Summary ---------------------------------------------------------------
    printf("\n" + "=" * 60 + "\n")
    if issues:
        printf("ISSUES FOUND: %d\n", len(issues))
        for issue in issues:
            printf("  - %s\n", issue)
        fail("cluster health check failed with %d issues" % len(issues))
    else:
        printf("ALL CLEAR: %d nodes, %d deployments healthy\n", len(nodes), total_deps)

main()
