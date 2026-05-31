---
title: "JSON & YAML"
description: "Encode, decode, and read/write JSON and YAML"
weight: 30
---

# JSON & YAML

The `json` and `yaml` modules share the same shape: module-level `encode`/`decode` for strings, and `file`/`source` factories for reading and writing files. `yaml` additionally handles multi-document files.

## Encoding and decoding strings

```python
# JSON
text = json.encode({"host": "localhost", "port": 8080})
data = json.decode('{"host":"localhost","port":8080}')
print(data["host"])   # localhost

# YAML
text = yaml.encode({"replicas": 3})
data = yaml.decode("replicas: 3")
```

## Reading files

```python
pkg = json.file("package.json").decode()
print(pkg["name"], pkg["version"])

cfg = yaml.file("config.yaml").decode()
```

A multi-document YAML file (`---`-separated) decodes to a list:

```python
docs = yaml.file("manifests.yaml").decode_all()
for doc in docs:
    print(doc["kind"])
```

## Writing files

```python
config = {
    "database": {"host": "db.example.com", "port": 5432},
}
json.source(config).write_file("config.json", indent="  ")
yaml.source(config).write_file("config.yaml")
```

## Round-trip editing

```python
data = json.file("settings.json").decode()
data["debug"] = False
json.source(data).write_file("settings.json", indent="  ")
```

Every function and method that can fail has a `try_` variant returning a [`Result`](../fundamentals/language.md#error-handling) — `json.try_decode(s)`, `f.try_decode()`, `w.try_write_file(path)`. See the [json](../references/api/json.md) and [yaml](../references/api/yaml.md) references.
