#!/usr/bin/env kite
# microservices.star - Multi-service app with composable functions
#
# Replaces: helm install myapp ./umbrella-chart (umbrella chart with subcharts)
#
# Composable make_deployment()/make_service() functions replace Helm subcharts.
# Frontend + API + Worker deployed from a shared service profile dict.
# Cross-service wiring is just code ("http://api:%d" % port).
# Pipe pattern — generates YAML to stdout.
#
# Usage (pipe to kubectl):
#   kite run examples/cloud/microservices/microservices.star | kubectl apply -f -
#   kite run examples/cloud/microservices/microservices.star --var domain=app.example.com | kubectl apply -f -
#   kite run examples/cloud/microservices/microservices.star --var image.tag=v2.1 --var log.level=debug | kubectl apply -f -
#
# Usage (preview):
#   kite run examples/cloud/microservices/microservices.star | kubectl apply --dry-run=client -f -
#   kite run examples/cloud/microservices/microservices.star > manifests.yaml

# Service profiles — each entry defines a "subchart"
services = {
    "frontend": {"port": 80,   "replicas": 2, "health": "/"},
    "api":      {"port": 8080, "replicas": 3, "health": "/healthz"},
    "worker":   {"port": 9090, "replicas": 2, "health": "/healthz"},
}

def make_deployment(name, profile, registry, tag, log_level, api_url, ns):
    """Build a Deployment for a service — replaces a Helm subchart."""
    container = k8s.obj.container(
        name=name,
        image="%s/%s:%s" % (registry, name, tag),
        ports=[k8s.obj.container_port(container_port=profile["port"], name="http")],
        env=[
            k8s.obj.env_var(name="SERVICE_NAME", value=name),
            k8s.obj.env_var(name="LOG_LEVEL", value=log_level),
            k8s.obj.env_var(name="API_URL", value=api_url),
        ],
        env_from=[
            k8s.obj.env_from(config_map_ref={"name": "shared-config"}),
        ],
        readiness_probe=k8s.obj.probe(
            http_get={"path": profile["health"], "port": profile["port"]},
            initial_delay_seconds=5,
            period_seconds=10,
        ),
        liveness_probe=k8s.obj.probe(
            http_get={"path": profile["health"], "port": profile["port"]},
            initial_delay_seconds=15,
            period_seconds=20,
        ),
        resources=k8s.obj.resource_requirements(
            requests={"cpu": "100m", "memory": "128Mi"},
            limits={"cpu": "500m", "memory": "256Mi"},
        ),
    )

    return k8s.obj.deployment(
        name=name,
        namespace=ns,
        labels={"app": "microservices", "component": name},
        replicas=profile["replicas"],
        containers=[container],
    )

def make_service(name, profile, ns):
    """Build a Service for a service component."""
    return k8s.obj.service(
        name=name,
        namespace=ns,
        labels={"app": "microservices", "component": name},
        selector={"app": "microservices", "component": name},
        ports=[k8s.obj.service_port(name="http", port=profile["port"], target_port=profile["port"])],
    )

def build_ingress(domain, ns):
    """Build an Ingress routing /api -> api, / -> frontend."""
    api_path = k8s.obj.ingress_path(
        path="/api",
        path_type="Prefix",
        backend={"service": {"name": "api", "port": {"number": 8080}}},
    )
    frontend_path = k8s.obj.ingress_path(
        path="/",
        path_type="Prefix",
        backend={"service": {"name": "frontend", "port": {"number": 80}}},
    )
    rule = k8s.obj.ingress_rule(
        host=domain,
        paths={"paths": [api_path, frontend_path]},
    )
    return k8s.obj.ingress(
        name="microservices",
        namespace=ns,
        labels={"app": "microservices"},
        ingress_class_name="nginx",
        rules=[rule],
    )

def main():
    # --- Variables ---------------------------------------------------------------
    tag = var_str("image.tag", "latest")
    registry = var_str("image.registry", "myapp")
    domain = var_str("domain", "")
    log_level = var_str("log.level", "info")
    ns = var_str("namespace", "default")

    # Cross-service wiring — just code, not template expressions
    api_port = services["api"]["port"]
    api_url = "http://api:%d" % api_port

    resources = []

    # --- Shared ConfigMap (replaces Helm global values) --------------------------
    shared_config = k8s.obj.config_map(
        name="shared-config",
        namespace=ns,
        labels={"app": "microservices"},
        data={
            "ENVIRONMENT": ns,
            "LOG_FORMAT": "json",
            "API_URL": api_url,
        },
    )
    resources.append(shared_config)

    # --- Generate Deployment + Service per component -----------------------------
    for name in services:
        profile = services[name]
        dep = make_deployment(name, profile, registry, tag, log_level, api_url, ns)
        svc = make_service(name, profile, ns)
        resources.append(dep)
        resources.append(svc)

    # --- Ingress (conditional) ---------------------------------------------------
    if domain:
        ingress = build_ingress(domain, ns)
        resources.append(ingress)

    # --- Output YAML -------------------------------------------------------------
    print(k8s.yaml(resources))

main()
