---
title: "Clients"
description: "Make HTTP requests with the http.url object"
weight: 1
---

# HTTP clients

When a script needs to reach an outside service — fetch a config, call an API, post a webhook — it does so through the `http` module's client. Every request starts from a single object, `http.url`, which you build from an address and then act on. There is no client to construct and no session to manage: you name a URL and call the method you want.

## Making requests

You begin by handing `http.url(...)` an address. It returns a URL object, and the verb you call on it decides the request:

```python
resp = http.url("https://api.example.com/data").get()
print(resp.status_code)
data = json.decode(resp.body)
```

The URL object exposes `get`, `post`, `put`, `patch`, and `delete`. Each one sends its request and returns an `http.response` carrying the `status_code`, the `body` as text, and the response `headers` — which is why the example reads `resp.status_code` straight off the return and decodes `resp.body` as JSON without an intervening step.

## Headers, bodies, and timeouts

Real requests rarely stop at a bare GET. Methods that send data take a body as their first argument, and every method accepts `headers` and a `timeout` as keywords:

```python
resp = http.url("https://api.example.com/users").post(
    json.encode({"name": "alice"}),
    headers={"Content-Type": "application/json"},
    timeout="10s",
)
if resp.status_code == 201:
    print("created")
```

Here the encoded JSON is the POST body, the `Content-Type` header tells the server how to read it, and `timeout="10s"` caps how long the call may wait — a duration string, not a number — so a slow endpoint fails fast instead of hanging the script. The response is the same `http.response` as before, so you check `status_code` to confirm the resource was created.

## Error handling

A network call can fail before any status code comes back — the host is down, DNS does not resolve, the connection times out. By default those failures raise and stop the script. When you would rather inspect the failure than abort on it, each request method has a `try_` variant that returns a [`Result`](../../fundamentals/language.md#error-handling) instead of raising:

```python
result = http.url("https://unreachable.example.com").try_get()
if result.ok:
    print(result.value.status_code)
else:
    print("request failed:", result.error)
```

A `Result` reports success through `result.ok`. On success the response is in `result.value` — note that the status code lives one level down, at `result.value.status_code` — and on failure the reason is in `result.error`. The cost of `try_` is that you now branch on every call site; reach for it when a failed request is a condition to handle rather than a reason to stop.

## Permissions

Outbound requests are a gated capability. The client lives under the `http.client` rule, which the `allow-net` profile grants, so a script that makes requests needs at least that profile — under the default `deny-all`, an `http.url(...).get()` is denied before it leaves the process. See [Permission](../../fundamentals/security/permission.md) for the profile ladder.

See the [http API reference](../../references/api/http.md#httpurl) for the full request and response surface. To build a server, see [HTTP servers](servers.md).
