# Default controller artifact generator
# Executed by: kite-cloud kube gen-controller-artifacts

def main():
    controller_script = var_str("controller_script", "controller.star")
    image = var_str("image")
    ns = var_str("namespace", "default")
    replicas = var_int("replicas", 1)
    output_format = var_str("output_format", "yaml")
    crd_yaml = var_str("crd_yaml", "")

    # Derive controller name from script path
    name = controller_script.replace(".star", "").replace("/", "-").replace("_", "-").strip("-")

    if output_format == "yaml":
        generate_yaml(name, ns, image, replicas, crd_yaml)
    elif output_format == "script":
        generate_script(name, ns, image, replicas, crd_yaml)

def generate_yaml(name, ns, image, replicas, crd_yaml):
    # CRD (injected by CLI if --resource was provided)
    if crd_yaml:
        printf("---\n%s", crd_yaml)

    # Namespace
    printf("---\n%s", k8s.yaml(k8s.obj.namespace(name=ns)))

    # ServiceAccount
    printf("---\n%s", k8s.yaml(k8s.obj.service_account(name=name, namespace=ns)))

    # ClusterRole (permissive — tighten for production)
    printf("---\n%s", k8s.yaml(k8s.obj.cluster_role(
        name=name,
        rules=[k8s.obj.policy_rule(api_groups=["*"], resources=["*"], verbs=["*"])],
    )))

    # ClusterRoleBinding
    printf("---\n%s", k8s.yaml(k8s.obj.cluster_role_binding(
        name=name,
        role_ref={"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": name},
        subjects=[k8s.obj.subject(kind="ServiceAccount", name=name, namespace=ns)],
    )))

    # Deployment
    printf("---\n%s", k8s.yaml(k8s.obj.deployment(
        name=name,
        namespace=ns,
        replicas=replicas,
        containers=[k8s.obj.container(name="controller", image=image)],
        labels={"app": name},
    )))

def generate_script(name, ns, image, replicas, crd_yaml):
    printf("#!/usr/bin/env kite-cloud\n")
    printf("# Generated deployment script for %s\n\n", name)

    if crd_yaml:
        printf("# Apply CRD\n")
        printf("k8s.apply(yaml.decode(\"\"\"%s\"\"\"))\n\n", crd_yaml)

    printf("# Create namespace\n")
    printf("k8s.apply(k8s.obj.namespace(name=\"%s\"))\n\n", ns)

    printf("# ServiceAccount\n")
    printf("k8s.apply(k8s.obj.service_account(name=\"%s\", namespace=\"%s\"))\n\n", name, ns)

    printf("# ClusterRole (permissive — tighten for production)\n")
    printf("k8s.apply(k8s.obj.cluster_role(\n")
    printf("    name=\"%s\",\n", name)
    printf("    rules=[k8s.obj.policy_rule(api_groups=[\"*\"], resources=[\"*\"], verbs=[\"*\"])],\n")
    printf("))\n\n")

    printf("# ClusterRoleBinding\n")
    printf("k8s.apply(k8s.obj.cluster_role_binding(\n")
    printf("    name=\"%s\",\n", name)
    printf("    role_ref={\"apiGroup\": \"rbac.authorization.k8s.io\", \"kind\": \"ClusterRole\", \"name\": \"%s\"},\n", name)
    printf("    subjects=[k8s.obj.subject(kind=\"ServiceAccount\", name=\"%s\", namespace=\"%s\")],\n", name, ns)
    printf("))\n\n")

    printf("# Deploy controller\n")
    printf("k8s.apply(k8s.obj.deployment(\n")
    printf("    name=\"%s\", namespace=\"%s\", replicas=%d,\n", name, ns, replicas)
    printf("    containers=[k8s.obj.container(name=\"controller\", image=\"%s\")],\n", image)
    printf("    labels={\"app\": \"%s\"},\n", name)
    printf("))\n\n")

    printf("printf(\"Controller %s deployed to %s\\n\", \"%s\", \"%s\")\n", name, ns, name, ns)

main()
