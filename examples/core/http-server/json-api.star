#!/usr/bin/env kite
# json-api.star - JSON API with multiple routes
#
# Returning a dict without a "body" key auto-serializes as JSON.
# Returning a dict with "body", "status", and "headers" keys gives
# explicit control over the response.
#
# Run:   kite run examples/core/http-server/json-api.star
# Test:  curl http://localhost:8080/api/users
#        curl http://localhost:8080/api/health

users = [
    {"id": 1, "name": "Alice", "role": "admin"},
    {"id": 2, "name": "Bob",   "role": "user"},
    {"id": 3, "name": "Carol", "role": "user"},
]

def list_users(req):
    """Return all users as JSON (auto-serialized)."""
    return {"users": users, "count": len(users)}

def health(req):
    """Health check with explicit status and headers."""
    return {
        "status": 200,
        "headers": {"X-Health": "ok"},
        "body": "healthy",
    }

srv = http.server()
srv.handle("GET /api/users", list_users)
srv.handle("GET /api/health", health)
srv.serve(port=8080)
