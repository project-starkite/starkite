---
title: "JSON & YAML"
description: "Encode, decode, and read/write JSON and YAML"
weight: 30
---

# JSON & YAML

Almost every automation eventually has to read a config file, parse a command's JSON output, or write a manifest back to disk, and the `json` and `yaml` modules are how you do that without leaving Starlark. The two share one shape on purpose: module-level `encode`/`decode` turn values into strings and back, and `file`/`source` factories handle the disk on either end. `yaml` carries one extra capability for the multi-document files that Kubernetes and similar tools emit. Both are pure-compute modules — they transform data already in hand and never touch the filesystem on their own — so the only step that needs a permission is the one that actually reads or writes a file.

## Encoding and decoding strings

Start with data you already hold in memory. `encode` serializes a value to a string, and `decode` parses a string back into a value, so a round trip through either module leaves you with native Starlark dicts and lists:

```python
# JSON
text = json.encode({"host": "localhost", "port": 8080})
data = json.decode('{"host":"localhost","port":8080}')
print(data["host"])   # localhost

# YAML
text = yaml.encode({"replicas": 3})
data = yaml.decode("replicas: 3")
```

After `decode`, `data` is an ordinary dict you index with `data["host"]` — there is no wrapper type to unpack. This is the path to reach for when the bytes come from somewhere other than a file: the body of an HTTP response, the stdout of a command, a string you assembled yourself.

## Reading files

When the data lives on disk, skip the read-then-decode dance and hand the path straight to the module. `json.file(path)` and `yaml.file(path)` return a file object whose `decode()` method reads and parses in one call:

```python
pkg = json.file("package.json").decode()
print(pkg["name"], pkg["version"])

cfg = yaml.file("config.yaml").decode()
```

The result is the same native dict you would get from `decode` on a string; the file object just spares you opening the file yourself. Because this step reads from disk, it is the one call here that needs filesystem permission.

YAML adds a wrinkle that JSON does not: a single file can hold several documents separated by `---`. Calling `decode()` on such a file gives you only the first document, so when you want all of them, reach for `decode_all()`, which returns a list — one entry per document:

```python
docs = yaml.file("manifests.yaml").decode_all()
for doc in docs:
    print(doc["kind"])
```

This is what makes the `yaml` module practical for Kubernetes manifests, where one file routinely bundles a Namespace, a ConfigMap, and a Deployment.

## Writing files

Going the other direction, `source` wraps a value in a writer and `write_file` serializes it to a path. For JSON, the optional `indent` argument controls pretty-printing — pass two spaces and the output is human-readable rather than packed onto one line:

```python
config = {
    "database": {"host": "db.example.com", "port": 5432},
}
json.source(config).write_file("config.json", indent="  ")
yaml.source(config).write_file("config.yaml")
```

YAML writes are already block-formatted, so `yaml.source(...).write_file(...)` takes no indent argument. Both of these calls write to disk, so both need filesystem permission for the target path.

## Round-trip editing

The factories compose into the pattern you will use most: read a file, change a value in the resulting dict, and write the same structure back. Because `decode()` hands you a mutable dict, the edit is a plain assignment, and `source(...).write_file(...)` closes the loop:

```python
data = json.file("settings.json").decode()
data["debug"] = False
json.source(data).write_file("settings.json", indent="  ")
```

Reusing the same `indent` on the way out keeps the file's formatting stable, so a one-key change produces a one-line diff instead of reflowing the whole document.

## Handling failure

Any of these calls can fail — a malformed string, a missing file, an unwritable path — and by default a failure aborts the script. When you would rather inspect the failure than crash on it, every function and method that can fail also has a `try_` variant that returns a [`Result`](../fundamentals/language.md#error-handling) instead of raising: `json.try_decode(s)`, `f.try_decode()`, `w.try_write_file(path)`. Check the `Result` and you decide what a parse error or a missing file means for the run rather than letting it end the script. See the [json](../references/api/json.md) and [yaml](../references/api/yaml.md) references for the full signatures.
