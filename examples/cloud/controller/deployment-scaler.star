#!/usr/bin/env kite-cloud
# deployment-scaler.star — Enforces max replicas on labeled deployments
#
# Watches deployments with the label "enforce-max-replicas=true" and
# scales them down if they exceed the configured maximum.
#
# Usage:
#   kite-cloud run examples/cloud/controller/deployment-scaler.star
#   kite-cloud run examples/cloud/controller/deployment-scaler.star --var max_replicas=5

max_replicas = var_int("max_replicas", 3)

def reconcile(event, obj):
    if event == "DELETED":
        printf("[DELETED] %s/%s\n", obj.metadata.namespace, obj.metadata.name)
        return

    replicas = obj.spec.replicas
    if replicas == None:
        replicas = 1

    if replicas > max_replicas:
        printf("[SCALE DOWN] %s/%s from %d to %d replicas\n",
            obj.metadata.namespace, obj.metadata.name, replicas, max_replicas)
        k8s.apply({
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": obj.metadata.name,
                "namespace": obj.metadata.namespace,
            },
            "spec": {"replicas": max_replicas},
        })
    else:
        printf("[OK] %s/%s has %d replicas (max: %d)\n",
            obj.metadata.namespace, obj.metadata.name, replicas, max_replicas)

printf("Enforcing max %d replicas on deployments with label enforce-max-replicas=true...\n", max_replicas)
k8s.control("deployments",
    reconcile = reconcile,
    labels = "enforce-max-replicas=true",
    resync = "1m",
)
