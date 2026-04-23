# http_test.star - Tests for http client module
# Uses http.server as local test backend — no network access required.

def test_get_simple():
    """Test http.url().get() with simple request."""
    def handler(req):
        return {"status": 200, "body": "get-ok"}

    srv = http.server()
    srv.handle("/get", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/get" % port).get()
    assert(resp.status_code == 200, "should return 200, got %d" % resp.status_code)
    assert(resp.get_text() != "", "should have body")
    assert(resp.get_text() == "get-ok", "body should be 'get-ok', got '%s'" % resp.get_text())

    srv.shutdown()

def test_get_with_headers():
    """Test http.url().get() with custom headers."""
    def handler(req):
        custom = req.headers.get("X-Custom", "missing")
        return {"body": custom}

    srv = http.server()
    srv.handle("/headers", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/headers" % port).get(
        headers={"X-Custom": "test-value"}
    )
    assert(resp.status_code == 200, "should return 200")
    assert(resp.get_text() == "test-value", "should echo custom header, got '%s'" % resp.get_text())

    srv.shutdown()

def test_post_json():
    """Test http.url().post() with JSON body."""
    def handler(req):
        data = json.decode(req.body)
        return {"key": data["key"]}

    srv = http.server()
    srv.handle("POST /post", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/post" % port).post({"key": "value"})
    assert(resp.status_code == 200, "should return 200")
    data = json.decode(resp.get_text())
    assert(data["key"] == "value", "should echo key, got '%s'" % data["key"])

    srv.shutdown()

def test_post_form():
    """Test http.url().post() with form data."""
    def handler(req):
        return {"body": req.body}

    srv = http.server()
    srv.handle("POST /form", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/form" % port).post(
        "name=test&value=123",
        headers={"Content-Type": "application/x-www-form-urlencoded"}
    )
    assert(resp.status_code == 200, "should return 200")
    assert(resp.get_text() == "name=test&value=123", "should echo form body")

    srv.shutdown()

def test_status_codes():
    """Test http handling of various status codes."""
    def handler(req):
        code = int(req.query["code"])
        return {"status": code, "body": "status:%d" % code}

    srv = http.server()
    srv.handle("/status", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/status?code=201" % port).get()
    assert(resp.status_code == 201, "should return 201, got %d" % resp.status_code)

    srv.shutdown()

def test_not_found():
    """Test http.url().get() with 404 response."""
    def handler(req):
        return {"status": 404, "body": "not found"}

    srv = http.server()
    srv.handle("/notfound", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/notfound" % port).get()
    assert(resp.status_code == 404, "should return 404")

    srv.shutdown()

def test_response_headers():
    """Test http response headers."""
    def handler(req):
        return {"status": 200, "headers": {"X-Test": "hello"}, "body": "ok"}

    srv = http.server()
    srv.handle("/resp-headers", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/resp-headers" % port).get()
    assert(resp.status_code == 200, "should return 200")
    assert(resp.headers["X-Test"] == "hello", "should have X-Test header")

    srv.shutdown()

def test_timeout():
    """Test http.url().get() with timeout (request completes within timeout)."""
    def handler(req):
        return "fast"

    srv = http.server()
    srv.handle("/fast", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/fast" % port).get(timeout="5s")
    assert(resp.status_code == 200, "should complete within timeout")

    srv.shutdown()

def test_json_response():
    """Test parsing JSON response body."""
    def handler(req):
        return {"slideshow": {"title": "Sample", "slides": []}}

    srv = http.server()
    srv.handle("/json", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/json" % port).get()
    assert(resp.status_code == 200, "should return 200")
    data = json.decode(resp.get_text())
    assert(data != None, "should parse JSON body")
    assert(data["slideshow"]["title"] == "Sample", "should have slideshow title")

    srv.shutdown()

def test_get_bytes():
    """Test resp.get_bytes() returns bytes type."""
    def handler(req):
        return "binary-data"

    srv = http.server()
    srv.handle("/bytes", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/bytes" % port).get()
    b = resp.get_bytes()
    assert(type(b) == "bytes", "get_bytes() should return bytes, got %s" % type(b))

    srv.shutdown()

def test_get_text():
    """Test resp.get_text() returns string type."""
    def handler(req):
        return "text-data"

    srv = http.server()
    srv.handle("/text", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/text" % port).get()
    t = resp.get_text()
    assert(type(t) == "string", "get_text() should return string, got %s" % type(t))
    assert(t == "text-data", "get_text() should return 'text-data', got '%s'" % t)

    srv.shutdown()

def test_response_type():
    """Test that type(resp) is http.response."""
    def handler(req):
        return "ok"

    srv = http.server()
    srv.handle("/type", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/type" % port).get()
    assert(type(resp) == "http.response", "type should be http.response, got %s" % type(resp))

    srv.shutdown()

def test_url_type():
    """Test that type(http.url(url)) is http.url."""
    u = http.url("http://example.com")
    assert(type(u) == "http.url", "type should be http.url, got %s" % type(u))

def test_body_property():
    """Test that resp.body returns bytes."""
    def handler(req):
        return "body-test"

    srv = http.server()
    srv.handle("/body", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/body" % port).get()
    assert(type(resp.body) == "bytes", "body should be bytes, got %s" % type(resp.body))

    srv.shutdown()

def test_status_property():
    """Test that resp.status returns status string."""
    def handler(req):
        return "ok"

    srv = http.server()
    srv.handle("/st", handler)
    srv.start(port=0)
    port = srv.port()

    resp = http.url("http://localhost:%d/st" % port).get()
    assert("200" in resp.status, "status should contain '200', got '%s'" % resp.status)

    srv.shutdown()

def test_try_get():
    """Test http.url(url).try_get() returns Result."""
    def handler(req):
        return "try-ok"

    srv = http.server()
    srv.handle("/try", handler)
    srv.start(port=0)
    port = srv.port()

    result = http.url("http://localhost:%d/try" % port).try_get()
    assert(result.ok == True, "try_get should succeed")
    assert(result.value.status_code == 200, "should return 200")
    assert(result.value.get_text() == "try-ok", "body should be 'try-ok'")

    srv.shutdown()
