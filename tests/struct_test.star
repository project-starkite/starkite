# struct_test.star - End-to-end tests for the built-in struct() constructor and adapters.

def test_struct_creation_unpack_and_immutability():
    empty = struct()
    assert_equal(type(empty), "struct")
    assert_true(bool(empty))

    cfg = struct(host = "db.internal", port = 5432, tls = True)
    assert_equal(cfg.host, "db.internal")
    assert_equal(cfg.port, 5432)
    assert_true(cfg.tls)

    from_dict = struct(**{"region": "us-central1", "replicas": 3})
    assert_equal(from_dict.region, "us-central1")
    assert_equal(from_dict.replicas, 3)

    assert_equal(str(struct(b = 2, a = 1)), "struct(a = 1, b = 2)")

def test_struct_union_merge():
    base = struct(region = "us-central1", zone = "us-central1-a", replicas = 1)
    override = struct(zone = "us-west1-b", replicas = 3)
    merged = base + override

    assert_equal(merged.region, "us-central1")
    assert_equal(merged.zone, "us-west1-b")
    assert_equal(merged.replicas, 3)
    assert_equal(base.zone, "us-central1-a")

def test_struct_equality_hashing_and_introspection():
    a = struct(x = 1, y = "ok")
    b = struct(y = "ok", x = 1)
    assert_equal(a, b)
    assert_not_equal(a, struct(x = 2, y = "ok"))

    seen = {a: "entry"}
    assert_equal(seen[b], "entry")

    assert_equal(dir(a), ["x", "y"])
    assert_true(hasattr(a, "x"))
    assert_false(hasattr(a, "missing"))
    assert_equal(getattr(a, "y"), "ok")
    assert_equal(getattr(a, "missing", "default"), "default")

def test_struct_closures_and_client_pattern():
    def make_client(project, region):
        return struct(
            project = project,
            region = region,
            path = lambda res: "projects/%s/locations/%s/%s" % (project, region, res),
        )

    client = make_client("edge-proj", "us-central1")
    assert_equal(client.project, "edge-proj")
    assert_equal(client.path("clusters/hub"), "projects/edge-proj/locations/us-central1/clusters/hub")

def test_struct_json_serialization():
    payload = struct(
        cluster = "hub",
        nodes = [struct(name = "edge-1", port = 18080)],
        meta = {"region": struct(code = "us-west1")},
    )
    decoded = json.decode(json.encode(payload))
    assert_equal(decoded["cluster"], "hub")
    assert_equal(decoded["nodes"][0]["name"], "edge-1")
    assert_equal(decoded["meta"]["region"]["code"], "us-west1")
    assert_equal(json.source(payload).encode(), json.encode(payload))

    fn_res = json.try_encode(struct(fn = lambda x: x))
    assert_false(fn_res.ok)

def test_struct_yaml_encode_and_merge():
    base = struct(
        app = "edge",
        containers = [struct(name = "web", image = "nginx:1.27", ports = [80])],
    )
    patch = struct(
        containers = [struct(name = "web", image = "nginx:1.28")],
    )
    merged = yaml.merge(base, patch, list_mode = "by_key", list_key = "name")
    assert_equal(merged["containers"][0]["image"], "nginx:1.28")
    assert_equal(merged["containers"][0]["ports"], [80])

    encoded = yaml.encode(struct(name = "hub", tls = True))
    assert_equal(yaml.decode(encoded), {"name": "hub", "tls": True})

    docs = yaml.decode_all(yaml.encode_all([struct(kind = "First"), struct(kind = "Second")]))
    assert_equal([d["kind"] for d in docs], ["First", "Second"])

def test_struct_template_and_k8s_yaml():
    rendered = template.render("{{.env}}:{{.host}}:{{.port}}", struct(env = "prod", host = "api", port = 8443))
    assert_equal(rendered, "prod:api:8443")

    from_tmpl = template.text("{{.host}}").render(struct(host = "hub"))
    assert_equal(from_tmpl, "hub")

    manifest = struct(
        apiVersion = "v1",
        kind = "ConfigMap",
        metadata = struct(name = "the-map"),
        data = struct(enableRisky = "true"),
    )
    kyaml = k8s.yaml(manifest)
    assert_contains(kyaml, 'enableRisky: "true"')

def test_struct_builtin_interop():
    # Built-in `user` global is a branded *starlarkstruct.Struct ("user" vs "struct")
    assert_equal(type(user), "struct")
    assert_true(user.name() != "")
    assert_true(user != struct(name = user.name))

def test_struct_module_inputs():
    # log accepts struct in attrs
    log.info("struct-test", attrs = struct(vm = "edge-1", port = 443))
    sub = log.logger().attrs(struct(region = "us-west1"))
    sub.info("sublogger")

    # sql.insert accepts struct row and list of structs
    db = sql.open("sqlite", ":memory:")
    db.exec("CREATE TABLE edges (name TEXT, zone TEXT)")
    db.insert("edges", struct(name = "edge-1", zone = "us-west1-a"))
    db.insert("edges", [struct(name = "edge-2", zone = "us-west1-b")])
    assert_equal(db.query_value("SELECT count(*) FROM edges"), 2)
    db.close()

    # csv.source accepts list of structs
    csv_path = fs.path(temp_dir()) / "struct_rows.csv"
    csv.source([struct(name = "edge-1", zone = "us-west1-a")]).write_file(csv_path.string)
    rows = csv.file(csv_path.string).read(header = True)
    assert_equal(len(rows), 1)
    assert_equal(rows[0]["name"], "edge-1")
    csv_path.remove()

    # fleet.new accepts list of structs
    fl = fleet.new([struct(name = "edge-1", address = "10.0.0.10", region = "us-west1")])
    assert_equal(fl.count, 1)
    assert_equal(fl.addresses(), ["10.0.0.10"])

    # ssh.config accepts struct for auth, jump, and env
    c = ssh.config(
        hosts = ["10.0.0.10"],
        auth = struct(user = "deploy", use_agent = True),
        jump = struct(host = "bastion.internal", port = 2222),
        env = struct(APP_ENV = "prod"),
        dry_run = True,
    )
    res = c.exec("uptime", env = struct(PORT = "8080"))
    assert_equal(len(res), 1)
    assert_contains(res[0].stdout, "PORT=")

    # k8s.obj and k8s.validate accept structs
    pod = k8s.obj.pod(
        name = "edge-pod",
        labels = struct(app = "edge"),
        containers = [k8s.obj.container(name = "agent", image = "alpine:3.20")],
    )
    val = k8s.validate(pod, expression = "object.metadata.labels.app == 'edge'")
    assert_equal(type(val), "struct")
    assert_true(val.valid)
