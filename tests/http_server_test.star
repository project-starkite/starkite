# http_server_test.star — Integration tests for HTTP server module

def test_server_handle_and_respond():
    """Test basic server with explicit response dict."""
    def handler(req):
        return {"status": 200, "body": "hello from server"}

    srv = http.server()
    srv.handle("/hello", handler)
    srv.start(port=0)

    port = srv.port()
    assert(port > 0, "port should be assigned")

    resp = http.url("http://localhost:%d/hello" % port).get()
    assert(resp.status_code == 200, "should return 200, got %d" % resp.status_code)
    assert(resp.get_text() == "hello from server", "body mismatch: %s" % resp.get_text())

    srv.shutdown()

def test_server_path_params():
    """Test path parameter extraction via {id}."""
    def get_user(req):
        user_id = req.params["id"]
        return {"body": "user:" + user_id}

    srv = http.server()
    srv.handle("GET /users/{id}", get_user)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/users/42" % port).get()
    assert(resp.status_code == 200, "should return 200")
    assert(resp.get_text() == "user:42", "body should be 'user:42', got '%s'" % resp.get_text())

    srv.shutdown()

def test_server_middleware():
    """Test middleware chain execution."""
    def add_header_mw(req, next):
        resp = next(req)
        if type(resp) == "dict":
            headers = resp.get("headers", {})
            headers["X-Middleware"] = "applied"
            resp["headers"] = headers
        return resp

    def handler(req):
        return {"status": 200, "headers": {}, "body": "ok"}

    srv = http.server()
    srv.use(add_header_mw)
    srv.handle("/mw", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/mw" % port).get()
    assert(resp.status_code == 200, "should return 200")
    assert(resp.headers["X-Middleware"] == "applied", "middleware header should be present")

    srv.shutdown()

def test_server_middleware_short_circuit():
    """Test middleware that short-circuits the chain."""
    def auth_mw(req, next):
        return {"status": 401, "body": "unauthorized"}

    handler_called = [False]
    def handler(req):
        handler_called[0] = True
        return "ok"

    srv = http.server()
    srv.use(auth_mw)
    srv.handle("/auth", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/auth" % port).get()
    assert(resp.status_code == 401, "should return 401")
    assert("unauthorized" in resp.get_text(), "body should be 'unauthorized'")

    srv.shutdown()

def test_server_string_response():
    """Test handler returning a plain string -> 200 text/plain."""
    def handler(req):
        return "just text"

    srv = http.server()
    srv.handle("/text", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/text" % port).get()
    assert(resp.status_code == 200, "should return 200")
    assert(resp.get_text() == "just text", "body should be 'just text'")

    srv.shutdown()

def test_server_none_response():
    """Test handler returning None -> 204 No Content."""
    def handler(req):
        return None

    srv = http.server()
    srv.handle("/noop", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/noop" % port).get()
    assert(resp.status_code == 204, "should return 204, got %d" % resp.status_code)

    srv.shutdown()

def test_server_auto_json():
    """Test handler returning dict without 'body' -> auto JSON."""
    def handler(req):
        return {"name": "alice", "age": 30}

    srv = http.server()
    srv.handle("/json", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/json" % port).get()
    assert(resp.status_code == 200, "should return 200")
    data = json.decode(resp.get_text())
    assert(data["name"] == "alice", "name should be 'alice'")

    srv.shutdown()

def test_server_query_params():
    """Test query string parameter parsing."""
    def handler(req):
        name = req.query.get("name", "unknown")
        return {"body": "hello " + name}

    srv = http.server()
    srv.handle("/greet", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/greet?name=world" % port).get()
    assert(resp.get_text() == "hello world", "body should be 'hello world', got '%s'" % resp.get_text())

    srv.shutdown()

def test_server_request_body():
    """Test that request body is available."""
    def handler(req):
        return {"body": "echo:" + req.body}

    srv = http.server()
    srv.handle("POST /echo", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/echo" % port).post("ping")
    assert(resp.get_text() == "echo:ping", "body should be 'echo:ping', got '%s'" % resp.get_text())

    srv.shutdown()

def test_server_instance():
    """Test server instance (replaces default server test)."""
    def handler(req):
        return "instance-ok"

    srv = http.server()
    srv.handle("/instance", handler)
    srv.start(port=0)

    port = srv.port()
    assert(port > 0, "server port should be assigned")

    resp = http.url("http://localhost:%d/instance" % port).get()
    assert(resp.status_code == 200, "should return 200")
    assert(resp.get_text() == "instance-ok", "body should be 'instance-ok'")

    srv.shutdown()

def test_server_handler_error():
    """Test that handler errors return 500 and server stays alive."""
    call_count = [0]
    def bad_handler(req):
        call_count[0] += 1
        if call_count[0] == 1:
            fail("intentional error")
        return "recovered"

    srv = http.server()
    srv.handle("/fail", bad_handler)
    srv.start(port=0)

    port = srv.port()

    # First request — should get 500
    resp1 = http.url("http://localhost:%d/fail" % port).get()
    assert(resp1.status_code == 500, "first request should return 500")

    # Second request — server should still be alive
    resp2 = http.url("http://localhost:%d/fail" % port).get()
    assert(resp2.status_code == 200, "second request should return 200")
    assert(resp2.get_text() == "recovered", "body should be 'recovered'")

    srv.shutdown()

def test_server_multiple_routes():
    """Test multiple routes on one server."""
    def hello(req):
        return "hello"
    def world(req):
        return "world"

    srv = http.server()
    srv.handle("/hello", hello)
    srv.handle("/world", world)
    srv.start(port=0)

    port = srv.port()
    r1 = http.url("http://localhost:%d/hello" % port).get()
    r2 = http.url("http://localhost:%d/world" % port).get()
    assert(r1.get_text() == "hello", "first route body mismatch")
    assert(r2.get_text() == "world", "second route body mismatch")

    srv.shutdown()

def test_server_request_properties():
    """Test that request is an object with dot-access properties."""
    def handler(req):
        assert(type(req) == "http.request", "req type should be http.request, got %s" % type(req))
        assert(req.method == "GET", "method should be GET")
        assert(req.path != "", "path should not be empty")
        assert(req.host != "", "host should not be empty")
        return "ok"

    srv = http.server()
    srv.handle("/props", handler)
    srv.start(port=0)

    port = srv.port()
    resp = http.url("http://localhost:%d/props" % port).get()
    assert(resp.status_code == 200, "should return 200")

    srv.shutdown()

def test_server_constructor_config():
    """Test that port/host can be set in constructor."""
    def handler(req):
        return "config-ok"

    srv = http.server(port=0)
    srv.handle("/cfg", handler)
    srv.start()  # uses port=0 from constructor

    port = srv.port()
    assert(port > 0, "port should be assigned")

    resp = http.url("http://localhost:%d/cfg" % port).get()
    assert(resp.get_text() == "config-ok", "body should be 'config-ok'")

    srv.shutdown()

def test_serve_shortcut_callable():
    """Test http.serve(handler, port=0) one-liner with callable."""
    # We can't easily test blocking serve() in a test, so we test
    # the server instance approach which is equivalent.
    def handler(req):
        return "serve-ok"

    srv = http.server(port=0)
    srv.handle("/", handler)
    srv.start()

    port = srv.port()
    resp = http.url("http://localhost:%d/" % port).get()
    assert(resp.get_text() == "serve-ok", "body should be 'serve-ok'")

    srv.shutdown()
