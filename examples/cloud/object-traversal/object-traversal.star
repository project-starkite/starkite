#!/usr/bin/env kite
# object-traversal.star - Deep inspection, dot notation, and dictionary operations
#
# Demonstrates how starkite represents Kubernetes resources as AttrDict objects,
# supporting recursive dot notation, dictionary indexing, iteration, in-place
# mutation, and seamless JSON/YAML serialization.
#
# Usage:
#   kite run examples/cloud/object-traversal/object-traversal.star
#   kite run examples/cloud/object-traversal/object-traversal.star --var namespace=kube-system

def main():
    ns = var_str("namespace", "kube-system")
    k = k8s.config(namespace=ns)

    printf("Listing pods in namespace '%s'...\n", ns)
    pods = k.list("pods")

    if not pods:
        printf("No pods found in namespace %s\n", ns)
        return

    pod = pods[0]

    # --- 1. Recursive Dot Notation ---------------------------------------------
    printf("\n[1] Recursive Dot-Notation Access:\n")
    printf("  Kind:              %s\n", pod.kind)
    printf("  API Version:       %s\n", pod.apiVersion)
    printf("  Pod Name:          %s\n", pod.metadata.name)
    printf("  Namespace:         %s\n", pod.metadata.namespace)
    printf("  Node Name:         %s\n", pod.spec.nodeName)
    printf("  Status Phase:      %s\n", pod.status.phase)
    if pod.spec.containers:
        printf("  Primary Container: %s (image: %s)\n",
            pod.spec.containers[0].name,
            pod.spec.containers[0].image)

    # --- 2. Starlark Mapping & Bracket Indexing -------------------------------
    printf("\n[2] Starlark Mapping & Bracket Indexing:\n")
    printf("  pod['kind']:               %s\n", pod["kind"])
    printf("  pod['metadata']['name']:   %s\n", pod["metadata"]["name"])
    printf("  pod.get('apiVersion'):     %s\n", pod.get("apiVersion"))
    printf("  pod.get('missing', 'none'): %s\n", pod.get("missing", "none"))

    # --- 3. IterableMapping Iteration & Length ---------------------------------
    printf("\n[3] IterableMapping & Dict Methods:\n")
    printf("  Total top-level keys (%d): %s\n", len(pod), ", ".join(list(pod.keys())))

    labels = pod.metadata.get("labels")
    if labels:
        printf("  Labels (%d total):\n", len(labels))
        for key, val in labels.items():
            printf("    - %s = %s\n", key, val)
    else:
        printf("  (no labels)\n")

    # --- 4. In-Place Mutation --------------------------------------------------
    printf("\n[4] In-Place Mutation via Mapping Indexing:\n")
    if not pod.metadata.get("labels"):
        pod["metadata"]["labels"] = {}
    pod["metadata"]["labels"]["starkite.io/audited"] = "true"
    printf("  Updated label via dot access: %s\n", pod.metadata.labels["starkite.io/audited"])

    # --- 5. Serialization & Dict Conversion ------------------------------------
    printf("\n[5] Seamless Serialization:\n")
    encoded_json = json.encode({"name": pod.metadata.name, "phase": pod.status.phase})
    printf("  JSON Summary: %s\n", encoded_json)

    as_dict = pod.to_dict()
    printf("  Exported native dict type: %s\n", type(as_dict))
