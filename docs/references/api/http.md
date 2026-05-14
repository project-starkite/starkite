---
title: "http"
description: "HTTP client, server, and URL builder"
weight: 3
---

The `http` module provides HTTP client functionality, a URL builder, and an embedded HTTP server for building web services in starkite scripts.

## Module Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `http.url(url_string)` | `http.url` | Create a URL object for making requests |
| `http.config(timeout="30s", headers={})` | `None` | Set default client configuration |
| `http.server(port=0, host="", tls_cert="", tls_key="", read_timeout="", write_timeout="", idle_timeout="", shutdown_timeout="", max_header_bytes=0, max_body_bytes=0)` | `http.server` | Create an HTTP server. Timeout kwargs take duration strings (e.g. `"30s"`); `max_*_bytes` take integer byte sizes |
| `http.serve(handler_or_routes, port=0, host="", tls_cert="", tls_key="")` | `None` | Quick-start a server with a handler or route dict |

## http.url

The `http.url` object provides methods for making HTTP requests.

```python
url = http.url("https://api.example.com/v1/users")
```

### Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `url.get(headers={}, timeout="")` | `http.response` | Send GET request |
| `url.post(body, headers={}, timeout="")` | `http.response` | Send POST request |
| `url.put(body, headers={}, timeout="")` | `http.response` | Send PUT request |
| `url.patch(body, headers={}, timeout="")` | `http.response` | Send PATCH request |
| `url.delete(headers={}, timeout="")` | `http.response` | Send DELETE request |

The `body` parameter accepts strings, bytes, or dicts (automatically JSON-encoded). The `timeout` parameter overrides the default from `http.config()`.

## http.response

The response object returned by all request methods.

### Properties

| Property | Type | Description |
|----------|------|-------------|
| `status_code` | `int` | HTTP status code (e.g. `200`) |
| `status` | `string` | Status text (e.g. `"200 OK"`) |
| `body` | `string` | Response body as text |
| `headers` | `dict` | Response headers |

### Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `resp.get_text()` | `string` | Get response body as text |
| `resp.get_bytes()` | `bytes` | Get response body as bytes |

## http.server

The server object for building HTTP services.

### Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `srv.handle(pattern, handler)` | `None` | Register a route handler |
| `srv.use(middleware)` | `None` | Add middleware function |
| `srv.start(port=0, host="", tls_cert="", tls_key="")` | `None` | Start server in background |
| `srv.serve(port=0, host="", tls_cert="", tls_key="")` | `None` | Start server and block |
| `srv.shutdown()` | `None` | Gracefully shut down the server |
| `srv.port()` | `int` | Get the port the server is listening on |

### Handler Functions

Handler functions receive an `http.request` and return a response:

```python
def handler(req):
    return {
        "status": 200,
        "body": "Hello, World!",
        "headers": {"Content-Type": "text/plain"},
    }
```

## http.request

The request object passed to handler functions.

### Properties

| Property | Type | Description |
|----------|------|-------------|
| `method` | `string` | HTTP method (`"GET"`, `"POST"`, etc.) |
| `path` | `string` | Request path |
| `url` | `string` | Full request URL |
| `params` | `dict` | URL path parameters |
| `query` | `dict` | Query string parameters |
| `headers` | `dict` | Request headers |
| `body` | `string` | Request body |
| `remote_addr` | `string` | Client remote address |
| `host` | `string` | Request host |

## Examples

### HTTP Client

```python
# Simple GET
resp = http.url("https://httpbin.org/get").get()
print(resp.status_code)  # 200
print(resp.body)

# POST with JSON body
resp = http.url("https://httpbin.org/post").post(
    {"name": "kite", "version": "1.0"},
    headers={"Authorization": "Bearer token123"},
)
data = json.decode(resp.body)

# Set default timeout and headers
http.config(
    timeout="10s",
    headers={"User-Agent": "kite/1.0"},
)

# Error handling with try_
result = http.url("https://unreachable.example.com").try_get()
if not result.ok:
    print("Request failed:", result.error)
```

### HTTP Server

```python
def hello(req):
    name = req.query.get("name", "World")
    return {"status": 200, "body": "Hello, " + name + "!"}

def create_user(req):
    user = json.decode(req.body)
    # ... process user
    return {"status": 201, "body": json.encode({"id": 1, "name": user["name"]})}

# Register routes and serve
srv = http.server()
srv.handle("GET /hello", hello)
srv.handle("POST /users", create_user)
srv.serve(port=8080)
```

### Quick Server

```python
# Serve with a route dictionary
http.serve({
    "GET /":       lambda req: {"status": 200, "body": "Home"},
    "GET /health": lambda req: {"status": 200, "body": "ok"},
}, port=8080)
```

> **Note:**
All `http.url` request methods support `try_` variants. For example, `url.try_get()` and `url.try_post(body)` return a `Result` instead of raising on network errors.

