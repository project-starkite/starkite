---
title: "Managing workloads"
description: "Deploy, scale, autoscale, and inspect container workloads in Kubernetes"
weight: 12
---

# Managing workloads

The `k8s` module provides high-level APIs to deploy and manage containerized workloads in a cluster. By utilizing these APIs, you can automate deployment pipelines, perform scaling actions, configure autoscalers, and monitor resource consumption.

## Deploying Workloads

The `client.deploy()` function provides a unified interface to deploy containerized workloads. In a single API call, it creates a Kubernetes `Deployment` and an optional `ClusterIP` Service, matching standard application patterns.

```python
def main():
    # Configure the client for the target namespace
    client = k8s.config(namespace="staging")
    
    # Deploy an application and expose it on port 80
    result = client.deploy(
        name = "alice-web",
        image = "nginx:1.27",
        replicas = 3,
        port = 80,
        labels = {"app": "alice-web", "team": "platform"},
    )
    
    # The function returns an AttrDict with the names of the created resources
    print("Created Deployment:", result.deployment)
    print("Created Service:", result.service)
```

## Scaling Workloads

To adjust the capacity of a workload, use the `client.scale()` function. This directly updates the replica count of the target controller (such as a Deployment or StatefulSet).

```python
def scale_app():
    client = k8s.config(namespace="staging")
    
    # Scale the deployment to 5 replicas
    client.scale(
        kind = "deployment",
        name = "alice-web",
        replicas = 5,
    )
    print("Workload scaled to 5 replicas.")
```

## Autoscaling Workloads

For dynamic scaling based on resource demand, use `client.autoscale()`. This configures a `HorizontalPodAutoscaler` (HPA) targeting the deployment.

```python
def configure_autoscaling():
    client = k8s.config(namespace="staging")
    
    # Attach an HPA targeting 70% CPU utilization
    client.autoscale(
        kind = "deployment",
        name = "alice-web",
        min = 2,
        max = 10,
        cpu_percent = 70,
    )
    print("Horizontal Pod Autoscaler configured.")
```

## Inspecting Workload Metrics

To monitor the performance of your running workloads, use the `client.top_pods()` function. It retrieves real-time CPU and memory utilization details from the cluster's Metric Server.

```python
def check_resource_metrics():
    client = k8s.config(namespace="staging")
    
    # Query pod resource metrics sorted by CPU usage
    metrics = client.top_pods(sort_by="cpu", timeout="10s")
    
    print("Active Resource Consumption:")
    for pod in metrics:
        if "alice-web" in pod.name:
            print(
                "Pod Name:", pod.name,
                "CPU request:", pod.cpu_request,
                "Status:", pod.status,
            )
```
