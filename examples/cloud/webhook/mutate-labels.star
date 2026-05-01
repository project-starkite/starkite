#!/usr/bin/env cloudkite
# mutate-labels.star — Inject default labels into all deployments
#
# Usage:
#   cloudkite run mutate-labels.star \
#       --var tls_cert=/tmp/cert.pem --var tls_key=/tmp/key.pem

def mutate(obj):
    # Get or create labels via dict-style access
    labels = obj["metadata"]["labels"]
    labels["managed-by"] = "starkite"
    return obj

tls_cert = var_str("tls_cert", "/certs/tls.crt")
tls_key = var_str("tls_key", "/certs/tls.key")

printf("Starting mutation webhook on :9443...\n")
k8s.webhook("/mutate",
    mutate = mutate,
    port = 9443,
    tls_cert = tls_cert,
    tls_key = tls_key,
)
