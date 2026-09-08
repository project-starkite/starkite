# containers_test.star — Unit tests for Starkite containers module using mock engine server
# Run: kite test tests/containers_test.star --permissions=allow-all

def _create_mock_engine():
    """Builds an HTTP mock server simulating the Docker / Podman Engine REST API."""
    state = {
        "created": [],
        "started": [],
        "stopped": [],
        "restarted": [],
        "removed": [],
        "pulled": [],
        "pruned": [],
        "wait_code": 0,
    }

    srv = http.server()

    def ping_h(req):
        return "OK"

    def version_h(req):
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({
                "Version": "27.1.1",
                "ApiVersion": "1.45",
                "Platform": {"Name": "Docker Engine - Community"},
            }),
        }

    def create_h(req):
        body = json.decode(req.body)
        name = req.query.get("name", "test-cnt")
        cid = "cnt-" + str(len(state["created"]) + 1)
        state["created"].append({"id": cid, "name": name, "body": body})
        return {
            "status": 201,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({"Id": cid, "Warnings": []}),
        }

    def start_h(req):
        cid = req.params.get("id", "")
        state["started"].append(cid)
        return {"status": 204, "body": ""}

    def stop_h(req):
        cid = req.params.get("id", "")
        timeout = req.query.get("t", "10")
        state["stopped"].append({"id": cid, "timeout": timeout})
        return {"status": 204, "body": ""}

    def restart_h(req):
        cid = req.params.get("id", "")
        timeout = req.query.get("t", "10")
        state["restarted"].append({"id": cid, "timeout": timeout})
        return {"status": 204, "body": ""}

    def wait_h(req):
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({"StatusCode": state["wait_code"]}),
        }

    def delete_h(req):
        cid = req.params.get("id", "")
        force = req.query.get("force", "false")
        v = req.query.get("v", "false")
        state["removed"].append({"id": cid, "force": force, "volumes": v})
        return {"status": 204, "body": ""}

    def inspect_h(req):
        cid = req.params.get("id", "cnt-0001")
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({
                "Id": cid,
                "Name": "/test-app",
                "Config": {
                    "Image": "alpine:latest",
                    "Cmd": ["sleep", "60"],
                    "Env": ["ENV_VAR=prod"],
                },
                "State": {
                    "Status": "running",
                    "Running": True,
                    "ExitCode": 0,
                    "Pid": 1234,
                },
                "NetworkSettings": {
                    "Ports": {
                        "80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}],
                        "5432/tcp": [{"HostIp": "127.0.0.1", "HostPort": "54321"}],
                    },
                },
            }),
        }

    def list_h(req):
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode([
                {
                    "Id": "c1001",
                    "Names": ["/web"],
                    "Image": "nginx:alpine",
                    "State": "running",
                    "Status": "Up 2 hours",
                },
                {
                    "Id": "c1002",
                    "Names": ["/db"],
                    "Image": "postgres:16",
                    "State": "exited",
                    "Status": "Exited (0) 5 minutes ago",
                },
            ]),
        }

    def exec_create_h(req):
        return {
            "status": 201,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({"Id": "exec-001"}),
        }

    def exec_start_h(req):
        return {
            "status": 200,
            "body": "executed successfully\n",
        }

    def exec_inspect_h(req):
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({
                "ID": "exec-001",
                "Running": False,
                "ExitCode": 0,
            }),
        }

    def logs_h(req):
        return {
            "status": 200,
            "body": "app started\nlistening on 8080\n",
        }

    def images_h(req):
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode([
                {
                    "Id": "sha256:img123456",
                    "RepoTags": ["alpine:latest", "alpine:3.19"],
                    "RepoDigests": ["alpine@sha256:digest1"],
                    "Created": 1700000000,
                    "Size": 7340032,
                    "Labels": {"maintainer": "kite"},
                },
            ]),
        }

    def pull_h(req):
        img = req.query.get("fromImage", "")
        auth = req.headers.get("X-Registry-Auth", "")
        state["pulled"].append({"image": img, "auth": auth})
        return {
            "status": 200,
            "body": json.encode({"status": "Pulling from library/alpine"}) + "\n" + json.encode({"status": "Download complete"}) + "\n",
        }

    def containers_prune_h(req):
        state["pruned"].append("containers")
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({
                "ContainersDeleted": ["c_dead1", "c_dead2"],
                "SpaceReclaimed": 4096,
            }),
        }

    def volumes_prune_h(req):
        state["pruned"].append("volumes")
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({
                "VolumesDeleted": ["vol_unused1"],
                "SpaceReclaimed": 8192,
            }),
        }

    def images_prune_h(req):
        state["pruned"].append("images")
        return {
            "status": 200,
            "headers": {"Content-Type": "application/json"},
            "body": json.encode({
                "ImagesDeleted": [{"Deleted": "sha256:dangling1"}],
                "SpaceReclaimed": 16384,
            }),
        }

    srv.handle("GET /_ping", ping_h)
    srv.handle("GET /v1.45/version", version_h)
    srv.handle("POST /v1.45/containers/create", create_h)
    srv.handle("POST /v1.45/containers/{id}/start", start_h)
    srv.handle("POST /v1.45/containers/{id}/stop", stop_h)
    srv.handle("POST /v1.45/containers/{id}/restart", restart_h)
    srv.handle("POST /v1.45/containers/{id}/wait", wait_h)
    srv.handle("DELETE /v1.45/containers/{id}", delete_h)
    srv.handle("GET /v1.45/containers/{id}/json", inspect_h)
    srv.handle("GET /v1.45/containers/json", list_h)
    srv.handle("POST /v1.45/containers/{id}/exec", exec_create_h)
    srv.handle("POST /v1.45/exec/{id}/start", exec_start_h)
    srv.handle("GET /v1.45/exec/{id}/json", exec_inspect_h)
    srv.handle("GET /v1.45/containers/{id}/logs", logs_h)
    srv.handle("GET /v1.45/images/json", images_h)
    srv.handle("POST /v1.45/images/create", pull_h)
    srv.handle("POST /v1.45/containers/prune", containers_prune_h)
    srv.handle("POST /v1.45/volumes/prune", volumes_prune_h)
    srv.handle("POST /v1.45/images/prune", images_prune_h)

    srv.start(port=0)
    client = containers.config(host="http://localhost:%d" % srv.port())
    return srv, client, state

