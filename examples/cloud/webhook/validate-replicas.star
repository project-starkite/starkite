#!/usr/bin/env kite-cloud
# validate-replicas.star — Reject deployments with more than 10 replicas
#
# Usage:
#   kite-cloud run validate-replicas.star
#
# Requires TLS certificates mounted at /certs/ (or override paths below)

def validate(obj):
    replicas = obj.spec.replicas
    if replicas != None and replicas > 10:
        return {"allowed": False, "message": "max 10 replicas allowed, got %d" % replicas}

    labels = obj.metadata.labels
    if labels == None or labels.get("team") == None:
        return {"allowed": False, "message": "team label is required"}

    return {"allowed": True}

tls_cert = var_str("tls_cert", "/certs/tls.crt")
tls_key = var_str("tls_key", "/certs/tls.key")

printf("Starting validation webhook on :9443...\n")
k8s.webhook("/validate",
    validate = validate,
    port = 9443,
    tls_cert = tls_cert,
    tls_key = tls_key,
)
