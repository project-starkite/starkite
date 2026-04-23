#!/usr/bin/env kite
# path-params.star - Route path parameters (Go 1.22+ ServeMux patterns)
#
# Uses {name} placeholders in route patterns. Parameters are available
# in req.params as a dict.
#
# Run:   kite run examples/core/http-server/path-params.star
# Test:  curl http://localhost:8080/users/42
#        curl http://localhost:8080/users/42/posts/7
#        curl http://localhost:8080/files/css/style.css

users = {
    "42": {"id": 42, "name": "Alice"},
    "99": {"id": 99, "name": "Bob"},
}

def get_user(req):
    """Single path parameter: /users/{id}."""
    user_id = req.params["id"]
    user = users.get(user_id)
    if not user:
        return {"status": 404, "body": "user not found"}
    return user

def get_user_post(req):
    """Multiple path parameters: /users/{user_id}/posts/{post_id}."""
    return {
        "user_id": req.params["user_id"],
        "post_id": req.params["post_id"],
    }

def serve_file(req):
    """Wildcard path parameter: /files/{path...} captures the rest of the URL."""
    return {"file": req.params["path"]}

srv = http.server()
srv.handle("GET /users/{id}", get_user)
srv.handle("GET /users/{user_id}/posts/{post_id}", get_user_post)
srv.handle("GET /files/{path...}", serve_file)
srv.serve(port=8080)
