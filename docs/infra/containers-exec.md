---
title: "Executing commands"
description: "Run in-container processes with ExecResult and handle execution environments"
weight: 4
---

# Executing commands

Starkite allows executing commands directly inside running containers without SSH daemon overhead, returning structured execution results with captured output streams and exit codes.

## In-Container Execution (`exec`)

Use `container.exec()` to dispatch a process inside an already running container:

```python
def main():
    client = containers.config()
    c = client.run("alpine:latest", command=["sleep", "300"], detach=True)
    defer(lambda: c.remove(force=True))

    # Run command inside container
    res = c.exec(["echo", "Hello from Starkite"])
    print("Exit code:", res.exit_code)
    print("Stdout:", res.stdout.strip())
    print("Success:", res.ok)
```

### Execution Parameters

`container.exec(command, env=None, user=None, workdir=None)`:

| Parameter | Type | Default | Description |
|:---|:---|:---|:---|
| `command` | `str` \| `list[str]` | *Required* | Command and arguments to execute. |
| `env` | `dict[str, str]` | `None` | Environment variables for the process execution (`{"KEY": "VAL"}`). |
| `user` | `str` | `""` | User or UID to run as (e.g., `"root"`, `"1000"`). |
| `workdir` | `str` | `""` | Working directory inside the container. |

---

## Processing Execution Results (`ExecResult`)

The `container.exec()` method returns an `ExecResult` object exposing the following attributes:

* **`.exit_code`** (*int*): The process termination exit code (`0` for clean exit).
* **`.stdout`** (*str*): The standard output text captured from the process.
* **`.stderr`** (*str*): The standard error text captured from the process.
* **`.ok`** (*bool*): Boolean flag indicating whether the process succeeded (`exit_code == 0`).

`ExecResult` evaluates directly to truthy in conditional checks when `ok` is `True`:

```python
def check_service(c):
    res = c.exec(["pg_isready", "-U", "postgres"])
    if res:
        print("Database service is ready")
    else:
        print("Service check failed:", res.stderr)
```

---

## Environment Variables and Working Directory

You can customize process execution settings without modifying container configurations:

```python
def run_build(c):
    # Execute build script in a custom directory with specific environment variables
    res = c.exec(
        command = ["make", "build"],
        workdir = "/workspace/app",
        env = {
            "GOOS": "linux",
            "CGO_ENABLED": "0",
        },
        user = "builduser",
    )

    if not res:
        fail("Build failed (exit %d): %s" % (res.exit_code, res.stderr))

    print("Build succeeded:", res.stdout)
```

---

## Practical Recipe: Database Readiness and Schema Migration

This recipe starts a database container and uses `c.exec()` to poll for readiness, initialize a table schema, and verify data directly inside the container:

```python
def main():
    client = containers.config()
    pg = client.run(
        image = "postgres:16-alpine",
        env = {"POSTGRES_PASSWORD": "secretpassword", "POSTGRES_DB": "appdb"},
        detach = True,
    )
    defer(lambda: pg.remove(force=True))

    # Poll until postgres process inside container is accepting connections
    print("Waiting for PostgreSQL ready status...")
    retry.with_backoff(
        lambda: pg.exec(["pg_isready", "-U", "postgres", "-d", "appdb"]),
        attempts = 10,
        initial = "500ms",
    )

    # Execute schema creation inside container using psql
    create_sql = "CREATE TABLE users (id SERIAL PRIMARY KEY, username VARCHAR(50));"
    init_res = pg.exec(["psql", "-U", "postgres", "-d", "appdb", "-c", create_sql])
    if not init_res:
        fail("Schema creation failed: " + init_res.stderr)
    print("Schema initialized successfully.")

    # Insert test record
    insert_sql = "INSERT INTO users (username) VALUES ('alice');"
    pg.exec(["psql", "-U", "postgres", "-d", "appdb", "-c", insert_sql])

    # Query verification
    query_res = pg.exec(["psql", "-U", "postgres", "-d", "appdb", "-t", "-c", "SELECT count(*) FROM users;"])
    print("User count verified:", query_res.stdout.strip())
```

Run with:
```bash
kite run ./verify_db.star --permissions=allow-local
```

---

## Permissions

Executing commands inside containers requires the `containers.write` capability.

This capability is included in the `allow-local` and `allow-all` profiles:

```bash
kite run ./exec_cmd.star --permissions=allow-local
```
