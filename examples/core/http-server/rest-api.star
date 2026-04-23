#!/usr/bin/env kite
# rest-api.star - Complete REST API with CRUD operations
#
# Demonstrates method-aware routing (GET, POST, PUT, DELETE),
# path parameters, JSON request/response, and error handling.
#
# Run:   kite run examples/core/http-server/rest-api.star
# Test:
#   curl http://localhost:8080/api/tasks
#   curl -X POST http://localhost:8080/api/tasks \
#        -H 'Content-Type: application/json' \
#        -d '{"title": "Learn starkite", "done": false}'
#   curl http://localhost:8080/api/tasks/1
#   curl -X PUT http://localhost:8080/api/tasks/1 \
#        -H 'Content-Type: application/json' \
#        -d '{"title": "Learn starkite", "done": true}'
#   curl -X DELETE http://localhost:8080/api/tasks/1

next_id = [1]
tasks = {}

def list_tasks(req):
    return {"tasks": [t for t in tasks.values()]}

def create_task(req):
    body = req.body
    if not body:
        return {"status": 400, "body": "request body required"}

    task = json.decode(body)
    task["id"] = next_id[0]
    next_id[0] += 1
    tasks[str(task["id"])] = task

    return {
        "status": 201,
        "headers": {"Content-Type": "application/json"},
        "body": json.encode(task),
    }

def get_task(req):
    task_id = req.params["id"]
    task = tasks.get(task_id)
    if not task:
        return {"status": 404, "body": "task not found"}
    return task

def update_task(req):
    task_id = req.params["id"]
    if task_id not in tasks:
        return {"status": 404, "body": "task not found"}

    updated = json.decode(req.body)
    updated["id"] = int(task_id)
    tasks[task_id] = updated
    return updated

def delete_task(req):
    task_id = req.params["id"]
    if task_id not in tasks:
        return {"status": 404, "body": "task not found"}

    tasks.pop(task_id)
    return None  # 204 No Content

srv = http.server()
srv.handle("GET /api/tasks", list_tasks)
srv.handle("POST /api/tasks", create_task)
srv.handle("GET /api/tasks/{id}", get_task)
srv.handle("PUT /api/tasks/{id}", update_task)
srv.handle("DELETE /api/tasks/{id}", delete_task)
srv.serve(port=8080)
