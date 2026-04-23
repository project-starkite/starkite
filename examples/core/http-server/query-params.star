#!/usr/bin/env kite
# query-params.star - Query string parameter handling
#
# Query parameters are available in req.query as a dict.
# Single values are strings; repeated keys become lists.
#
# Run:   kite run examples/core/http-server/query-params.star
# Test:  curl 'http://localhost:8080/search?q=starkite&limit=10'
#        curl 'http://localhost:8080/filter?tag=go&tag=cli&tag=starlark'

items = [
    {"name": "starctl", "tags": ["go", "cli"]},
    {"name": "starbase", "tags": ["go", "starlark"]},
    {"name": "startype", "tags": ["go", "starlark", "types"]},
]

def search(req):
    """Search with optional query parameters."""
    q = req.query.get("q", "")
    limit = int(req.query.get("limit", "10"))

    results = [item for item in items if q in item["name"]]
    return {"query": q, "limit": limit, "results": results[:limit]}

def filter_by_tags(req):
    """Filter using repeated query parameter (?tag=a&tag=b).
    Repeated keys arrive as a list."""
    tags = req.query.get("tag", [])
    # Single value comes as string, normalize to list
    if type(tags) == "string":
        tags = [tags]

    results = []
    for item in items:
        if any([t in item["tags"] for t in tags]):
            results.append(item)

    return {"tags": tags, "results": results}

srv = http.server()
srv.handle("GET /search", search)
srv.handle("GET /filter", filter_by_tags)
srv.serve(port=8080)
