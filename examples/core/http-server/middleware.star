#!/usr/bin/env kite
# middleware.star - Middleware chain
#
# Middleware functions receive (req, next) and can:
#   - Inspect or modify the request before calling next(req)
#   - Inspect or modify the response after calling next(req)
#   - Short-circuit by returning a response without calling next
#
# Middleware runs in registration order (first registered = outermost).
#
# Run:   kite run examples/core/http-server/middleware.star
# Test:  curl -v http://localhost:8080/hello
#        curl -v http://localhost:8080/admin (returns 401)

def logging_mw(req, next):
    """Log each request method and path."""
    printf("[%s] %s %s\n", time.format(time.now(), time.Kitchen), req.method, req.path)
    return next(req)

def cors_mw(req, next):
    """Add CORS headers to every response."""
    resp = next(req)
    if type(resp) == "dict":
        headers = resp.get("headers", {})
        headers["Access-Control-Allow-Origin"] = "*"
        headers["Access-Control-Allow-Methods"] = "GET, POST, PUT, DELETE"
        resp["headers"] = headers
    return resp

def auth_mw(req, next):
    """Block requests to /admin without an Authorization header."""
    if "/admin" in req.path:
        auth = req.headers.get("Authorization", "")
        if not auth:
            return {"status": 401, "body": "unauthorized"}
    return next(req)

def hello(req):
    return {"status": 200, "headers": {}, "body": "hello"}

def admin(req):
    return {"status": 200, "headers": {}, "body": "admin area"}

srv = http.server()
srv.use(logging_mw)
srv.use(cors_mw)
srv.use(auth_mw)
srv.handle("/hello", hello)
srv.handle("/admin", admin)
srv.serve(port=8080)
