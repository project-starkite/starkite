# HTTP Server Examples

Examples demonstrating the `http.server` module in starkite.

## Examples

### hello.star
Minimal HTTP server. Uses `http.serve()` one-liner shortcut with a routes dict — no explicit `http.server()` call needed.

```
kite run examples/core/http-server/hello.star
curl http://localhost:8080/hello
```

### json-api.star
JSON API with multiple routes. Shows the two dict response modes: returning a dict without a `body` key auto-serializes the entire dict as JSON (`application/json`), while a dict with `body`/`status`/`headers` keys gives explicit control over the response.

```
kite run examples/core/http-server/json-api.star
curl http://localhost:8080/api/users
curl http://localhost:8080/api/health
```

### path-params.star
Route path parameters using Go 1.22+ ServeMux patterns. Demonstrates single parameters (`{id}`), multiple parameters (`{user_id}/posts/{post_id}`), and wildcard catch-all parameters (`{path...}`). Parameters are available via `req.params`.

```
kite run examples/core/http-server/path-params.star
curl http://localhost:8080/users/42
curl http://localhost:8080/users/42/posts/7
curl http://localhost:8080/files/css/style.css
```

### query-params.star
Query string parameter handling. Single values arrive as strings, repeated keys (`?tag=a&tag=b`) arrive as lists. Parameters are available via `req.query`.

```
kite run examples/core/http-server/query-params.star
curl 'http://localhost:8080/search?q=starkite&limit=10'
curl 'http://localhost:8080/filter?tag=go&tag=starlark'
```

### request-body.star
Handling POST/PUT request bodies. The raw body is available as `req.body` (a string). Parse JSON payloads with `json.decode()`. Shows creating resources and echoing data back.

```
kite run examples/core/http-server/request-body.star
curl -X POST http://localhost:8080/echo -d 'hello world'
curl -X POST http://localhost:8080/api/items \
     -H 'Content-Type: application/json' \
     -d '{"name": "widget", "price": 9.99}'
curl http://localhost:8080/api/items
```

### middleware.star
Middleware chain with logging, CORS, and authentication. Middleware functions receive `(req, next)` and can inspect/modify requests, call `next(req)` to continue the chain, modify responses, or short-circuit by returning without calling `next`. Middleware runs in registration order.

```
kite run examples/core/http-server/middleware.star
curl -v http://localhost:8080/hello
curl http://localhost:8080/admin
curl -H 'Authorization: Bearer token' http://localhost:8080/admin
```

### response-types.star
All four supported response types side by side:
- **string** — `200 text/plain`
- **None** — `204 No Content`
- **dict with `body`** — explicit control over status, headers, and body
- **dict without `body`** — auto-serialized as `application/json`

```
kite run examples/core/http-server/response-types.star
curl http://localhost:8080/text
curl -i http://localhost:8080/no-content
curl http://localhost:8080/explicit
curl http://localhost:8080/json
```

### rest-api.star
Complete REST API with CRUD operations. Demonstrates method-aware routing (`GET /path`, `POST /path`, `PUT /path`, `DELETE /path`), path parameters, JSON request/response handling, and proper HTTP status codes (201 Created, 204 No Content, 404 Not Found).

```
kite run examples/core/http-server/rest-api.star
curl http://localhost:8080/api/tasks
curl -X POST http://localhost:8080/api/tasks \
     -H 'Content-Type: application/json' \
     -d '{"title": "Learn starkite", "done": false}'
curl http://localhost:8080/api/tasks/1
curl -X PUT http://localhost:8080/api/tasks/1 \
     -H 'Content-Type: application/json' \
     -d '{"title": "Learn starkite", "done": true}'
curl -X DELETE http://localhost:8080/api/tasks/1
```

### multi-server.star
Multiple independent server instances on different ports. Uses `start()` (non-blocking) to launch the first server and `serve()` (blocking) for the second. Each server has its own routes and middleware.

```
kite run examples/core/http-server/multi-server.star
curl http://localhost:8080/
curl http://localhost:9090/
curl http://localhost:9090/metrics
```

### webhook.star
Webhook receiver for event-driven automation. Receives JSON payloads via POST, logs requests with a middleware, and reacts to specific events. Useful as a starting point for CI/CD hooks or GitHub webhook handlers.

```
kite run examples/core/http-server/webhook.star
curl http://localhost:8080/health
curl -X POST http://localhost:8080/webhook \
     -H 'Content-Type: application/json' \
     -d '{"event": "push", "repo": "starkite", "branch": "main"}'
```

## Request Object

Every handler receives an `http.request` object with these properties:

| Key | Type | Description |
|-----|------|-------------|
| `method` | string | HTTP method (GET, POST, ...) |
| `path` | string | URL path |
| `url` | string | Full request URL |
| `params` | dict | Path parameters from route pattern |
| `query` | dict | Query string parameters |
| `headers` | dict | Request headers |
| `body` | string | Raw request body |
| `remote_addr` | string | Client address |
| `host` | string | Host header value |

## Server API

```python
# One-liner shortcut
http.serve(handler, port=8080)           # single handler, blocks
http.serve({...routes...}, port=8080)    # routes dict, blocks

# Server instance
srv = http.server(port=8080)     # create with config
srv.handle(pattern, handler)     # register route
srv.use(middleware)              # register middleware
srv.start()                      # start without blocking (uses constructor config)
srv.start(port=0)                # override port
srv.serve()                      # start and block until shutdown
srv.port()                       # get assigned port (useful with port=0)
srv.shutdown()                   # graceful shutdown
```