# ============================================================================
# Unit Tests
# ============================================================================

def test_config():
    """Verify containers.config constructor semantics."""
    c1 = containers.config(host="http://localhost:9999")
    assert(type(c1) == "containers.Client", "containers.config should return containers.Client")
    assert(c1.endpoint == "http://localhost:9999", "endpoint attribute should match")
    assert(hasattr(containers, "client") == False, "containers.client should not exist")

def test_ping_and_version():
    """Verify client.ping() and client.version() against mock server."""
    srv, client, state = _create_mock_engine()
    assert(client.ping() == True, "ping should return True")

    ver = client.version()
    assert(ver["Version"] == "27.1.1", "Version should match")
    assert(ver["ApiVersion"] == "1.45", "ApiVersion should match")
    assert(ver["Platform"]["Name"] == "Docker Engine - Community", "Platform name should match")
    srv.shutdown()

def test_container_create():
    """Verify client.create() request translation and container handle attributes."""
    srv, client, state = _create_mock_engine()

    c = client.create(
        image = "postgres:16-alpine",
        name = "db-server",
        command = ["postgres", "-D", "/data"],
        ports = {"5432/tcp": 5432},
        volumes = {"/tmp/pgdata": "/var/lib/postgresql/data:rw"},
        env = {"POSTGRES_PASSWORD": "secretpassword"},
        cpu = "2.0",
        memory = "1g",
        auto_remove = True,
    )

    assert(type(c) == "AttrDict", "create should return AttrDict")
    assert(c.id == "cnt-1", "container id should match mock response")
    assert(c.name == "db-server", "container name should match")
    assert(c.image == "postgres:16-alpine", "container image should match")
    assert(c.status == "created", "initial status should be created")

    assert(len(state["created"]) == 1, "one create request recorded")
    payload = state["created"][0]["body"]
    assert(payload["Image"] == "postgres:16-alpine", "payload Image should match")
    assert(payload["Cmd"] == ["postgres", "-D", "/data"], "payload Cmd should match")
    assert(payload["Env"] == ["POSTGRES_PASSWORD=secretpassword"], "payload Env should match")
    assert(payload["HostConfig"]["AutoRemove"] == True, "AutoRemove should be True")
    assert(payload["HostConfig"]["NanoCPUs"] == 2000000000, "NanoCPUs should be 2.0 -> 2e9")
    assert(payload["HostConfig"]["Memory"] == 1073741824, "Memory should be 1g -> 1073741824")
    assert(payload["HostConfig"]["Binds"] == ["/tmp/pgdata:/var/lib/postgresql/data:rw"], "Binds should match")

    srv.shutdown()

