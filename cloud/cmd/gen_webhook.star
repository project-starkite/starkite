# Default webhook artifact generator
# Executed by: kite-cloud kube gen-webhook-artifacts

def main():
    webhook_script = var_str("webhook_script", "webhook.star")
    name = var_str("webhook_name", "webhook")
    image = var_str("image")
    ns = var_str("namespace", "default")
    rules_json = var_str("rules_json", "[]")
    output_format = var_str("output_format", "yaml")

    rules = json.decode(rules_json)

    if output_format == "yaml":
        generate_yaml(name, ns, image, webhook_script, rules)
    elif output_format == "script":
        generate_script(name, ns, image, webhook_script, rules)

def generate_yaml(name, ns, image, webhook_script, rules):
    # Namespace
    printf("---\n%s", k8s.yaml(k8s.obj.namespace(name=ns)))

    # ServiceAccount
    printf("---\n%s", k8s.yaml(k8s.obj.service_account(name=name, namespace=ns)))

    # Deployment with TLS volume
    printf("---\n")
    printf("apiVersion: apps/v1\n")
    printf("kind: Deployment\n")
    printf("metadata:\n")
    printf("  name: %s\n", name)
    printf("  namespace: %s\n", ns)
    printf("spec:\n")
    printf("  replicas: 1\n")
    printf("  selector:\n")
    printf("    matchLabels:\n")
    printf("      app: %s\n", name)
    printf("  template:\n")
    printf("    metadata:\n")
    printf("      labels:\n")
    printf("        app: %s\n", name)
    printf("    spec:\n")
    printf("      serviceAccountName: %s\n", name)
    printf("      containers:\n")
    printf("        - name: webhook\n")
    printf("          image: %s\n", image)
    printf("          command: [\"kite-cloud\", \"run\", \"/app/%s\"]\n", webhook_script)
    printf("          ports:\n")
    printf("            - containerPort: 9443\n")
    printf("          volumeMounts:\n")
    printf("            - name: tls\n")
    printf("              mountPath: /certs\n")
    printf("              readOnly: true\n")
    printf("      volumes:\n")
    printf("        - name: tls\n")
    printf("          secret:\n")
    printf("            secretName: %s-tls\n", name)

    # Service
    printf("---\n")
    printf("apiVersion: v1\n")
    printf("kind: Service\n")
    printf("metadata:\n")
    printf("  name: %s\n", name)
    printf("  namespace: %s\n", ns)
    printf("spec:\n")
    printf("  ports:\n")
    printf("    - port: 443\n")
    printf("      targetPort: 9443\n")
    printf("  selector:\n")
    printf("    app: %s\n", name)

    # TLS Secret placeholder
    printf("---\n")
    printf("# TLS certificate placeholder — replace with real certs or use cert-manager\n")
    printf("apiVersion: v1\n")
    printf("kind: Secret\n")
    printf("metadata:\n")
    printf("  name: %s-tls\n", name)
    printf("  namespace: %s\n", ns)
    printf("type: kubernetes.io/tls\n")
    printf("data:\n")
    printf("  tls.crt: <base64-encoded-cert>\n")
    printf("  tls.key: <base64-encoded-key>\n")

    # ValidatingWebhookConfiguration
    if rules:
        printf("---\n")
        printf("apiVersion: admissionregistration.k8s.io/v1\n")
        printf("kind: ValidatingWebhookConfiguration\n")
        printf("metadata:\n")
        printf("  name: %s\n", name)
        printf("webhooks:\n")
        printf("  - name: %s.%s.svc\n", name, ns)
        printf("    clientConfig:\n")
        printf("      service:\n")
        printf("        name: %s\n", name)
        printf("        namespace: %s\n", ns)
        printf("        path: /validate\n")
        printf("      caBundle: <base64-encoded-ca>\n")
        printf("    rules:\n")
        for rule in rules:
            printf("      - apiGroups: %s\n", json.encode(rule.get("apiGroups", ["*"])))
            printf("        apiVersions: %s\n", json.encode(rule.get("apiVersions", ["*"])))
            printf("        resources: %s\n", json.encode(rule.get("resources", ["*"])))
            printf("        operations: %s\n", json.encode(rule.get("operations", ["CREATE", "UPDATE"])))
            printf("        scope: \"*\"\n")
        printf("    admissionReviewVersions: [\"v1\"]\n")
        printf("    sideEffects: None\n")
        printf("    failurePolicy: Fail\n")

def generate_script(name, ns, image, webhook_script, rules):
    printf("#!/usr/bin/env kite-cloud\n")
    printf("# Generated webhook deployment script for %s\n\n", name)

    printf("# Create namespace\n")
    printf("k8s.apply(k8s.obj.namespace(name=\"%s\"))\n\n", ns)

    printf("# ServiceAccount\n")
    printf("k8s.apply(k8s.obj.service_account(name=\"%s\", namespace=\"%s\"))\n\n", name, ns)

    printf("# Deployment\n")
    printf("k8s.apply({\n")
    printf("    \"apiVersion\": \"apps/v1\",\n")
    printf("    \"kind\": \"Deployment\",\n")
    printf("    \"metadata\": {\"name\": \"%s\", \"namespace\": \"%s\"},\n", name, ns)
    printf("    \"spec\": {\n")
    printf("        \"replicas\": 1,\n")
    printf("        \"selector\": {\"matchLabels\": {\"app\": \"%s\"}},\n", name)
    printf("        \"template\": {\n")
    printf("            \"metadata\": {\"labels\": {\"app\": \"%s\"}},\n", name)
    printf("            \"spec\": {\n")
    printf("                \"serviceAccountName\": \"%s\",\n", name)
    printf("                \"containers\": [{\n")
    printf("                    \"name\": \"webhook\",\n")
    printf("                    \"image\": \"%s\",\n", image)
    printf("                    \"command\": [\"kite-cloud\", \"run\", \"/app/%s\"],\n", webhook_script)
    printf("                    \"ports\": [{\"containerPort\": 9443}],\n")
    printf("                    \"volumeMounts\": [{\"name\": \"tls\", \"mountPath\": \"/certs\", \"readOnly\": True}],\n")
    printf("                }],\n")
    printf("                \"volumes\": [{\"name\": \"tls\", \"secret\": {\"secretName\": \"%s-tls\"}}],\n", name)
    printf("            },\n")
    printf("        },\n")
    printf("    },\n")
    printf("})\n\n")

    printf("# Service\n")
    printf("k8s.apply(k8s.obj.service(\n")
    printf("    name=\"%s\", namespace=\"%s\",\n", name, ns)
    printf("    port=443, target_port=9443,\n")
    printf("    labels={\"app\": \"%s\"},\n", name)
    printf("))\n\n")

    printf("printf(\"Webhook %s deployed to %s\\n\", \"%s\", \"%s\")\n", name, ns, name, ns)
    printf("printf(\"NOTE: Update %s-tls Secret with real TLS certificates\\n\")\n", name)

main()
