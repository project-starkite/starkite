---
title: "Container management"
description: "Create, run, inspect, stop, restart, wait, remove, and stream logs from containers with dynamic port allocation"
weight: 3
---

# Container management

Once connected to a container daemon, the `containers` module provides Starlark APIs to manage container lifecycles, configure resource constraints, inspect state, and stream logs.

## Creating and Running Containers

The `Client` object provides two methods for container provisioning:

* `client.run(...)`: Atomically creates and starts the container.
* `client.create(...)`: Allocates container resources without starting it, returning a `Container` handle ready for `.start()`.

```python
def main():
    client = containers.config()

    # Create and start a Redis container
    redis = client.run(
        image = "redis:7-alpine",
        name = "app-cache",
        ports = {"6379/tcp": 0},      # Allocate dynamic ephemeral host port
        memory = "256MB",             # Hard memory limit
        detach = True,                # Run in background
    )

    print("Container ID:", redis.id[:12])
    print("Container Name:", redis.name)
    print("Container Status:", redis.status)
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

The `Container` handle returned by `run()`, `create()`, or `get()` provides control methods:

```python
def manage_state(c):
    # Stop container (sends SIGTERM, then SIGKILL after timeout seconds)
    c.stop(timeout = 10)

    # Start stopped container
    c.start()

    # Restart container
    c.restart(timeout = 5)

    # Block until container stops and retrieve exit code
    exit_code = c.wait(condition = "not-running")
    print("Process exited with code:", exit_code)

    # Remove container from daemon
    c.remove(force = True, volumes = True)
```

### Container Properties

* `.id` (*str*): Unique 64-character container SHA ID.
* `.name` (*str*): Assigned container name (without leading `/`).
* `.image` (*str*): Image reference used to run the container.
* `.status` (*str*): Current status string (`"running"`, `"created"`, `"exited"`, etc.).

---

## Dynamic Port Resolution

When running integration test fixtures or parallel CI runners, hardcoding host ports risks collision. Pass `0` as the host port in `ports` to let the engine allocate an ephemeral host port:

```python
def main():
    client = containers.config()

    pg = client.run(
        image = "postgres:16-alpine",
        env = {"POSTGRES_PASSWORD": "secretpassword"},
        ports = {"5432/tcp": 0},  # Request ephemeral host port
        detach = True,
    )

    # Resolve dynamic host port
    host_port = pg.port("5432/tcp")
    print("PostgreSQL allocated host port:", host_port)

    # Connect to database using resolved port
    dsn = "postgres://postgres:secretpassword@localhost:%d/postgres?sslmode=disable" % host_port
    print("DSN:", dsn)

    pg.remove(force = True)
```

`container.port(port_spec)` accepts `"5432/tcp"` or `"5432"` and returns the resolved host port as an integer.

---

## Listing and Inspecting Containers

### Listing Containers

* `client.list(all=False)`: Returns a list of `Container` handles for running containers. Pass `all=True` to include stopped and created containers.

```python
def main():
    client = containers.config()

    for c in client.list(all=True):
        print("Container:", c.id[:12], c.name, c.status)
```

### Retrieving and Inspecting

* `client.get(id_or_name)`: Retrieves an existing container handle by ID or name.
* `container.inspect()`: Returns the complete raw daemon inspection dictionary, including `State`, `Config`, `NetworkSettings`, and `HostConfig`.

```python
def main():
    client = containers.config()
    c = client.get("app-cache")

    info = c.inspect()
    print("Running:", info["State"]["Running"])
    print("IP Address:", info["NetworkSettings"]["IPAddress"])
```

---

## Streaming Container Logs

The `container.logs()` method streams log output from a container, returning an `io.reader` compatible with Starkite's `io` standard library module.

### Automatic Multiplexing Demux

Docker and Podman multiplex stdout and stderr streams over a single connection using an 8-byte binary framing protocol:

```
[ STREAM_TYPE (1 byte) ] [ 0x00 0x00 0x00 (3 bytes) ] [ FRAME_SIZE (4 bytes uint32) ]
```

Starkite's socket transport automatically decodes and strips these binary headers in real time, delivering a clean, readable text stream to your script without raw binary frame corruption.

```python
def main():
    client = containers.config()
    c = client.run("alpine:latest", command=["sh", "-c", "echo 'starting'; sleep 1; echo 'done'"])
    defer(lambda: c.remove(force=True))

    # Read container log stream
    reader = c.logs(stdout=True, stderr=True, tail="50")
    print(reader.read_all())
```

### Log Stream Parameters

`container.logs(follow=False, tail="all", stdout=True, stderr=True)`:

| Parameter | Type | Default | Description |
|:---|:---|:---|:---|
| `follow` | `bool` | `False` | Stream logs continuously as they arrive. |
| `tail` | `str` | `"all"` | Number of log lines to retrieve from the end (e.g., `"100"`, `"all"`). |
| `stdout` | `bool` | `True` | Include standard output in stream. |
| `stderr` | `bool` | `True` | Include standard error in stream. |

---

## Practical Recipes

### Ephemeral Test Database

This recipe starts an ephemeral PostgreSQL container on a dynamic host port, waits for readiness using `retry.with_backoff`, executes database queries, and guarantees cleanup with `defer()`:

```python
def setup_test_db(client):
    pg = client.run(
        image = "postgres:16-alpine",
        env = {"POSTGRES_PASSWORD": "secretpassword", "POSTGRES_DB": "testdb"},
        ports = {"5432/tcp": 0},
        detach = True,
    )

    host_port = pg.port("5432/tcp")
    dsn = "postgres://postgres:secretpassword@localhost:%d/testdb?sslmode=disable" % host_port

    # Wait for database readiness
    retry.with_backoff(
        lambda: sql.open("postgres", dsn).exec("SELECT 1"),
        attempts = 10,
        initial = "500ms",
    )

    return pg, dsn

def main():
    client = containers.config()
    pg, dsn = setup_test_db(client)
    defer(lambda: pg.remove(force=True))

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
    client = containers.config()

    containers_list = client.list(all=True)
    print("Inspecting", len(containers_list), "containers...")

    for c in containers_list:
        data = c.inspect()
        state = data.get("State", {})
        status = state.get("Status", "unknown")
        exit_code = state.get("ExitCode", 0)

        if status == "exited" and exit_code != 0:
            print("Container '%s' failed with exit code %d" % (c.name, exit_code))
            print("--- Last 20 log lines ---")
            log_reader = c.logs(tail="20", stdout=True, stderr=True)
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
* `containers.manage`: `remove()`.

All of these capabilities are included in the `allow-local` profile:

```bash
kite run ./manage.star --permissions=allow-local
```
