---
title: "Container management"
description: "Create, run, inspect, stop, restart, wait, delete, and stream logs from containers using AttrDict representations and client dispatchers"
weight: 3
---

# Container management

Once connected to a container daemon, the `containers` module provides Starlark APIs to manage container lifecycles, configure resource constraints, inspect state, and stream logs using a module-based hybrid pattern aligned with `k8s` and `ssh`.

## Creating and Running Containers

All container provisioning methods anchor on a configured client instance (`containers.config()`) and return a pure-data dictionary with attribute dot-access (`AttrDict`):

* `client.run(...)`: Atomically creates and starts the container, returning its `AttrDict`.
* `client.create(...)`: Allocates container resources without starting it, returning its `AttrDict` in `"created"` status.

```python
def main():
    dockr = containers.config()

    # Create and start a Redis container
    box = dockr.run(
        image = "redis:7-alpine",
        name = "app-cache",
        ports = {"6379/tcp": 0},      # Allocate dynamic ephemeral host port
        memory = "256MB",             # Hard memory limit
        detach = True,                # Run in background
    )

    # Dot-access or dictionary indexing
    print("Container ID:", box.id[:12])
    print("Container Name:", box.name)
    print("Container Status:", box.status)
    print("Container Ports:", box.ports)
```

### Provisioning Parameters

| Parameter | Type | Default | Description |
|:---|:---|:---|:---|
| `image` | `str` | *Required* | OCI image reference (e.g., `"redis:7-alpine"`). |
| `name` | `str` | `""` | Optional container name. |
| `command` | `str` \| `list[str]` | `None` | Process command and arguments. |
| `env` | `dict[str, str]` \| `list[str]` | `None` | Environment variables (`{"KEY": "VAL"}` or `["KEY=VAL"]`). |
| `ports` | `dict[str, int \| str]` | `None` | Port mappings (e.g., `{"5432/tcp": 0}` for dynamic host port, `{"80/tcp": 8080}`). |
| `volumes` | `dict[str, str]` | `None` | Bind mounts mapping `{"host_path": "container_path"}`. |
| `detach` | `bool` | `True` | Run in background. If `False`, blocks until container exits. |
| `network` | `str` | `""` | Network mode (e.g., `"bridge"`, `"host"`, `"none"`). |
| `cpu` | `str` \| `float` \| `int` | `None` | CPU limit (e.g., `"1.5"`, `1.5`, or nanoCPUs). |
| `memory` | `str` \| `int` | `None` | Memory limit (e.g., `"512MB"`, `"1GB"`, or byte count). |
| `auto_remove` | `bool` | `False` | Automatically delete container upon termination. |

---

## State Control Operations

All lifecycle verbs are methods on the configured client `dockr`. Each verb accepts **either** the container `AttrDict` directly or a string container name/ID:

```python
def manage_state(dockr, box):
    # Stop container (sends SIGTERM, then SIGKILL after timeout seconds)
    dockr.stop(box, timeout = 10)

    # Start stopped container
    dockr.start(box)

    # Restart container
    dockr.restart(box, timeout = 5)

    # Block until container stops and retrieve exit code
    exit_code = dockr.wait(box, condition = "not-running")
    print("Process exited with code:", exit_code)

    # Delete container from daemon (alias: dockr.remove)
    dockr.delete(box, force = True, volumes = True)

    # String identifiers are supported interchangeably:
    dockr.start("app-cache")
    dockr.stop("app-cache", timeout = 5)
    dockr.delete("app-cache", force = True)
```

### Container `AttrDict` Properties

* `.id` (*str*): Unique 64-character container SHA ID.
* `.name` (*str*): Assigned container name (without leading `/`).
* `.image` (*str*): Image reference used to run the container.
* `.status` (*str*): Current status string (`"running"`, `"created"`, `"exited"`, `"removed"`).
* `.ports` (*dict*): Dictionary of port bindings.

### Zero-Friction Serialization

Because `box` is a pure dictionary with **zero attached Go methods or network transports**, it serializes directly to JSON and YAML:

```python
# Export container description to disk
json_manifest = json.encode(box)
yaml_manifest = yaml.encode(box)
fs.path("container.json").write(json_manifest)
```

---

## Selective Module-Scope Shortcuts

For quick one-liners using the ambient default daemon without calling `containers.config()`, the `containers` module exports selective shortcuts:

* `containers.run(image, ...)`: Creates and starts a container using default daemon socket discovery.
* `containers.exec(target, command, ...)`: Dispatches a command inside a running container.
* `containers.stop(target, timeout=10)`: Stops a container.
* `containers.delete(target, force=False)` / `containers.remove(...)`: Deletes a container.

```python
def main():
    # Quick one-liner run
    box = containers.run("alpine:latest", command=["sleep", "60"], detach=True)

    # Command execution
    res = containers.exec(box, ["echo", "hello"])
    print(res.stdout)

    # Quick shutdown and cleanup
    containers.stop(box, timeout=2)
    containers.delete(box, force=True)
```

---

## Dynamic Port Resolution

When running integration test fixtures or parallel CI runners, hardcoding host ports risks collision. Pass `0` as the host port in `ports` to let the engine allocate an ephemeral host port:

```python
def main():
    dockr = containers.config()

    pg = dockr.run(
        image = "postgres:16-alpine",
        env = {"POSTGRES_PASSWORD": "secretpassword"},
        ports = {"5432/tcp": 0},  # Request ephemeral host port
        detach = True,
    )

    # Resolve dynamic host port via client
    host_port = dockr.port(pg, "5432/tcp")
    print("PostgreSQL allocated host port:", host_port)

    # Connect to database using resolved port
    dsn = "postgres://postgres:secretpassword@localhost:%d/postgres?sslmode=disable" % host_port
    print("DSN:", dsn)

    dockr.delete(pg, force = True)
```

