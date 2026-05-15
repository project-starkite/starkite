---
title: "Serve and call HTTP"
description: "Spin up a minimal HTTP server, then call it from a starkite script"
weight: 2
---

# Serve and call HTTP

Two sides of HTTP in starkite: `http.serve()` runs a server with method-aware routing; `http.url(...).get()` makes a client call.

**Source:** [`examples/core/http-server/hello.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/http-server/hello.star)

## Server

```python
#!/usr/bin/env kite

def handler(req):
    return "Hello from starkite!"

http.serve({"GET /hello": handler}, port=8080)
```

Run it:

```bash
kite run examples/core/http-server/hello.star
```

In another shell:

```bash
$ curl http://localhost:8080/hello
Hello from starkite!
```

## Client

A one-line GET, no script file required:

```bash
kite exec 'print(http.url("http://localhost:8080/hello").get().get_text())'
```

In a script, the same call:

```python
resp = http.url("http://api.example.com/health").get(timeout="5s")
print(resp.status_code, resp.get_text())
```

## What's happening

- `http.serve(routes, port=N)` is a blocking call. The dict keys are `"<METHOD> <PATH>"`; values are handler callables that take `req` and return either a string body or a `{"status": …, "body": …}` dict.
- `http.url(u)` returns a builder; `.get()`, `.post()`, `.put()`, `.patch()`, `.delete()` perform the request and return a response object with `.status_code`, `.get_text()`, `.body` attributes.
- For complex servers (middleware, path params, request bodies), see the [http-server examples](https://github.com/project-starkite/starkite/tree/main/examples/core/http-server) directory.

## See also

- [`http` reference](../../references/api/http.md) — full client + server surface, including `http.server()` for stateful servers, middleware, JSON helpers