def test_container_start():
    """Verify client.start(target) updates status and issues engine POST."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest", name="app")
    assert(c.status == "created", "should be created")

    client.start(c)
    assert(c.status == "running", "status should transition to running")
    assert(len(state["started"]) == 1, "started should be recorded")
    assert(state["started"][0] == c.id, "started id should match")

    srv.shutdown()

def test_container_stop_and_restart():
    """Verify client.stop() and client.restart() methods and status transitions."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest")
    client.start(c)
    assert(c.status == "running")

    client.stop(c, timeout=15)
    assert(c.status == "exited", "status should transition to exited")
    assert(len(state["stopped"]) == 1, "stop recorded")
    assert(state["stopped"][0]["id"] == c.id)
    assert(state["stopped"][0]["timeout"] == "15", "custom timeout passed in query param")

    client.restart(c, timeout=5)
    assert(c.status == "running", "status should transition back to running")
    assert(len(state["restarted"]) == 1, "restart recorded")
    assert(state["restarted"][0]["id"] == c.id)
    assert(state["restarted"][0]["timeout"] == "5", "custom timeout passed in query param")

    srv.shutdown()

def test_container_wait():
    """Verify client.wait(target) returns engine exit code."""
    srv, client, state = _create_mock_engine()
    state["wait_code"] = 137

    c = client.create(image="alpine:latest")
    client.start(c)
    exit_code = client.wait(c)
    assert(exit_code == 137, "exit code should match wait StatusCode")

    srv.shutdown()

def test_container_remove():
    """Verify client.delete() / client.remove() query params and status transition."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest")

    client.delete(c, force=True, volumes=True)
    assert(c.status == "removed", "status should transition to removed")
    assert(len(state["removed"]) == 1, "remove recorded")
    assert(state["removed"][0]["id"] == c.id)
    assert(state["removed"][0]["force"] == "true", "force param passed")
    assert(state["removed"][0]["volumes"] == "true", "volumes param passed")

    srv.shutdown()

def test_container_inspect():
    """Verify client.inspect(target) returns complete nested state AttrDict."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest")

    info = client.inspect(c)
    assert(type(info) == "AttrDict", "inspect should return an AttrDict")
    assert(info.Id == c.id, "inspect Id should match container id")
    assert(info.Name == "/test-app", "inspect Name should match")
    assert(info.State.Running == True, "State.Running should be True")
    assert(info.Config.Image == "alpine:latest", "Config.Image should match")

    srv.shutdown()

