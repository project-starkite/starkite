---
title: "inventory"
description: "Inventory management for host lists and infrastructure data"
weight: 25
---

The `inventory` module loads, filters, groups, and merges inventory data from structured files (YAML/JSON). Inventories represent collections of hosts, services, or other infrastructure entities.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `inventory.file(path)` | `InventoryValue` | Load an inventory from a YAML or JSON file |
| `inventory.list(inventory)` | `list[dict]` | Return all items as a list of dictionaries |
| `inventory.filter(inventory, func=None, **kwargs)` | `InventoryValue` | Filter items by a predicate function or keyword match |
| `inventory.group_by(inventory, key)` | `dict` | Group items by a key, returning `{value: InventoryValue}` |
| `inventory.merge(*inventories)` | `InventoryValue` | Merge multiple inventories into one |
| `inventory.addresses(inventory, key="address")` | `list` | Extract a list of addresses from inventory items |

## InventoryValue

| Property | Type | Description |
|----------|------|-------------|
| `count` | `int` | Number of items in the inventory |
| `items` | `list[dict]` | The underlying list of items |

## Examples

### Load and list inventory

```python
inv = inventory.file("hosts.yaml")
print("Total hosts:", inv.count)

for host in inventory.list(inv):
    print(host["name"], host["address"])
```

### Filter by keyword

```python
inv = inventory.file("hosts.yaml")

# Filter by keyword arguments
prod_hosts = inventory.filter(inv, env="production")
print("Production hosts:", prod_hosts.count)
```

### Filter by function

```python
inv = inventory.file("hosts.yaml")

# Filter by predicate function
large = inventory.filter(inv, func=lambda h: h.get("cpu", 0) >= 8)
for host in large.items:
    print(host["name"], "cpus:", host["cpu"])
```

### Group by key

```python
inv = inventory.file("hosts.yaml")
groups = inventory.group_by(inv, "region")

for region, hosts in groups.items():
    print(region, ":", hosts.count, "hosts")
```

### Merge inventories

```python
web = inventory.file("web-hosts.yaml")
db = inventory.file("db-hosts.yaml")
all_hosts = inventory.merge(web, db)
print("All hosts:", all_hosts.count)
```

### Extract addresses

```python
inv = inventory.file("hosts.yaml")
addrs = inventory.addresses(inv)
print("Addresses:", addrs)

# Custom address key
addrs = inventory.addresses(inv, key="ip")
```

### Combined workflow

```python
inv = inventory.file("fleet.yaml")
prod = inventory.filter(inv, env="production")
by_region = inventory.group_by(prod, "region")

for region, hosts in by_region.items():
    addrs = inventory.addresses(hosts)
    print("Deploying to", region, ":", addrs)
    for addr in addrs:
        result = exec("ssh %s 'systemctl restart app'" % addr)
        if not result.ok:
            log.error("deploy failed", {"host": addr, "error": result.stderr})
```

> **Note:**
All `inventory` functions that can fail support `try_` variants that return a `Result` instead of raising an error.

