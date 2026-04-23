#!/usr/bin/env kite
# namespace-stack.star - Scaffold a namespace with quotas and RBAC
#
# Generates a complete namespace setup: Namespace + ResourceQuota +
# LimitRange + RoleBinding. Pure YAML generation via yaml.encode() —
# no cluster access needed, just pipe to kubectl.
#
# Usage:
#   kite run examples/cloud/namespace-stack/namespace-stack.star --var name=team-backend | kubectl apply -f -
#   kite run examples/cloud/namespace-stack/namespace-stack.star --var name=team-backend | kubectl diff -f -
#   kite run examples/cloud/namespace-stack/namespace-stack.star --var name=staging --var cpu=4 --var mem=8Gi > ns.yaml

def main():
    name = var_str("name")
    cpu_limit = var_str("cpu", "8")
    mem_limit = var_str("mem", "16Gi")

    # --- Namespace ---------------------------------------------------------
    print("---")
    print(yaml.encode({
        "apiVersion": "v1",
        "kind": "Namespace",
        "metadata": {
            "name": name,
            "labels": {"team": name, "managed-by": "starctl"},
        },
    }))

    # --- ResourceQuota -----------------------------------------------------
    print("---")
    print(yaml.encode({
        "apiVersion": "v1",
        "kind": "ResourceQuota",
        "metadata": {"name": "default-quota", "namespace": name},
        "spec": {"hard": {
            "limits.cpu": cpu_limit,
            "limits.memory": mem_limit,
            "pods": "50",
        }},
    }))

    # --- LimitRange — defaults so every pod gets limits --------------------
    print("---")
    print(yaml.encode({
        "apiVersion": "v1",
        "kind": "LimitRange",
        "metadata": {"name": "default-limits", "namespace": name},
        "spec": {"limits": [{
            "type": "Container",
            "default": {"cpu": "200m", "memory": "256Mi"},
            "defaultRequest": {"cpu": "100m", "memory": "128Mi"},
        }]},
    }))

    # --- RoleBinding — namespace-scoped admin for the team group -----------
    print("---")
    print(yaml.encode({
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "RoleBinding",
        "metadata": {"name": "team-admin", "namespace": name},
        "subjects": [{"kind": "Group", "name": name, "apiGroup": "rbac.authorization.k8s.io"}],
        "roleRef": {"kind": "ClusterRole", "name": "admin", "apiGroup": "rbac.authorization.k8s.io"},
    }))

main()
