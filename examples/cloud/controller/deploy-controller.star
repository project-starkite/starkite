#!/usr/bin/env kite-cloud
# deploy-controller.star — Programmatic deployment of a controller
# This shows how to deploy everything using Starlark instead of YAML

# Load and apply CRD
load("resource.star", "crd")
k8s.apply(crd)

# Create namespace
ns = "myapp-system"
k8s.apply(k8s.obj.namespace(name=ns))

# ServiceAccount
k8s.apply(k8s.obj.service_account(name="myapp-controller", namespace=ns))

# ClusterRole (permissive — tighten for production)
k8s.apply(k8s.obj.cluster_role(
    name="myapp-controller",
    rules=[k8s.obj.policy_rule(api_groups=["*"], resources=["*"], verbs=["*"])],
))

# ClusterRoleBinding
k8s.apply(k8s.obj.cluster_role_binding(
    name="myapp-controller",
    role_ref={"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "myapp-controller"},
    subjects=[k8s.obj.subject(kind="ServiceAccount", name="myapp-controller", namespace=ns)],
))

# Deploy controller
image = var_str("image", "ghcr.io/myorg/myapp-controller:latest")
k8s.apply(k8s.obj.deployment(
    name="myapp-controller",
    namespace=ns,
    replicas=var_int("replicas", 2),
    containers=[k8s.obj.container(name="controller", image=image)],
    labels={"app": "myapp-controller"},
))

printf("Controller deployed to %s\n", ns)
