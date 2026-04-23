#!/usr/bin/env kite
# response-types.star - All supported response types
#
# Handlers can return:
#   1. string        -> 200 text/plain
#   2. None          -> 204 No Content
#   3. dict w/ body  -> explicit {status, headers, body}
#   4. dict w/o body -> auto-serialized as JSON (application/json)
#
# Run:   kite run examples/core/http-server/response-types.star
# Test:  curl http://localhost:8080/text
#        curl -i http://localhost:8080/no-content
#        curl http://localhost:8080/explicit
#        curl http://localhost:8080/json

def text_response(req):
    """Return a plain string -> 200 text/plain."""
    return "Hello, plain text!"

def no_content(req):
    """Return None -> 204 No Content."""
    return None

def explicit_response(req):
    """Return dict with body -> full control over status, headers, body."""
    return {
        "status": 201,
        "headers": {
            "Content-Type": "text/html; charset=utf-8",
            "X-Custom": "example",
        },
        "body": "<h1>Created</h1>",
    }

def json_response(req):
    """Return dict without body key -> auto-serialized as JSON."""
    return {
        "name": "starkite",
        "version": "0.1.0",
        "features": ["http-server", "middleware", "path-params"],
    }

srv = http.server()
srv.handle("GET /text", text_response)
srv.handle("GET /no-content", no_content)
srv.handle("GET /explicit", explicit_response)
srv.handle("GET /json", json_response)
srv.serve(port=8080)
