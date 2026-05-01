#!/usr/bin/env cloudkite
# configmap-sync.star — Watches ConfigMaps and logs all changes
#
# Usage:
#   cloudkite run examples/cloud/controller/configmap-sync.star
#   cloudkite run examples/cloud/controller/configmap-sync.star --var namespace=my-ns

ns = var_str("namespace", "default")

def on_create(obj):
    data = obj.data
    keys = len(data) if data != None else 0
    printf("[CREATED] ConfigMap %s/%s with %d keys\n", obj.metadata.namespace, obj.metadata.name, keys)

def on_update(old, new):
    printf("[UPDATED] ConfigMap %s/%s\n", new.metadata.namespace, new.metadata.name)

def on_delete(obj):
    printf("[DELETED] ConfigMap %s/%s\n", obj.metadata.namespace, obj.metadata.name)

printf("Watching ConfigMaps in namespace %s...\n", ns)
k8s.control("configmaps",
    on_create = on_create,
    on_update = on_update,
    on_delete = on_delete,
    namespace = ns,
)
