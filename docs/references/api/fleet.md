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
| `fleet.new(**kwargs \| source)` | `Fleet` | Canonical factory constructor. Constructs a fleet from a file, hosts file, list, function, or JSON string. |
| `fleet.file(path)` | `Fleet` | Load a fleet from a static YAML or JSON file. Alias for `fleet.new(file=path)`. |
| `fleet.hosts_file(path="/etc/hosts", loopback=False)` | `Fleet` | Ingest compute resources from a standard POSIX hosts file. |
| `fleet.host_file(path="/etc/hosts", loopback=False)` | `Fleet` | Alias for `fleet.hosts_file()`. |

---

## `fleet.new` Keyword Parameters

`fleet.new()` accepts either a single positional source argument or explicit keyword arguments:

| Parameter | Type | Default | Description |
|---|---|---|---|
| `source` | `list` \| `callable` \| `string` \| `dict` | `None` | Generic compute source. |
| `file` | `string` | `""` | Path to static YAML or JSON file to ingest. |
| `hosts_file` | `string` \| `bool` | `""` | Path to POSIX hosts file (or `True` for `"/etc/hosts"`). |
| `loopback` | `bool` | `False` | When parsing hosts files, whether to include `127.0.0.1` and `::1`. |
| `list` | `list[dict]` \| `list[string]` | `None` | Explicit list of resource dictionaries or address strings. |
| `function` | `callable` | `None` | Explicit discovery function or lambda returning resources. |
| `json` | `string` | `""` | Explicit raw JSON string payload. |

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

### 2. Construct from POSIX Hosts File

```python
# Ingest cluster nodes from /etc/hosts (loopback excluded)
cluster = fleet.hosts_file()

# Custom hosts file with loopback included
all_hosts = fleet.hosts_file("infrastructure/lan_hosts", loopback=True)
```

### 3. Construct from In-Memory Lists

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

### 4. Construct from Discovery Functions

```python
def query_cmdb():
    resp = http.url("http://cmdb.corp.local/api/v1/servers").get()
    return resp.json()["data"]

cloud_fleet = fleet.new(function=query_cmdb)
```

### 5. Query and Filter Fleets

```python
servers = fleet.file("hosts.yaml")

# Keyword filtering
prod_web = servers.filter(role="web", env="production")
print("Production web servers:", prod_web.count)

# Predicate function filtering
large_nodes = servers.filter(lambda s: s.get("cpu", 0) >= 8)
```

### 6. Group by Attribute

```python
servers = fleet.file("hosts.yaml")
by_role = servers.group_by("role")

for role, sub_fleet in by_role.items():
    print(role, ":", sub_fleet.count, "nodes")
```

### 7. Target with SSH Executor

```python
cluster = fleet.hosts_file()
workers = cluster.filter(lambda h: h["name"].startswith("picluster-"))

# Pass fleet directly to SSH client
client = ssh.config(
    fleet       = workers,
    auth        = {
        "user": "deploy",
        "key":  "~/.ssh/id_ed25519",
    },
    exec_policy = "concurrent",
)

results = client.exec("uptime")
for r in results:
    print(r.host, ":", r.stdout.strip())
```
