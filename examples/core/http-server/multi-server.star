#!/usr/bin/env kite
# multi-server.star - Multiple independent server instances
#
# Each http.server() call creates a separate server with its own
# routes, middleware, and port. Use start() (non-blocking) to run
# multiple servers concurrently.
#
# Run:   kite run examples/core/http-server/multi-server.star
# Test:  curl http://localhost:8080/  (public API)
#        curl http://localhost:9090/  (internal/admin)

def public_handler(req):
    return {"service": "public-api", "status": "ok"}

def admin_handler(req):
    return {"service": "admin", "status": "ok"}

def metrics_handler(req):
    return {"uptime": "running", "requests": 0}

# Public API server
api = http.server()
api.handle("GET /", public_handler)
api.start(port=8080)
printf("Public API running on :%d\n", api.port())

# Admin/metrics server on a different port
admin = http.server()
admin.handle("GET /", admin_handler)
admin.handle("GET /metrics", metrics_handler)
admin.serve(port=9090)  # blocks until shutdown
