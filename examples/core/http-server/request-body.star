#!/usr/bin/env kite
# request-body.star - Handling POST/PUT request bodies
#
# The request body is available as req.body (a string).
# Parse JSON bodies with json.decode().
#
# Run:   kite run examples/core/http-server/request-body.star
# Test:  curl -X POST http://localhost:8080/echo -d 'hello world'
#        curl -X POST http://localhost:8080/api/items \
#             -H 'Content-Type: application/json' \
#             -d '{"name": "widget", "price": 9.99}'

items = []

def echo(req):
    """Echo back the raw request body."""
    return {
        "status": 200,
        "body": req.body,
    }

def create_item(req):
    """Parse JSON body and add to items list."""
    body = req.body
    if not body:
        return {"status": 400, "body": "request body required"}

    item = json.decode(body)
    item["id"] = len(items) + 1
    items.append(item)

    return {
        "status": 201,
        "headers": {"Content-Type": "application/json"},
        "body": json.encode(item),
    }

def list_items(req):
    """Return all items."""
    return {"items": items, "count": len(items)}

srv = http.server()
srv.handle("POST /echo", echo)
srv.handle("POST /api/items", create_item)
srv.handle("GET /api/items", list_items)
srv.serve(port=8080)
