---
title: "Servers"
description: "Build HTTP servers with routes, handlers, and middleware"
weight: 2
---

# HTTP servers

The `http` module runs production-style HTTP servers. A handler is a Starlark function that receives a request and returns a response; routes map method + path patterns to handlers.

## A minimal server

```python
def hello(req):
    name = req.query.get("name", "World")
    return {"status": 200, "body": "Hello, " + name}

srv = http.server()
srv.handle("GET /hello", hello)
srv.serve(port=8080)
```

`srv.serve()` blocks. Route patterns use Go's `method path` form (`"GET /hello"`, `"POST /users"`, `"GET /users/{id}"`).

## Request and response

A handler receives a `req` with fields like `req.query`, `req.body`, `req.params` (path variables), and `req.headers`. It returns a dict with `status`, `body`, and optional `headers`; returning `None` sends `204 No Content`.

```python
def create_user(req):
    body = json.decode(req.body)
    return {"status": 201, "body": json.encode({"created": body["name"]})}

srv = http.server()
srv.handle("POST /users", create_user)
srv.serve(port=8080)
```

## Quick start with a route map

`http.serve()` starts a server from a route dict in one call:

```python
http.serve({
    "GET /healthz": lambda req: {"status": 200, "body": "ok"},
    "GET /version": lambda req: {"status": 200, "body": "v1"},
}, port=8080)
```

## Middleware

`srv.use(fn)` wraps every handler — useful for logging, auth, or shared headers:

```python
def log_requests(next):
    def handler(req):
        log.info("request", {"method": req.method, "path": req.path})
        return next(req)
    return handler

srv = http.server(read_timeout="30s", write_timeout="60s")
srv.use(log_requests)
srv.handle("GET /hello", hello)
srv.serve(port=8080)
```

## Timeouts and TLS

`http.server()` accepts `read_timeout`, `write_timeout`, `idle_timeout`, `shutdown_timeout` (duration strings), `max_body_bytes`, and `tls_cert` / `tls_key` for HTTPS. See the [http API reference](../../references/api/http.md#httpserver) for the full surface.
