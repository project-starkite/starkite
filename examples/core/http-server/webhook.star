#!/usr/bin/env kite
# webhook.star - Webhook receiver and processor
#
# Receives webhook POST payloads, logs them, and runs a local
# command in response. Useful for CI/CD, GitHub webhooks, or
# any event-driven automation.
#
# Run:   kite run examples/core/http-server/webhook.star
# Test:  curl -X POST http://localhost:8080/webhook \
#             -H 'Content-Type: application/json' \
#             -d '{"event": "push", "repo": "starkite", "branch": "main"}'

def logging_mw(req, next):
    printf("[webhook] %s %s from %s\n", req.method, req.path, req.remote_addr)
    return next(req)

def receive_webhook(req):
    body = req.body
    if not body:
        return {"status": 400, "body": "empty payload"}

    payload = json.decode(body)
    event = payload.get("event", "unknown")
    repo = payload.get("repo", "unknown")
    branch = payload.get("branch", "unknown")

    printf("  event=%s repo=%s branch=%s\n", event, repo, branch)

    # React to specific events
    if event == "push" and branch == "main":
        printf("  -> triggering build for %s\n", repo)
        # result = local.exec("make build")

    return {
        "status": 200,
        "headers": {"Content-Type": "application/json"},
        "body": json.encode({"received": True, "event": event}),
    }

def health(req):
    return "ok"

srv = http.server()
srv.use(logging_mw)
srv.handle("POST /webhook", receive_webhook)
srv.handle("GET /health", health)

printf("Webhook server listening on :8080\n")
srv.serve(port=8080)
