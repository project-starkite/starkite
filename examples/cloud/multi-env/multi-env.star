#!/usr/bin/env kite
# multi-env.star - Generate manifests for dev + staging + prod in one shot
#
# Demonstrates Tier 3 (k8s.obj constructors) for generating per-environment
# Deployment + Service pairs with different replica counts and resource
# profiles. Pure YAML generation — pipe to kubectl to apply.
#
# Usage:
#   kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 | kubectl apply -f -
#   kite run examples/cloud/multi-env/multi-env.star --var image=myapp:v2.1 --var envs=staging,prod | kubectl apply -f -
#   kite run examples/cloud/multi-env/multi-env.star --var image=nginx:latest | kubectl apply --dry-run=server -f -
#   kite run examples/cloud/multi-env/multi-env.star --var image=nginx:latest | kubectl diff -f -

# Per-environment profiles
profiles = {
    "dev":     {"replicas": 1, "cpu": "100m", "mem": "128Mi"},
    "staging": {"replicas": 2, "cpu": "200m", "mem": "256Mi"},
    "prod":    {"replicas": 5, "cpu": "500m", "mem": "512Mi"},
}

def main():
    image = var_str("image")
    app = var_str("app", "web")
    envs_str = var_str("envs", "dev,staging,prod")
    envs = envs_str.split(",")

    # --- Generate per-environment Deployment + Service ---------------------
    for e in envs:
        p = profiles.get(e, profiles["dev"])
        ns = "%s-%s" % (app, e)

        dep = k8s.obj.deployment(
            name=app,
            labels={"app": app, "env": e},
            replicas=p["replicas"],
            containers=[
                k8s.obj.container(
                    name=app,
                    image=image,
                    ports=[k8s.obj.container_port(container_port=8080, name="http")],
                    resources=k8s.obj.resource_requirements(
                        requests={"cpu": p["cpu"], "memory": p["mem"]},
                    ),
                    env=[
                        k8s.obj.env_var(name="ENV", value=e),
                        k8s.obj.env_var(name="APP_NAME", value=app),
                    ],
                ),
            ],
        )

        svc = k8s.obj.service(
            name=app,
            selector={"app": app, "env": e},
            ports=[k8s.obj.service_port(name="http", port=80, target_port=8080)],
        )

        printf("# --- %s ---\n", ns)
        print(k8s.yaml([dep, svc]))

main()
