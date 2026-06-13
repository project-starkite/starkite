---
title: "Servers"
description: "Build HTTP servers with routes, handlers, and middleware"
weight: 2
---

# HTTP servers

Sometimes a script needs to answer requests rather than make them — a health endpoint, a webhook receiver, a small internal API. The `http` module gives you a production-style server for exactly that, and it keeps the model small: a handler is an ordinary Starlark function that takes a request and returns a response, and a route maps a method-and-path pattern to one of those functions. You wire the routes, then hand control to the server.

## A minimal server

The shortest path from nothing to a running endpoint is three calls — create a server, register a route, and serve. Each handler is a plain function, so the request arrives as its argument and the response leaves as its return value:

```python
def hello(req):
    name = req.query.get("name", "World")
    return {"status": 200, "body": "Hello, " + name}

srv = http.server()
srv.handle("GET /hello", hello)
srv.serve(port=8080)
```

`srv.serve()` blocks, so the call at the bottom is where the script stays for the life of the server — everything above it is setup. Route patterns follow Go's `method path` form: `"GET /hello"`, `"POST /users"`, and `"GET /users/{id}"` for a path variable, which the handler later reads back from `req.params`.

## Request and response

Each handler works with two values, and both are plain Starlark. The request is the object passed in: you read what the client sent through `req.query` for the query string, `req.body` for the payload, `req.params` for path variables, and `req.headers` for headers. The response is the dict you return — a `status`, a `body`, and optional `headers` — and returning `None` instead is shorthand for `204 No Content` when there is nothing to send back:

```python
def create_user(req):
    body = json.decode(req.body)
    return {"status": 201, "body": json.encode({"created": body["name"]})}

srv = http.server()
srv.handle("POST /users", create_user)
srv.serve(port=8080)
```

Here the handler decodes the incoming JSON body, then encodes its own JSON for the response, so the dict it returns carries a `201 Created` and a serialized payload. The body is a string on both sides; `json.decode` and `json.encode` are what bridge it to and from structured data.

## Quick start with a route map

When a server is small enough that the handlers are one-liners, registering each route by hand is more ceremony than it is worth. `http.serve()` collapses the whole setup into a single call: hand it a dict of pattern-to-handler and a port, and it builds the server, registers every route, and blocks — all at once:

```python
http.serve({
    "GET /healthz": lambda req: {"status": 200, "body": "ok"},
    "GET /version": lambda req: {"status": 200, "body": "v1"},
}, port=8080)
```

This is the same server you would build with `http.server()` and repeated `srv.handle()` calls, written as data. Reach for it when the routes are trivial and lose nothing by being inline; switch back to the explicit form once a handler grows past a lambda or you need middleware.

## Middleware

Cross-cutting concerns — logging every request, checking auth, stamping a shared header — do not belong inside each handler, because repeating them invites drift. `srv.use(fn)` lets you wrap every handler from the outside instead. A middleware is a function that takes the next handler and returns a replacement: it does its work, then calls through:

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

The inner `handler` logs the method and path, then forwards the request to `next(req)` and returns whatever it produces, so the wrapped behavior runs on the way in and the real handler runs unchanged. Because the wrap happens around every registered route, one `srv.use()` covers the whole server rather than each endpoint.

## Timeouts and TLS

A server exposed beyond your own machine should not wait forever on a slow client or accept an unbounded body, so `http.server()` takes the knobs that bound those risks. `read_timeout`, `write_timeout`, `idle_timeout`, and `shutdown_timeout` are duration strings (`"30s"`); `max_body_bytes` caps the request size; and `tls_cert` / `tls_key` switch the listener to HTTPS. The defaults are permissive, so set the timeouts deliberately for anything past a local experiment. See the [http API reference](../../references/api/http.md#httpserver) for the full surface.

## Permissions

Serving HTTP is a privileged act — it opens a listening socket — so it sits behind the permission ladder rather than running by default. `http.*` serving lives in the `allow-local` profile, alongside `os.exec` under `$CWD` and `k8s` access. Under the default `deny-all`, a script that calls `srv.serve()` is stopped at that call; grant the profile to let it through:

```bash
kite ./server.star --permissions=allow-local
```

Run a server under the profile it will use in production, no looser. If a server script needs more than `allow-local`, that is a signal to look at what else it is reaching for. See [Permission](../../fundamentals/security/permission.md).