def test_container_port():
    """Verify client.port(target, port) extracts host port mapping."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest")

    port80 = client.port(c, "80/tcp")
    assert(port80 == 8080, "mapped port for 80/tcp should be 8080")

    port80_norm = client.port(c, "80")
    assert(port80_norm == 8080, "client.port('80') should normalize to 80/tcp and return 8080")

    port5432 = client.port(c, "5432/tcp")
    assert(port5432 == 54321, "mapped port for 5432/tcp should be 54321")

    srv.shutdown()

def test_client_run_detached():
    """Verify client.run(..., detach=True) creates and starts container."""
    srv, client, state = _create_mock_engine()

    c = client.run("redis:7-alpine", name="my-redis", detach=True)
    assert(type(c) == "AttrDict", "run should return AttrDict")
    assert(c.status == "running", "detached run should leave container in running status")
    assert(len(state["created"]) == 1, "created recorded")
    assert(len(state["started"]) == 1, "started recorded")
    assert(len(state["stopped"]) == 0, "should not be stopped")

    srv.shutdown()

def test_client_run_synchronous():
    """Verify client.run(..., detach=False) creates, starts, and waits."""
    srv, client, state = _create_mock_engine()
    state["wait_code"] = 0

    c = client.run("alpine:latest", command=["echo", "done"], detach=False)
    assert(type(c) == "AttrDict", "run should return AttrDict")
    assert(len(state["created"]) == 1, "created recorded")
    assert(len(state["started"]) == 1, "started recorded")

    srv.shutdown()

def test_client_get_and_list():
    """Verify client.get() and client.list() container handle population."""
    srv, client, state = _create_mock_engine()

    c = client.get("cnt-target")
    assert(type(c) == "AttrDict", "get should return AttrDict")
    assert(c.id == "cnt-target", "get container id should match")
    assert(c.name == "test-app", "name should be populated from inspect")

    containers_list = client.list(all=True)
    assert(len(containers_list) == 2, "should list 2 containers")
    assert(type(containers_list[0]) == "AttrDict", "list item should be AttrDict")
    assert(containers_list[0].id == "c1001", "first container id matches")
    assert(containers_list[0].name == "web", "first container name stripped of leading slash")
    assert(containers_list[0].image == "nginx:alpine", "first container image matches")
    assert(containers_list[0].status == "running", "first container status matches")

    assert(containers_list[1].id == "c1002", "second container id matches")
    assert(containers_list[1].name == "db", "second container name matches")
    assert(containers_list[1].image == "postgres:16", "second container image matches")
    assert(containers_list[1].status == "exited", "second container status matches")

    srv.shutdown()

def test_container_exec():
    """Verify client.exec(target, command) runs command and captures exit code and output."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest")

    res = client.exec(c, ["echo", "hello"], env={"MY_VAR": "val"})
    assert(type(res) == "containers.ExecResult", "exec should return ExecResult")
    assert(res.ok == True, "res.ok should be True")
    assert(res.exit_code == 0, "res.exit_code should be 0")
    assert("executed successfully" in res.stdout, "stdout should contain exec output")

    srv.shutdown()

def test_container_logs():
    """Verify client.logs(target) returns io.reader with stream content."""
    srv, client, state = _create_mock_engine()
    c = client.create(image="alpine:latest")

    logs = client.logs(c, tail="50")
    assert(type(logs) == "io.reader", "logs should return io.reader")
    text = logs.text()
    assert("app started" in text, "logs text should contain expected log line")
    assert("listening on 8080" in text, "logs text should contain second log line")

    srv.shutdown()

def test_client_images():
    """Verify client.images() lists local images with normalized dict keys."""
    srv, client, state = _create_mock_engine()
    imgs = client.images()
    assert(len(imgs) == 1, "should return 1 image")
    img = imgs[0]
    assert(img["id"] == "sha256:img123456", "id should match")
    assert(img["Id"] == "sha256:img123456", "Id should match")
    assert(img["repo_tags"] == ["alpine:latest", "alpine:3.19"], "repo_tags should match")
    assert(img["RepoTags"] == ["alpine:latest", "alpine:3.19"], "RepoTags should match")
    assert(img["size"] == 7340032, "size should match")
    assert(img["created"] == 1700000000, "created should match")
    assert(img["labels"]["maintainer"] == "kite", "labels should match")

    srv.shutdown()

def test_client_pull():
    """Verify client.pull() pulls images and encodes auth headers."""
    srv, client, state = _create_mock_engine()

    # 1. Plain pull
    client.pull("alpine:latest")
    assert(len(state["pulled"]) == 1, "1 pull recorded")
    assert(state["pulled"][0]["image"] == "alpine:latest", "pulled image matches")
    assert(state["pulled"][0]["auth"] == "", "auth should be empty")

    # 2. Pull with auth dict
    client.pull("myreg.io/private/app:v1", auth={"username": "user", "password": "pw"})
    assert(len(state["pulled"]) == 2, "2 pulls recorded")
    assert(state["pulled"][1]["image"] == "myreg.io/private/app:v1", "pulled image matches")
    assert(state["pulled"][1]["auth"] != "", "auth header should be non-empty base64")

    srv.shutdown()

