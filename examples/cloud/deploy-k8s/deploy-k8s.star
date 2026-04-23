#!/usr/bin/env kite
# deploy-k8s.star - Generate Kubernetes deployment manifests
#
# This script demonstrates starctl's ability to generate Kubernetes
# manifests programmatically, as an alternative to Helm/Kustomize.
#
# Usage:
#   kite run examples/cloud/deploy-k8s/deploy-k8s.star                           # Generate YAML
#   kite run examples/cloud/deploy-k8s/deploy-k8s.star | kubectl apply -f -      # Apply to cluster
#   ENVIRONMENT=prod kite run examples/cloud/deploy-k8s/deploy-k8s.star          # Production config

# =============================================================================
# CONFIGURATION
# =============================================================================

config = {
    "environment": env("ENVIRONMENT", "dev"),
    "app": {
        "name": env("APP_NAME", "myapp"),
        "version": env("APP_VERSION", "1.0.0"),
        "image": env("APP_IMAGE", "nginx"),
        "tag": env("IMAGE_TAG", "latest"),
    },
    "replicas": {
        "dev": 1,
        "staging": 2,
        "prod": 3,
    },
    "resources": {
        "dev": {
            "requests": {"cpu": "100m", "memory": "128Mi"},
            "limits": {"cpu": "200m", "memory": "256Mi"},
        },
        "staging": {
            "requests": {"cpu": "200m", "memory": "256Mi"},
            "limits": {"cpu": "500m", "memory": "512Mi"},
        },
        "prod": {
            "requests": {"cpu": "500m", "memory": "512Mi"},
            "limits": {"cpu": "1000m", "memory": "1Gi"},
        },
    },
}

ENV = config["environment"]
APP = config["app"]
NAMESPACE = env("NAMESPACE", APP["name"] + "-" + ENV)

# =============================================================================
# HELPER FUNCTIONS
# =============================================================================

def labels(component = None):
    """Generate standard Kubernetes labels."""
    l = {
        "app.kubernetes.io/name": APP["name"],
        "app.kubernetes.io/version": APP["version"],
        "app.kubernetes.io/managed-by": "starctl",
        "environment": ENV,
    }
    if component:
        l["app.kubernetes.io/component"] = component
    return l

def selector_labels():
    """Generate selector labels (immutable)."""
    return {
        "app.kubernetes.io/name": APP["name"],
        "environment": ENV,
    }

def get_replicas():
    """Get replica count for current environment."""
    return config["replicas"].get(ENV, 1)

def get_resources():
    """Get resource limits for current environment."""
    return config["resources"].get(ENV, config["resources"]["dev"])

# =============================================================================
# MANIFEST GENERATORS
# =============================================================================

def namespace_manifest():
    """Generate Namespace manifest."""
    return {
        "apiVersion": "v1",
        "kind": "Namespace",
        "metadata": {
            "name": NAMESPACE,
            "labels": labels(),
        },
    }

def configmap_manifest():
    """Generate ConfigMap manifest."""
    return {
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {
            "name": APP["name"] + "-config",
            "namespace": NAMESPACE,
            "labels": labels("config"),
        },
        "data": {
            "ENVIRONMENT": ENV,
            "LOG_LEVEL": "debug" if ENV == "dev" else "info",
            "APP_VERSION": APP["version"],
        },
    }

def deployment_manifest():
    """Generate Deployment manifest."""
    return {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {
            "name": APP["name"],
            "namespace": NAMESPACE,
            "labels": labels("server"),
        },
        "spec": {
            "replicas": get_replicas(),
            "selector": {
                "matchLabels": selector_labels(),
            },
            "strategy": {
                "type": "RollingUpdate",
                "rollingUpdate": {
                    "maxSurge": 1,
                    "maxUnavailable": 0,
                },
            },
            "template": {
                "metadata": {
                    "labels": labels("server"),
                },
                "spec": {
                    "containers": [{
                        "name": APP["name"],
                        "image": "%s:%s" % (APP["image"], APP["tag"]),
                        "imagePullPolicy": "IfNotPresent",
                        "ports": [{"containerPort": 80, "name": "http"}],
                        "envFrom": [
                            {"configMapRef": {"name": APP["name"] + "-config"}},
                        ],
                        "resources": get_resources(),
                        "livenessProbe": {
                            "httpGet": {"path": "/", "port": "http"},
                            "initialDelaySeconds": 15,
                            "periodSeconds": 10,
                        },
                        "readinessProbe": {
                            "httpGet": {"path": "/", "port": "http"},
                            "initialDelaySeconds": 5,
                            "periodSeconds": 5,
                        },
                    }],
                },
            },
        },
    }

def service_manifest():
    """Generate Service manifest."""
    return {
        "apiVersion": "v1",
        "kind": "Service",
        "metadata": {
            "name": APP["name"],
            "namespace": NAMESPACE,
            "labels": labels("server"),
        },
        "spec": {
            "type": "ClusterIP",
            "selector": selector_labels(),
            "ports": [
                {"name": "http", "port": 80, "targetPort": "http"},
            ],
        },
    }

# =============================================================================
# OUTPUT
# =============================================================================

def generate_all():
    """Generate all manifests."""
    return [
        namespace_manifest(),
        configmap_manifest(),
        deployment_manifest(),
        service_manifest(),
    ]

def main():
    """Output as YAML stream."""
    for manifest in generate_all():
        print("---")
        print(yaml.encode(manifest))

# Run main
main()
