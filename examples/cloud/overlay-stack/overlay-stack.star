#!/usr/bin/env kite
# overlay-stack.star - Offline multi-environment manifests with yaml.merge(),
# k8s.obj cloning, and k8s.overlay().
#
# Usage:
#   kite run ./examples/cloud/overlay-stack/overlay-stack.star
#   kite run ./examples/cloud/overlay-stack/overlay-stack.star --var env=staging

base_defaults = {
    "replicas": 1,
    "image": "monopole/hello:1",
    "env": [
        {"name": "LOG_LEVEL", "value": "info"},
        {"name": "FEATURE_FLAG", "value": "false"},
    ],
}

env_patches = {
    "staging": {
        "replicas": 1,
        "env": [{"name": "LOG_LEVEL", "value": "debug"}, {"name": "FEATURE_FLAG", "value": "true"}],
    },
    "production": {
        "replicas": 3,
        "image": "registry.example.internal/monopole/hello:2",
        "env": [{"name": "LOG_LEVEL", "value": "warn"}],
    },
}

base_cm = k8s.obj.config_map(
    name = "app-config",
    data = {"altGreeting": "Good Morning!", "enableRisky": "false"},
)

base_dep = k8s.obj.deployment(
    name = "app-web",
    replicas = 1,
    labels = {"app": "hello"},
    containers = [{
        "name": "web",
        "image": "monopole/hello:1",
        "ports": [{"name": "http", "containerPort": 8080}],
        "env": [
            {"name": "ALT_GREETING", "valueFrom": {"configMapKeyRef": {"name": "app-config", "key": "altGreeting"}}},
        ],
    }],
)

base_svc = k8s.obj.service(
    name = "app-svc",
    selector = {"app": "hello"},
    ports = [{"name": "http", "port": 80, "targetPort": 8080}],
)

def harden_containers(obj):
    if obj.get("kind") != "Deployment":
        return obj
    for c in obj["spec"]["template"]["spec"]["containers"]:
        c["securityContext"] = {"runAsNonRoot": True, "readOnlyRootFilesystem": True}
    return obj

def main():
    env_name = var_str("env", "production")
    merged = yaml.merge(base_defaults, env_patches.get(env_name, {}), list_mode = "by_key", list_key = "name")

    cm = base_cm
    if env_name == "staging":
        cm = k8s.obj.config_map(base_cm, data = {"altGreeting": "Have a pineapple!", "enableRisky": "true"})

    ns_name = "hello-" + env_name
    extra = [k8s.obj.namespace(name = ns_name)]
    patches = [{"kind": "Deployment", "name": "app-web", "patch": {"spec": {"replicas": merged["replicas"]}}}]

    if env_name == "production":
        patches.append(harden_containers)
        extra += [
            k8s.obj.pod_disruption_budget(name = "app-pdb", min_available = 1, selector = {"matchLabels": {"app": "hello"}}),
            k8s.obj.horizontal_pod_autoscaler(
                name = "app-hpa",
                scale_target_ref = {"apiVersion": "apps/v1", "kind": "Deployment", "name": "app-web"},
                min_replicas = merged["replicas"],
                max_replicas = 6,
            ),
        ]

    manifests = k8s.overlay(
        [cm, base_dep, base_svc],
        namespace = ns_name,
        name_prefix = env_name + "-",
        labels = {"environment": env_name},
        images = {"monopole/hello": merged["image"]},
        patches = patches,
        extra = extra,
    )
    print(k8s.yaml(manifests))
