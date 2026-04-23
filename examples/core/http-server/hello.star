#!/usr/bin/env kite
# hello.star - Minimal HTTP server
#
# Run:   kite run examples/core/http-server/hello.star
# Test:  curl http://localhost:8080/hello

def handler(req):
    return "Hello from starkite!"

http.serve({"GET /hello": handler}, port=8080)
