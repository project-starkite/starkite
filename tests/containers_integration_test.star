# containers_integration_test.star — Integration tests for Starkite containers module against local Podman / Docker
# Run: kite test tests/containers_integration_test.star --permissions=allow-all

def _get_client():
    """Initializes container client, skipping if neither Docker nor Podman is reachable."""
    client = containers.config()
    res = client.try_ping()
    if not res.ok or not res.value:
        skip("container engine (Docker or Podman) is not running or reachable")
    return client

def _safe_cleanup(client, name):
    """Safely removes an existing container if present to guarantee test idempotency."""
    containers_list = client.list(all=True)
    for cnt in containers_list:
        if cnt.name == name or cnt.name == "/" + name:
            cnt.remove(force=True)

# ============================================================================
# Integration Tests
# ============================================================================

def test_integration_ping_and_version():
    """Verify live connectivity and API version reporting from local Podman engine."""
    client = _get_client()
    assert(client.ping() == True, "client.ping() should return True on live daemon")

    ver = client.version()
    assert(type(ver) == "dict", "version should return a dictionary")
    assert("Version" in ver, "version dict should contain 'Version'")
    assert("ApiVersion" in ver, "version dict should contain 'ApiVersion'")

def test_integration_container_lifecycle():
    """Verify full container lifecycle (create -> start -> inspect -> stop -> wait -> remove)."""
    client = _get_client()
    name = "starkite-test-lifecycle"
    _safe_cleanup(client, name)

    # 1. Create
    cnt = client.create(
        image = "alpine:latest",
        name = name,
        command = ["sleep", "30"],
        env = {"TEST_ENV": "active"},
    )
    assert(type(cnt) == "containers.Container", "create should return containers.Container")
    assert(cnt.id != "", "container id should be populated")
    assert(cnt.name == name, "container name should match")
    assert(cnt.status == "created", "initial status should be created")

    # 2. Start
    cnt.start()
    assert(cnt.status == "running", "status should transition to running")

    # 3. Inspect
    info = cnt.inspect()
    assert(type(info) == "dict", "inspect should return a dict")
    assert(info["State"]["Running"] == True, "container state should report running")

    # 4. Stop
    cnt.stop(timeout=2)
    assert(cnt.status == "exited", "status should transition to exited")

    # 5. Wait
    exit_code = cnt.wait()
    assert(type(exit_code) == "int", "wait should return integer exit code")

    # 6. Remove
    cnt.remove(force=True)
    assert(cnt.status == "removed", "status should transition to removed")

def test_integration_run_detached():
    """Verify client.run(..., detach=True) starts and returns running container."""
    client = _get_client()
    name = "starkite-test-run-detached"
    _safe_cleanup(client, name)

    cnt = client.run(
        image = "alpine:latest",
        name = name,
        command = ["sleep", "20"],
        detach = True,
    )
    assert(cnt.id != "", "container id should be populated")
    assert(cnt.status == "running", "detached run should leave container running")

    # Clean up
    cnt.stop(timeout=2)
    cnt.remove(force=True)

def test_integration_run_synchronous():
    """Verify client.run(..., detach=False) runs container to completion and waits."""
    client = _get_client()
    name = "starkite-test-run-sync"
    _safe_cleanup(client, name)

    cnt = client.run(
        image = "alpine:latest",
        name = name,
        command = ["echo", "hello-from-starkite"],
        detach = False,
    )
    assert(cnt.id != "", "container id should be populated")

    # Clean up
    cnt.remove(force=True)

def test_integration_list_and_get():
    """Verify container discovery via client.list() and direct lookup via client.get()."""
    client = _get_client()
    name = "starkite-test-list-target"
    _safe_cleanup(client, name)

    cnt = client.create(
        image = "alpine:latest",
        name = name,
        command = ["sleep", "10"],
    )

    # 1. List
    all_cnts = client.list(all=True)
    found = False
    for item in all_cnts:
        if item.id == cnt.id or item.name == name or item.name == "/" + name:
            found = True
            break
    assert(found == True, "created container should be discovered in client.list(all=True)")

    # 2. Get
    retrieved = client.get(cnt.id)
    assert(type(retrieved) == "containers.Container", "get should return containers.Container")
    assert(retrieved.id == cnt.id, "retrieved container id must match")

    # Clean up
    cnt.remove(force=True)

def test_integration_port_mapping():
    """Verify dynamic port allocation and lookup via container.port()."""
    client = _get_client()
    name = "starkite-test-port-mapping"
    _safe_cleanup(client, name)

    cnt = client.create(
        image = "alpine:latest",
        name = name,
        command = ["sleep", "20"],
        ports = {"8080/tcp": 0},
    )
    cnt.start()

    host_port = cnt.port("8080/tcp")
    assert(type(host_port) == "int", "host port should be an integer")
    assert(host_port > 0, "host port should be non-zero allocated port")

    # Clean up
    cnt.stop(timeout=2)
    cnt.remove(force=True)
