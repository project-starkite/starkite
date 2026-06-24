---
title: "JSON & YAML"
description: "Encode, decode, and read/write JSON and YAML"
weight: 30
---

# JSON & YAML

Starkite scripts use the `json` and `yaml` modules to parse and serialize structured configuration data. Both modules share an identical API for converting strings, reading files, and writing data to disk. As pure-compute modules, they require no security permissions unless they are reading or writing files directly on the host filesystem.

## Encoding and decoding strings

To convert Starlark values to strings or parse serialized data in memory, use the `encode` and `decode` functions. These operations convert data to and from native Starlark lists and dictionaries:

```python
# Serialize Starlark data to JSON/YAML strings
json_text = json.encode({"host": "localhost", "port": 8080})
yaml_text = yaml.encode({"replicas": 3})

# Parse JSON/YAML strings back to Starlark dictionaries
json_data = json.decode('{"host":"localhost","port":8080}')
yaml_data = yaml.decode("replicas: 3")

# Index parsed values directly as Starlark dictionaries
print(json_data["host"])  # Outputs: localhost
```

These in-memory functions are commonly used to process API response payloads, command outputs, or dynamically generated configurations.

## Reading and writing files

To interact with files on the filesystem, use the `file` and `source` helper factories:

* **`file(path)`**: Opens a file reader. Call `.decode()` on the returned file object to read and parse its contents.
* **`source(data)`**: Opens a data writer. Call `.write_file(path)` on the returned writer object to serialize the data and write it to disk.

### Examples

```python
# Read and parse a JSON file from disk (requires read permission)
package = json.file("package.json").decode()
print("Project name: " + package["name"])

# Write a dictionary to a formatted JSON file (requires write permission)
settings = {"database": {"host": "db.example.com", "port": 5432}}
json.source(settings).write_file("settings.json", indent="  ")

# Write a dictionary to a block-formatted YAML file
yaml.source(settings).write_file("settings.yaml")
```

The optional `indent` parameter on `json.source().write_file()` defines the indentation spacing for pretty-printed output.

### Pretty-printing JSON in memory

To format or pretty-print a JSON string directly in memory without writing to a file, call the `.encode()` method on a JSON source writer:

```python
data = {"status": "active", "code": 200}

# Generates a formatted JSON string with two-space indentation
pretty_json = json.source(data).encode(indent="  ")
```

## Handling multi-document YAML files

Kubernetes manifests and cloud configurations often combine multiple resource definitions into a single file separated by `---`. The `yaml` module provides specialized functions to handle these multi-document streams:

* **`yaml.file(path).decode_all()`**: Reads a file from disk and returns a list containing one parsed Starlark dictionary per document.
* **`yaml.decode_all(string)`**: Parses an in-memory YAML string containing multiple documents and returns a list of parsed dictionaries.
* **`yaml.encode_all(list)`**: Serializes a list of Starlark dictionaries into a single multi-document YAML string separated by `---`.

### Example: Reading and writing multi-resource manifests

```python
# Read all resource definitions from a single Kubernetes manifest file
resources = yaml.file("manifest.yaml").decode_all()
for res in resources:
    print("Resource Kind: " + res["kind"])

# Modify a value and re-encode all documents to a new file
resources[0]["metadata"]["labels"]["env"] = "production"
yaml.source(resources).write_file("manifest-prod.yaml")
```

When a list of dictionaries is passed to `yaml.source()`, the writer automatically serializes them as a multi-document stream separated by `---`.

## Failure handling

By default, syntax errors or missing files will raise a Starlark-level execution error and halt the script. To inspect and handle errors programmatically, use the `try_` variants which return a `Result` object containing `.ok`, `.value`, and `.error` attributes:

```python
# Attempt to read a JSON file safely
result = json.file("optional-config.json").try_decode()

if result.ok:
    config = result.value
    print("Config loaded successfully")
else:
    print("Failed to load config: " + result.error)
```

## See also

* [`json` API reference](../references/api/json.md) — Detailed function signatures and properties.
* [`yaml` API reference](../references/api/yaml.md) — Detailed function signatures and properties.

