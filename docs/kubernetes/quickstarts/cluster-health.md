---
title: "Cluster health audit"
description: "Read-only scan for unhealthy nodes, under-replicated deployments, and crash-looping pods"
weight: 30
---

# Cluster health audit

A read-only audit of an entire cluster. The script lists nodes, namespaces, deployments, and pods; checks for memory/disk/PID pressure, under-replicated workloads, and crash-loops; prints a summary; and exits non-zero when issues are found. It touches nothing — safe to point at production from a CI cron, a status page, or a Slack bot.

This is a Tier 1 read-only path: only `list`, `top_nodes`, `version`, and `context` are used.

**Source:** [`examples/cloud/cluster-health/cluster-health.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/cluster-health/cluster-health.star)

## Node check

```python
def check_nodes(k, issues):
    nodes = k.list("nodes")
    for node in nodes:
        name = node["metadata"]["name"]
        conditions = {c["type"]: c["status"]
                      for c in node["status"].get("conditions", [])}

        ready = conditions.get("Ready", "Unknown")
        pressure = [p for p in ("MemoryPressure", "DiskPressure", "PIDPressure")
                    if conditions.get(p) == "True"]

        if pressure:
            issues.append("Node %s has pressure: %s" % (name, ", ".join(pressure)))
        if ready != "True":
            issues.append("Node %s is %s" % (name, ready))
```

## Deployment + pod scan

```python
def check_deployments(k, namespaces, skip_system, issues):
    for ns in namespaces:
        ns_name = ns["metadata"]["name"]
        if skip_system and ns_name.startswith("kube-"):
            continue

        for dep in k.list("deployments", namespace=ns_name):
            desired = dep["spec"].get("replicas", 1)
            available = dep["status"].get("availableReplicas", 0)
            if available < desired:
                issues.append("%s/%s: %d/%d available" % (
                    ns_name, dep["metadata"]["name"], available, desired))

def check_pods(k, namespaces, skip_system, issues):
    for ns in namespaces:
        ns_name = ns["metadata"]["name"]
        if skip_system and ns_name.startswith("kube-"):
            continue

        for pod in k.list("pods", namespace=ns_name):
            phase = pod["status"].get("phase", "Unknown")
            if phase in ("Pending", "Unknown"):
                issues.append("Pod %s/%s is %s" % (ns_name, pod["metadata"]["name"], phase))

            for cs in pod["status"].get("containerStatuses", []):
                if cs.get("restartCount", 0) > 5:
                    issues.append("Pod %s/%s container %s restarted %d times" % (
                        ns_name, pod["metadata"]["name"], cs["name"], cs["restartCount"]))
```

## Run it

```bash
kite run examples/cloud/cluster-health/cluster-health.star
kite run examples/cloud/cluster-health/cluster-health.star --var k8s.context=prod-cluster
kite run examples/cloud/cluster-health/cluster-health.star --var skip.system=false
```

The script exits non-zero when any issue is found, so it slots directly into a CI cron or alerting pipeline.

## What's happening

- **`k.list(kind, namespace=...)`** returns a `list[dict]` of resources — pure JSON shape, no client objects to manage.
- **`k.top_nodes()`** returns the metrics-server view of node capacity and allocatable resources.
- **`k.version()` / `k.context()`** identify the cluster and the kubeconfig context the script targeted.
- **No writes** — every call is read-only. Point at production safely.

## See also

- [`k8s` reference](../../references/api/k8s.md) — `list`, `top_nodes`, `version`, `context`
