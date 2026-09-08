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

def test_integration_exec():
    """Verify live command execution, environment passing, and exit code capture inside running container."""
    client = _get_client()
    name = "starkite-test-exec"
    _safe_cleanup(client, name)

    cnt = client.run(
        image = "alpine:latest",
        name = name,
        command = ["sleep", "30"],
        detach = True,
    )

    # 1. Successful execution
    res = cnt.exec(["echo", "hello-from-exec"])
    assert(type(res) == "containers.ExecResult", "exec should return ExecResult")
    assert(res.ok == True, "exec should succeed")
    assert(res.exit_code == 0, "exit code should be 0")
    assert("hello-from-exec" in res.stdout, "stdout should contain echo output")

    # 2. Non-zero exit code
    res_fail = cnt.exec(["sh", "-c", "exit 42"])
    assert(res_fail.ok == False, "res.ok should be False for non-zero exit")
    assert(res_fail.exit_code == 42, "exit code should be 42")

    # 3. Environment variables
    res_env = cnt.exec(["sh", "-c", "echo $GREETING"], env={"GREETING": "starkite-rocks"})
    assert(res_env.ok == True, "env exec should succeed")
    assert("starkite-rocks" in res_env.stdout, "stdout should reflect passed env var")

    # Clean up
    cnt.stop(timeout=2)
    cnt.remove(force=True)

def test_integration_logs():
    """Verify stdout and stderr log capture from container via io.reader stream handle."""
    client = _get_client()
    name = "starkite-test-logs"
    _safe_cleanup(client, name)

    cnt = client.run(
        image = "alpine:latest",
        name = name,
        command = ["sh", "-c", "echo 'hello-stdout'; >&2 echo 'hello-stderr'"],
        detach = False,
    )

    reader = cnt.logs()
    assert(type(reader) == "io.reader", "logs should return io.reader")
    text = reader.text()
    assert("hello-stdout" in text, "logs should contain stdout line")
    assert("hello-stderr" in text, "logs should contain stderr line")

    # Clean up
    cnt.remove(force=True)

def test_integration_images():
    """Verify client.images() returns list of local images from live daemon."""
    client = _get_client()
    imgs = client.images()
    assert(type(imgs) == "list", "client.images() should return a list")
    assert(len(imgs) > 0, "daemon should have at least one local image (e.g. alpine)")

    img = imgs[0]
    assert(type(img) == "dict", "each image summary should be a dictionary")
    assert("id" in img or "Id" in img, "image should have id/Id")
    assert("repo_tags" in img or "RepoTags" in img, "image should have repo_tags")
    assert("size" in img or "Size" in img, "image should have size")

def test_integration_pull():
    """Verify client.pull() pulls image from registry or verifies existing cache."""
    client = _get_client()
    # Pull alpine:latest which is fast and reliable
    client.pull("docker.io/library/alpine:latest")

def test_integration_prune():
    """Verify client.prune() executes cleanup against live daemon and returns report."""
    client = _get_client()
    rep = client.prune(containers=True)
    assert(type(rep) == "dict", "prune should return a dictionary")
    assert("containers_deleted" in rep, "report should have containers_deleted")
    assert("space_reclaimed" in rep, "report should have space_reclaimed")


