---
title: "Orchestrating Fleets"
description: "Manage compute resources and multi-host infrastructure with fleet, ssh, and k8s"
weight: 50
---

# Orchestrating Fleets

The `fleet` module represents collections of compute resources (nodes, virtual machines, Kubernetes pods, or containers) and their metadata. Fleets decouple resource discovery and topology from execution transports like `ssh` and `k8s`.

---

## Architecture Overview

1. **Topology & Discovery (`fleet`)**: Ingests resources from static files, in-memory lists, discovery functions, POSIX hosts files, or Kubernetes clusters. Provides filtering and grouping.
2. **Execution Transports (`ssh`, `k8s`)**: Active executors consume `Fleet` instances directly (e.g., `ssh.config(fleet=web_fleet)`).

```
 ┌──────────────────────────────────────────────┐
 │                  Fleet Type                  │
 │      (Compute Resource Collection)           │
 │                                              │
 │  - Resources ([]Resource)                    │
 │  - Querying & Subsetting (.filter, .group_by)│
 │  - Extraction (.addresses, .names, .items)   │
 └──────────────────────┬───────────────────────┘
                        │
        ┌───────────────┴───────────────┐
        │                               │
        ▼ (Constructed By)              ▼ (Consumed By)
 ┌──────────────────────────────┐ ┌──────────────────────────────┐
 │ • fleet.new(...)             │ │ • ssh.config(fleet=f)        │
 │ • fleet.file("hosts.yaml")   │ │ • k8s.exec(fleet=f, ...)     │
 │ • fleet.hosts_file("/etc/..")│ │ • Custom automation loops    │
 │ • k8s.client.fleet(...)      │ └──────────────────────────────┘
 └──────────────────────────────┘
```

---

## 1. Fleet Constructors

### Unified Factory (`fleet.new`)

`fleet.new()` is the canonical factory for constructing a Fleet:

```python
# 1. From an in-memory list of dictionaries
cluster = fleet.new([
    {"name": "picluster-0", "address": "192.168.10.100", "role": "control-plane"},
    {"name": "picluster-1", "address": "192.168.10.101", "role": "worker"},
    {"name": "picluster-2", "address": "192.168.10.102", "role": "worker"},
])

# 2. From plain host IP / hostname strings
edge_nodes = fleet.new([
    "10.0.1.10",
    "10.0.1.11",
    "10.0.1.12",
])

# 3. From a discovery function / lambda
def discover_from_cmdb():
    resp = http.url("http://cmdb.corp.local/api/v1/hosts").get()
    return resp.json()["data"]

cloud_fleet = fleet.new(function=discover_from_cmdb)

# 4. From a static YAML or JSON file
file_fleet = fleet.new(file="infrastructure/hosts.yaml")

# 5. From a POSIX hosts file
hosts_fleet = fleet.new(hosts_file="/etc/hosts")

# 6. Empty fleet
empty_fleet = fleet.new()
```

---

### Static Configuration Files (`fleet.file`)

Load server metadata from a YAML or JSON file:

```yaml
# hosts.yaml
- name: web-prod-1
  address: 192.168.10.11
  env: production
  role: web
  zone: us-east-1a
- name: web-prod-2
  address: 192.168.10.12
  env: production
  role: web
  zone: us-east-1b
- name: db-prod-1
  address: 192.168.10.21
  env: production
  role: db
  zone: us-east-1a
```

```python
servers = fleet.file("hosts.yaml")
print("Total servers:", servers.count)
```

---

### POSIX Hosts Files (`fleet.hosts_file`)

Directly ingest and discover compute resources from standard POSIX `/etc/hosts` files. Useful for private LANs, edge devices, and bastion-proxied clusters:

```text
# /etc/hosts
127.0.0.1      localhost
192.168.10.100 picluster-0 picluster-0.local master
192.168.10.101 picluster-1 picluster-1.local worker-1
192.168.10.102 picluster-2 picluster-2.local worker-2
```

```python
# Discovers cluster nodes from /etc/hosts (loopback excluded by default)
cluster = fleet.hosts_file()

# Filter out specific nodes
workers = cluster.filter(lambda h: h["name"] != "picluster-0")
print("Workers count:", workers.count)
```

---

### Kubernetes Clusters (`k8s.client.fleet`)

Construct a fleet of Pods or Nodes directly from a Kubernetes cluster:

```python
k = k8s.config()

# Produce a fleet of worker nodes
node_fleet = k.fleet(
    kind = "Node",
    labels = {"node-role.kubernetes.io/worker": ""},
)

# Produce a fleet of application pods
pod_fleet = k.fleet(
    kind = "Pod",
    namespace = "production",
    labels = {"app": "web"},
)
```

---

## 2. Filtering & Grouping Fleets

### Filtering by Keywords
```python
# Filter by exact attribute match
prod_web = servers.filter(env="production", role="web")
print("Production web servers:", prod_web.count)
```

### Filtering by Predicate Function
```python
# Custom predicate function (e.g. at least 8 CPUs)
heavy_nodes = servers.filter(lambda s: s.get("cpu", 0) >= 8)
```

### Grouping Fleets
```python
# Group by role into a dictionary of sub-fleets
by_role = servers.group_by("role")
web_fleet = by_role["web"]
db_fleet = by_role["db"]
```

---

## 3. Extracting Attributes

| Method | Return Type | Description |
|---|---|---|
| `f.count` | `int` | Number of compute resources in the fleet. |
| `f.items` | `list[dict]` | All resources as Starlark dictionaries. |
| `f.addresses(key="address")` | `list[string]` | List of IP addresses / hostnames. |
| `f.names()` | `list[string]` | List of resource names. |
| `f.ids()` | `list[string]` | List of resource IDs. |
| `f.first()` | `dict` \| `None` | The first resource, or `None` if empty. |

---

## 4. End-to-End Orchestration with SSH

Pass a `Fleet` instance directly to `ssh.config(fleet=...)`:

```python
# deploy.star

# 1. Discover cluster nodes from local /etc/hosts
cluster = fleet.hosts_file()

# 2. Subset workers
workers = cluster.filter(lambda h: h["name"].startswith("picluster-") and h["name"] != "picluster-0")

# 3. Configure concurrent SSH client targeting the worker sub-fleet
client = ssh.config(
    fleet       = workers,
    auth        = {
        "user": "deploy",
        "key":  "~/.ssh/id_ed25519",
    },
    jump        = {
        "host": "bastion.corp.local",
    },
    exec_policy = "concurrent",
)

# 4. Execute command across all worker nodes concurrently
results = client.exec("uptime")

# 5. Format results
t = table.new(["HOST", "STATUS", "OUTPUT"])
for r in results:
    t.add_row(r.host, "OK" if r.ok else "FAIL", r.stdout.strip())
print(t.render())
```

Execute locally:

```bash
kite run ./deploy.star
```
