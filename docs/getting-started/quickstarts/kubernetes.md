---
title: "Deploy to Kubernetes"
description: "Use the k8s module to deploy and scale workloads, plus a Job manifest for running starkite as a pod"
weight: 9
---

# Deploy to Kubernetes

Two halves: scripting Kubernetes operations from a starkite script (`k8s` module — cloud edition), and running starkite scripts inside a Kubernetes Job from the published image.

## Script Kubernetes from starkite

**Source:** [`examples/cloud/quick-deploy/quick-deploy.star`](https://github.com/project-starkite/starkite/blob/main/examples/cloud/quick-deploy/quick-deploy.star) (excerpted)

```python
#!/usr/bin/env kite

ns = var_str("namespace", "default")
image = var_str("image", "nginx:1.27")
name = var_str("app.name", "web")
replicas = var_int("replicas", 3)

k = k8s.config(namespace=ns)

# Creates a Deployment + ClusterIP Service, waits for rollout.
result = k.deploy(name, image,
    replicas=replicas,
    port=80,
    labels={"team": "platform"})

printf("Deployment: %s\n", result["deployment"])
printf("Service:    %s (ClusterIP)\n", result["service"])

# Scale up.
k.scale("deployment", name, replicas + 2)

# Add an HPA targeting 70% CPU.
k.autoscale("deployment", name, min=replicas, max=(replicas + 2) * 2, cpu_percent=70)
```

Run it (requires `kitecloud` or `kite`):

```bash
kite run examples/cloud/quick-deploy/quick-deploy.star
kite run examples/cloud/quick-deploy/quick-deploy.star --var image=myapp:v2
```

## Run a starkite script inside Kubernetes

A one-shot Job that runs `/scripts/main.star` from a ConfigMap-mounted directory:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: kite-run
spec:
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: kite
          image: ghcr.io/project-starkite/kite:v0.1.0
          args: ["run", "/scripts/main.star"]
          volumeMounts:
            - name: scripts
              mountPath: /scripts
              readOnly: true
      volumes:
        - name: scripts
          configMap:
            name: my-scripts
```

For a one-off run:

```bash
kubectl run --rm -it kite \
  --image=ghcr.io/project-starkite/kite:latest \
  -- exec 'print("hi")'
```

## See also

- [Kubernetes](../../kubernetes/index.md) — the cloud-edition landing: controllers, webhooks, examples
- [`k8s` reference](../../references/api/k8s.md) — full three-tier API (CRUD · kubectl-equivalents · typed constructors)
- [Cloud examples on GitHub](https://github.com/project-starkite/starkite/tree/main/examples/cloud) — controllers, multi-env stacks, redis-cluster, wordpress-stack, more
