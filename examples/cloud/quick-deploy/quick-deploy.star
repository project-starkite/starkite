#!/usr/bin/env kite
# quick-deploy.star - Deploy, scale, and inspect an app with zero YAML
#
# Demonstrates Tier 2 (kubectl abstractions) — the simplest way to get
# an app running on Kubernetes. One function deploys, another scales,
# another shows resource usage.
#
# Usage:
#   kite run examples/cloud/quick-deploy/quick-deploy.star
#   kite run examples/cloud/quick-deploy/quick-deploy.star --var image=myapp:v2
#   kite run examples/cloud/quick-deploy/quick-deploy.star --var namespace=staging
#   kite run examples/cloud/quick-deploy/quick-deploy.star --dry-run

def main():
    ns = var_str("namespace", "default")
    image = var_str("image", "nginx:1.27")
    name = var_str("app.name", "web")
    replicas = var_int("replicas", 3)

    k = k8s.config(namespace=ns)

    # --- Deploy ---------------------------------------------------------------
    # Creates a Deployment + ClusterIP Service, waits for rollout to complete.
    printf("Deploying %s (%s) with %d replicas...\n", name, image, replicas)

    result = k.deploy(name, image,
        replicas=replicas,
        port=80,
        labels={"team": "platform", "managed-by": "starctl"},
        env={"APP_NAME": name})

    # deploy() returns {"deployment": name_str, "service": name_str}
    printf("  Deployment: %s\n", result["deployment"])
    if result.get("service"):
        printf("  Service:    %s (ClusterIP)\n", result["service"])

    # --- Scale up --------------------------------------------------------------
    new_count = replicas + 2
    printf("\nScaling %s to %d replicas...\n", name, new_count)
    k.scale("deployment", name, new_count)

    # --- Autoscale -------------------------------------------------------------
    printf("Setting up HPA (min=%d, max=%d, target CPU=70%%)...\n", replicas, new_count * 2)
    k.autoscale("deployment", name,
        min=replicas,
        max=new_count * 2,
        cpu_percent=70)

    # --- Resource usage --------------------------------------------------------
    printf("\nPod resource usage:\n")
    printf("  %-40s %8s %12s %s\n", "POD", "CPU REQ", "MEM REQ", "STATUS")
    printf("  %-40s %8s %12s %s\n", "---", "-------", "-------", "------")

    pods = k.top_pods(sort_by="cpu", timeout="15s")
    for p in pods:
        if name in p["name"]:
            printf("  %-40s %8s %12s %s\n",
                p["name"], p["cpu_request"], p["memory_request"], p["status"])

    printf("\nDone. %s is running in namespace %s.\n", name, ns)

main()
