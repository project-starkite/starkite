#!/usr/bin/env kite
# app-stack.star - Build and apply a full app stack using k8s.obj constructors
#
# Demonstrates Tier 3 (object constructors) composed with Tier 1 (apply/wait_for).
# Builds a ConfigMap + Deployment + Service with probes, resource limits,
# environment variables, and volume mounts — all validated, no raw dicts.
#
# Usage:
#   kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1
#   kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --var namespace=staging
#   kite run examples/cloud/app-stack/app-stack.star --var image=myapp:v1 --dry-run

def build_config_map(name, ns):
    """Build a ConfigMap with app settings."""
    return k8s.obj.config_map(
        name=name + "-config",
        data={
            "APP_ENV": ns,
            "LOG_LEVEL": "debug" if ns == "default" else "info",
            "LOG_FORMAT": "json",
            "MAX_CONNECTIONS": "100",
        },
    )

def build_deployment(name, image, replicas):
    """Build a Deployment with probes, resources, and volumes."""
    container = k8s.obj.container(
        name=name,
        image=image,
        ports=[
            k8s.obj.container_port(container_port=80, name="http"),
            k8s.obj.container_port(container_port=9090, name="metrics"),
        ],
        env=[
            k8s.obj.env_var(name="APP_NAME", value=name),
            k8s.obj.env_var(name="POD_NAME", value_from={
                "fieldRef": {"fieldPath": "metadata.name"},
            }),
        ],
        env_from=[
            k8s.obj.env_from(config_map_ref={"name": name + "-config"}),
        ],
        resources=k8s.obj.resource_requirements(
            requests={"cpu": "100m", "memory": "128Mi"},
            limits={"cpu": "500m", "memory": "512Mi"},
        ),
        readiness_probe=k8s.obj.probe(
            http_get={"path": "/", "port": 80},
            initial_delay_seconds=5,
            period_seconds=10,
        ),
        liveness_probe=k8s.obj.probe(
            http_get={"path": "/", "port": 80},
            initial_delay_seconds=15,
            period_seconds=20,
            timeout_seconds=3,
        ),
        volume_mounts=[
            k8s.obj.volume_mount(name="tmp", mount_path="/tmp"),
            k8s.obj.volume_mount(name="config", mount_path="/etc/app", read_only=True),
        ],
    )

    return k8s.obj.deployment(
        name=name,
        labels={"app": name},
        replicas=replicas,
        containers=[container],
        volumes=[
            k8s.obj.volume(name="tmp", empty_dir={}),
            k8s.obj.volume(name="config", config_map={"name": name + "-config"}),
        ],
    )

def build_service(name):
    """Build a ClusterIP Service with http and metrics ports."""
    return k8s.obj.service(
        name=name,
        type="ClusterIP",
        selector={"app": name},
        ports=[
            k8s.obj.service_port(name="http", port=80, target_port=80),
            k8s.obj.service_port(name="metrics", port=9090, target_port=9090),
        ],
    )

def main():
    ns = var_str("namespace", "default")
    image = var_str("image")
    name = var_str("app.name", "myapp")
    replicas = var_int("replicas", 2)

    k = k8s.config(namespace=ns)

    # --- Build resources -------------------------------------------------------
    cm = build_config_map(name, ns)
    dep = build_deployment(name, image, replicas)
    svc = build_service(name)

    # --- Preview YAML ----------------------------------------------------------
    printf("Resources to apply:\n\n")
    printf("%s\n", k8s.yaml([cm, dep, svc]))

    # --- Apply -----------------------------------------------------------------
    printf("Applying to namespace %s...\n", ns)
    k.apply([cm, dep, svc])

    # --- Wait for rollout ------------------------------------------------------
    printf("Waiting for deployment/%s to be available...\n", name)
    result = k.wait_for("deployment", name, condition="available", timeout="5m")

    if result["ready"]:
        printf("Stack deployed successfully.\n")
        printf("  Deployment: %s (%d replicas)\n", name, replicas)
        printf("  Service:    %s -> :%d\n", name, 80)
        printf("  ConfigMap:  %s-config\n", name)
    else:
        printf("Deployment did not become ready: %s\n", result["message"])
        fail("deployment failed")

main()
