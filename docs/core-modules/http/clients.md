---
title: "Clients"
description: "Make HTTP requests with the http.url object"
weight: 1
---

# HTTP clients

The `http` module makes outbound requests through the `http.url` object. Build a URL, then call a method on it.

## Making requests

```python
resp = http.url("https://api.example.com/data").get()
print(resp.status_code)
data = json.decode(resp.body)
```

`http.url(...)` returns a URL object with `get`, `post`, `put`, `delete`, and other method calls. Each returns an `http.response` with `status_code`, `body`, and `headers`.

## Headers, bodies, and timeouts

```python
resp = http.url("https://api.example.com/users").post(
    json.encode({"name": "alice"}),
    headers={"Content-Type": "application/json"},
    timeout="10s",
)
if resp.status_code == 201:
    print("created")
```

## Error handling

Request methods have `try_` variants that return a [`Result`](../../fundamentals/language.md#error-handling) instead of raising on network failure:

```python
result = http.url("https://unreachable.example.com").try_get()
if result.ok:
    print(result.value.status_code)
else:
    print("request failed:", result.error)
```

See the [http API reference](../../references/api/http.md#httpurl) for the full request/response surface. To build a server, see [HTTP servers](servers.md).
