---
title: "fleet"
description: "Compute resource fleet management, topology querying, and executor targeting"
weight: 25
---

# `fleet`

The `fleet` module represents collections of compute resources (servers, nodes, virtual machines, Kubernetes pods, or containers) and their metadata. Fleets model infrastructure topology and integrate directly with executors like `ssh` and `k8s`.

---

## Module Functions

| Function | Returns | Description |
|---|---|---|
| `fleet.file(path)` | `Fleet` | Load a fleet from a static YAML or JSON file. |
| `fleet.new(source)` | `Fleet` | Construct a fleet from an in-memory list, callable function, or JSON string. |
| `fleet.from_source(source)` | `Fleet` | Alias for `fleet.new(source)`. |
| `fleet.of(source)` | `Fleet` | Alias for `fleet.new(source)`. |

---

## Fleet Object Methods & Properties

A `Fleet` instance exposes the following attributes and methods:

| Member | Type | Description |
|---|---|---|
| `f.count` | `int` | Number of compute resources in the fleet. |
| `f.items` | `list[dict]` | All resources as Starlark dictionaries. |
| `f.filter(**kwargs)` | `Fleet` | Return a filtered subset matching exact keyword attributes. |
| `f.filter(predicate_fn)` | `Fleet` | Return a filtered subset where `predicate_fn(item)` is truthy. |
| `f.group_by(key)` | `dict[string, Fleet]` | Group resources by attribute into a dictionary of sub-fleets. |
| `f.addresses(key="address")` | `list[string]` | Extract a list of network IP addresses or hostnames. |
| `f.names()` | `list[string]` | Extract a list of resource names. |
| `f.ids()` | `list[string]` | Extract a list of resource IDs. |
| `f.first()` | `dict` \| `None` | The first resource dictionary, or `None` if the fleet is empty. |

---

## Examples

### 1. Construct from a Static File

```python
servers = fleet.file("infrastructure/hosts.yaml")
print("Total servers:", servers.count)

for host in servers.items:
    print(host["name"], host["address"], host["labels"])
```

### 2. Construct from In-Memory Lists

```python
# From structured dictionary list
web_fleet = fleet.new([
    {"name": "web-1", "address": "10.0.1.10", "role": "web", "zone": "us-east-1a"},
    {"name": "web-2", "address": "10.0.1.11", "role": "web", "zone": "us-east-1b"},
])

# From plain IP/hostname strings
raw_fleet = fleet.new([
    "192.168.1.10",
    "192.168.1.11",
])
```

### 3. Construct from Discovery Functions

```python
def query_cmdb():
    resp = http.url("http://cmdb.corp.local/api/v1/servers").get()
    return resp.json()["data"]

cloud_fleet = fleet.from_source(query_cmdb)
```

### 4. Query and Filter Fleets

```python
servers = fleet.file("hosts.yaml")

# Keyword filtering
prod_web = servers.filter(role="web", env="production")
print("Production web servers:", prod_web.count)

# Predicate function filtering
large_nodes = servers.filter(lambda s: s.get("cpu", 0) >= 8)
```

### 5. Group by Attribute

```python
servers = fleet.file("hosts.yaml")
by_role = servers.group_by("role")

for role, sub_fleet in by_role.items():
    print(role, ":", sub_fleet.count, "nodes")
```

### 6. Target with SSH Executor

```python
servers = fleet.file("hosts.yaml")
web_nodes = servers.filter(role="web")

# Pass fleet directly to SSH client
client = ssh.config(
    fleet       = web_nodes,
    user        = "deploy",
    key         = "~/.ssh/id_ed25519",
    exec_policy = "concurrent",
)

results = client.exec("uptime")
for r in results:
    print(r.host, ":", r.stdout.strip())
```