`dockr.port(target, port_spec)` accepts `"5432/tcp"`, `"5432"`, or `5432` and returns the resolved host port as an integer.

---

## Listing and Inspecting Containers

### Listing Containers

* `dockr.list(all=False)`: Returns a list of `AttrDict` objects for running containers. Pass `all=True` to include stopped and created containers.

```python
def main():
    dockr = containers.config()

    for c in dockr.list(all=True):
        print("Container:", c.id[:12], c.name, c.status)
```

### Retrieving and Inspecting

* `dockr.get(id_or_name)`: Retrieves an existing container `AttrDict` by ID or name.
* `dockr.inspect(target)`: Returns the complete raw daemon inspection `AttrDict`, including `State`, `Config`, `NetworkSettings`, and `HostConfig`.

```python
def main():
    dockr = containers.config()
    c = dockr.get("app-cache")

    info = dockr.inspect(c)
    print("Running:", info.State.Running)
    print("IP Address:", info.NetworkSettings.IPAddress)
```

---

## Streaming Container Logs

The `dockr.logs()` method streams log output from a container, returning an `io.reader` compatible with Starkite's `io` standard library module.

### Automatic Multiplexing Demux

Docker and Podman multiplex stdout and stderr streams over a single connection using an 8-byte binary framing protocol:

```
[ STREAM_TYPE (1 byte) ] [ 0x00 0x00 0x00 (3 bytes) ] [ FRAME_SIZE (4 bytes uint32) ]
```

Starkite's socket transport automatically decodes and strips these binary headers in real time, delivering a clean, readable text stream without binary header corruption.

```python
def main():
    dockr = containers.config()
    c = dockr.run("alpine:latest", command=["sh", "-c", "echo 'starting'; sleep 1; echo 'done'"])
    defer(lambda: dockr.delete(c, force=True))

    # Read container log stream
    reader = dockr.logs(c, stdout=True, stderr=True, tail="50")
    print(reader.read_all())
```

### Log Stream Parameters

`dockr.logs(target, follow=False, tail="all", stdout=True, stderr=True)`:

| Parameter | Type | Default | Description |
|:---|:---|:---|:---|
| `target` | `AttrDict` \| `dict` \| `str` | *Required* | Target container instance or string name/ID. |
| `follow` | `bool` | `False` | Stream logs continuously as they arrive. |
| `tail` | `str` | `"all"` | Number of log lines to retrieve from the end (e.g., `"100"`, `"all"`). |
| `stdout` | `bool` | `True` | Include standard output in stream. |
| `stderr` | `bool` | `True` | Include standard error in stream. |

---

## Practical Recipes

### Ephemeral Test Database

This recipe starts an ephemeral PostgreSQL container on a dynamic host port, waits for readiness using `retry.with_backoff`, executes database queries, and guarantees cleanup with `defer()`:

```python
def setup_test_db(dockr):
    pg = dockr.run(
        image = "postgres:16-alpine",
        env = {"POSTGRES_PASSWORD": "secretpassword", "POSTGRES_DB": "testdb"},
        ports = {"5432/tcp": 0},
        detach = True,
    )

    host_port = dockr.port(pg, "5432/tcp")
    dsn = "postgres://postgres:secretpassword@localhost:%d/testdb?sslmode=disable" % host_port

    # Wait for database readiness
    retry.with_backoff(
        lambda: sql.open("postgres", dsn).exec("SELECT 1"),
        attempts = 10,
        initial = "500ms",
    )

    return pg, dsn

def main():
    dockr = containers.config()
    pg, dsn = setup_test_db(dockr)
    defer(lambda: dockr.delete(pg, force=True))

    db = sql.open("postgres", dsn)
    db.exec("CREATE TABLE health_check (id SERIAL PRIMARY KEY, service TEXT)")
    db.exec("INSERT INTO health_check (service) VALUES ('billing-service')")

    rows = db.query("SELECT service FROM health_check")
    print("Database test passed:", rows[0]["service"])
```

Run with:
```bash
kite run ./test_db.star --permissions=allow-local
```

### Container Health Diagnostic and Log Inspection

This recipe iterates over all containers on the host, identifies containers that exited with a non-zero exit code, and extracts their last 20 log lines for troubleshooting:

```python
def main():
    dockr = containers.config()

    containers_list = dockr.list(all=True)
    print("Inspecting", len(containers_list), "containers...")

    for c in containers_list:
        data = dockr.inspect(c)
        state = data.get("State", {})
        status = state.get("Status", "unknown")
        exit_code = state.get("ExitCode", 0)

        if status == "exited" and exit_code != 0:
            print("Container '%s' failed with exit code %d" % (c.name, exit_code))
            print("--- Last 20 log lines ---")
            log_reader = dockr.logs(c, tail="20", stdout=True, stderr=True)
            print(log_reader.read_all())
            print("-------------------------")
```

Run with:
```bash
kite run ./diagnose.star --permissions=allow-local
```

---

## Permissions

Container management operations require the following capabilities:

* `containers.read`: `list()`, `get()`, `inspect()`, `port()`, `logs()`.
* `containers.write`: `create()`, `run()`, `start()`, `stop()`, `restart()`, `wait()`.
* `containers.manage`: `delete()` / `remove()`.

All of these capabilities are included in the `allow-local` profile:

```bash
kite run ./manage.star --permissions=allow-local
```
