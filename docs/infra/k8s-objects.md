---
title: "Object representation"
description: "Understand how Starkite represents Kubernetes objects, reads fields, and handles mutations"
weight: 15
---

# Object representation

Starkite represents Kubernetes resources using standard Starlark dictionaries and a specialized wrapper called `AttrDict`. This dual representation provides clean read-only traversal and secure read/write mutations.

## Read-Only Access (Dot Notation)

When reading fields from a Kubernetes resource (such as in a controller reconcile loop or a validating webhook), Starkite allows you to use dot notation to traverse nested structures.

```python
def check_deployment_details(obj):
    # Traverses directly into metadata and spec using dot notation
    name = obj.metadata.name
    namespace = obj.metadata.namespace
    replicas = obj.spec.replicas
    
    # Nested arrays are accessed using standard list index
    image = obj.spec.template.spec.containers[0].image
    
    print("Deployment:", name, "Image:", image)
```

### Key Behaviors of Dot Notation
* **Read-Only**: Dot notation is strictly read-only. Attempting to assign a value using dot notation (e.g., `obj.spec.replicas = 5`) raises a Starlark runtime error.
* **Graceful Null Safety**: If a nested field or parent key is missing in the underlying resource, dot notation returns `None` instead of raising a key error. This simplifies checks:
  ```python
  # Safe even if metadata or labels do not exist on the object
  labels = obj.metadata.labels
  if labels == None:
      print("No labels defined")
  ```

---

## Read/Write Access (Bracket Notation)

When you need to modify an existing resource (such as in a mutating webhook) or construct a new manifest from scratch, you must use standard Starlark dictionary bracket notation (`[]`).

```python
def mutate_deployment_labels(obj):
    # Ensure the labels dictionary is initialized
    if obj["metadata"].get("labels") == None:
        obj["metadata"]["labels"] = {}
        
    # Modify fields in-place using bracket notation
    obj["metadata"]["labels"]["managed-by"] = "starkite"
    obj["spec"]["replicas"] = 3
    
    # Return the mutated object
    return obj
```

### Key Behaviors of Bracket Notation
* **Read and Write**: Bracket notation allows both reading and writing of keys.
* **Mutation Propagation**: Nested dictionaries in Starkite share their underlying data. Modifying a key in a nested dictionary (e.g., `obj["spec"]["replicas"] = 3`) propagates the change to the parent object automatically.
* **JSON/YAML Conversion**: When you pass a dictionary to `k8s.apply()` or `yaml.encode()`, Starkite serializes the Starlark dictionary directly into standard JSON or YAML.

---

## Summary of Access Patterns

| Access Style | Syntax Example | Operations | Missing Keys | Common Use Case |
|:---|:---|:---|:---|:---|
| **Dot Notation** | `obj.spec.replicas` | Read-only | Returns `None` | Controllers, Validating Webhooks, Metrics inspection |
| **Bracket Notation** | `obj["spec"]["replicas"] = 3` | Read & Write | Raises error if parent key missing | Mutating Webhooks, Manifest generation |
