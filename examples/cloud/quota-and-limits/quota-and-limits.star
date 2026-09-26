#!/usr/bin/env kite
# quota-and-limits.star - Define resource quotas and limit ranges
#
# Builds Kubernetes LimitRange and ResourceQuota manifests using typed
# k8s.obj constructors, and outputs multi-document YAML or applies to cluster.
#
# Usage:
#   # Preview generated YAML manifests
#   kite run ./examples/cloud/quota-and-limits/quota-and-limits.star
#
#   # Override namespace with --var
#   kite run ./examples/cloud/quota-and-limits/quota-and-limits.star --var namespace=prod
#
#   # Pipe directly to kubectl
#   kite run ./examples/cloud/quota-and-limits/quota-and-limits.star | kubectl apply -f -
#
#   # Apply directly to cluster using starkite k8s client
#   kite run ./examples/cloud/quota-and-limits/quota-and-limits.star --var apply=true

def build_limit_range(name, ns):
    """Build a LimitRange setting container defaults and bounds."""
    return k8s.obj.limit_range(
        name=name,
        namespace=ns,
        labels={"tier": "governance", "env": ns},
        limits=[
            k8s.obj.limit_range_item(
                type="Container",
                default={"cpu": "500m", "memory": "256Mi"},
                default_request={"cpu": "100m", "memory": "128Mi"},
                max={"cpu": "2", "memory": "2Gi"},
                min={"cpu": "50m", "memory": "64Mi"},
                max_limit_request_ratio={"cpu": "4", "memory": "2"},
            ),
            k8s.obj.limit_range_item(
                type="PersistentVolumeClaim",
                min={"storage": "1Gi"},
                max={"storage": "50Gi"},
            ),
        ],
    )

def build_resource_quota(name, ns):
    """Build a ResourceQuota setting hard compute and object limits."""
    return k8s.obj.resource_quota(
        name=name,
        namespace=ns,
        labels={"tier": "governance", "env": ns},
        hard={
            "requests.cpu": "4",
            "requests.memory": "8Gi",
            "limits.cpu": "8",
            "limits.memory": "16Gi",
            "pods": "20",
            "persistentvolumeclaims": "10",
            "services.loadbalancers": "2",
        },
        scopes=["NotTerminating"],
    )

def main():
    ns = var_str("namespace", "engineering")
    should_apply = var_bool("apply", False)

    limits = build_limit_range("team-limits", ns)
    quota = build_resource_quota("compute-quota", ns)

    resources = [limits, quota]

    if should_apply:
        printf("Applying LimitRange and ResourceQuota to namespace %s...\n", ns)
        k = k8s.config(namespace=ns)
        k.apply(resources)
        printf("Governance resources successfully applied to %s.\n", ns)
    else:
        # Emit multi-document YAML stream
        print(k8s.yaml(resources))
