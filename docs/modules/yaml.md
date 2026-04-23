---
title: "yaml"
description: "YAML encoding, decoding, and file I/O"
weight: 6
---

The `yaml` module provides YAML encoding and decoding, with support for multi-document YAML files.

## Module-Level Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `yaml.encode(value)` | `string` | Encode a value to a YAML string |
| `yaml.encode_all(list)` | `string` | Encode a list of values to a multi-document YAML string |
| `yaml.decode(string)` | `any` | Decode a YAML string to a value |
| `yaml.decode_all(string)` | `list` | Decode a multi-document YAML string to a list of values |

## Factory Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `yaml.file(path)` | `yaml.file` | Create a file object for reading YAML |
| `yaml.source(data)` | `yaml.writer` | Create a writer object from data |

## yaml.file

Read and decode YAML files, including multi-document files.

```python
f = yaml.file("config.yaml")
```

### Methods and Properties

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `f.decode()` | `any` | Decode the first (or only) document |
| `f.decode_all()` | `list` | Decode all documents in the file |
| `f.try_decode()` | `Result` | Decode the file, returning a Result |
| `f.try_decode_all()` | `Result` | Decode all documents, returning a Result |
| `f.path` | `string` | The file path |

## yaml.writer

Write data to YAML files.

```python
w = yaml.source({"apiVersion": "v1", "kind": "ConfigMap"})
```

### Methods and Properties

| Method / Property | Returns | Description |
|-------------------|---------|-------------|
| `w.write_file(path)` | `None` | Write YAML to a file |
| `w.try_write_file(path)` | `Result` | Write YAML to a file, returning a Result |
| `w.data` | `any` | The underlying data |

## Examples

### Encoding and Decoding

```python
# Encode a dict to YAML
text = yaml.encode({"name": "myapp", "replicas": 3})
print(text)
# name: myapp
# replicas: 3

# Decode a YAML string
data = yaml.decode("name: myapp\nreplicas: 3")
print(data["name"])  # myapp
```

### Multi-Document YAML

```python
# Encode multiple documents
docs = [
    {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": "prod"}},
    {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "config"}},
]
text = yaml.encode_all(docs)
print(text)
# apiVersion: v1
# kind: Namespace
# ...
# ---
# apiVersion: v1
# kind: ConfigMap
# ...

# Decode multiple documents
docs = yaml.decode_all(text)
for doc in docs:
    print(doc["kind"])
```

### Reading YAML Files

```python
# Single document
config = yaml.file("config.yaml").decode()
print(config["database"]["host"])

# Multi-document (e.g. Kubernetes manifests)
manifests = yaml.file("k8s/app.yaml").decode_all()
for manifest in manifests:
    print(manifest["kind"], manifest["metadata"]["name"])

# With error handling
result = yaml.file("config.yaml").try_decode()
if result.ok:
    config = result.value
else:
    print("Failed:", result.error)
```

### Writing YAML Files

```python
config = {
    "server": {"host": "0.0.0.0", "port": 8080},
    "database": {"host": "localhost", "port": 5432},
}

yaml.source(config).write_file("config.yaml")
```

### Round-Trip Editing

```python
# Read, modify, write
config = yaml.file("values.yaml").decode()
config["image"]["tag"] = "v2.1.0"
config["replicas"] = 5
yaml.source(config).write_file("values.yaml")
```

> **Note:**
All `yaml` functions and methods that can fail support `try_` variants. For example, `yaml.try_decode(s)`, `yaml.try_file(path)`, `f.try_decode()`, and `w.try_write_file(path)` return a `Result` instead of raising an error.