def test_client_prune():
    """Verify client.prune() removes containers, volumes, and images."""
    srv, client, state = _create_mock_engine()

    # 1. Default prune (containers only)
    rep1 = client.prune()
    assert(rep1["containers_deleted"] == ["c_dead1", "c_dead2"], "containers_deleted matches")
    assert(rep1["volumes_deleted"] == [], "volumes_deleted should be empty")
    assert(rep1["images_deleted"] == [], "images_deleted should be empty")
    assert(rep1["space_reclaimed"] == 4096, "space_reclaimed matches")

    # 2. Comprehensive prune (containers, volumes, images)
    rep2 = client.prune(containers=True, volumes=True, images=True)
    assert(rep2["containers_deleted"] == ["c_dead1", "c_dead2"], "containers_deleted matches")
    assert(rep2["volumes_deleted"] == ["vol_unused1"], "volumes_deleted matches")
    assert(rep2["images_deleted"] == ["sha256:dangling1"], "images_deleted matches")
    assert(rep2["space_reclaimed"] == 4096 + 8192 + 16384, "combined space_reclaimed matches")

    srv.shutdown()

def test_attrdict_serialization():
    """Verify AttrDict serialization via json.encode() and yaml.encode()."""
    srv, client, state = _create_mock_engine()
    c = client.create(
        image = "postgres:16-alpine",
        name = "serialized-box",
        ports = {"5432/tcp": 5432},
    )
    assert(type(c) == "AttrDict", "c should be AttrDict")

    # JSON serialization
    j_str = json.encode(c)
    decoded = json.decode(j_str)
    assert(decoded["id"] == "cnt-1", "json decode preserves id")
    assert(decoded["name"] == "serialized-box", "json decode preserves name")
    assert(decoded["status"] == "created", "json decode preserves status")

    # YAML serialization
    y_str = yaml.encode(c)
    assert("serialized-box" in y_str, "yaml string contains container name")
    assert("postgres:16-alpine" in y_str, "yaml string contains image name")

    srv.shutdown()

def test_verbs_with_string_identifiers():
    """Verify client verbs accept plain string IDs/names in addition to AttrDict."""
    srv, client, state = _create_mock_engine()

    # 1. start with string ID
    client.start("cnt-1")
    assert(len(state["started"]) == 1, "start recorded")
    assert(state["started"][0] == "cnt-1", "started ID matches")

    # 2. stop with string ID
    client.stop("cnt-1", timeout=5)
    assert(len(state["stopped"]) == 1, "stop recorded")
    assert(state["stopped"][0]["id"] == "cnt-1", "stopped ID matches")

    # 3. restart with string ID
    client.restart("cnt-1", timeout=3)
    assert(len(state["restarted"]) == 1, "restart recorded")
    assert(state["restarted"][0]["id"] == "cnt-1", "restarted ID matches")

    # 4. delete with string ID
    client.delete("cnt-1", force=True)
    assert(len(state["removed"]) == 1, "remove recorded")
    assert(state["removed"][0]["id"] == "cnt-1", "removed ID matches")

    srv.shutdown()

def test_module_shortcuts():
    """Verify selective module root shortcuts are exposed on containers module."""
    assert(hasattr(containers, "run") == True, "containers.run should be exposed")
    assert(hasattr(containers, "exec") == True, "containers.exec should be exposed")
    assert(hasattr(containers, "stop") == True, "containers.stop should be exposed")
    assert(hasattr(containers, "delete") == True, "containers.delete should be exposed")
    assert(hasattr(containers, "remove") == True, "containers.remove should be exposed")
    assert(hasattr(containers, "try_run") == True, "containers.try_run should be exposed")
    assert(hasattr(containers, "try_exec") == True, "containers.try_exec should be exposed")
    assert(hasattr(containers, "try_stop") == True, "containers.try_stop should be exposed")
    assert(hasattr(containers, "try_delete") == True, "containers.try_delete should be exposed")



