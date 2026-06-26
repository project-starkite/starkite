---
title: "Clients"
description: "Make HTTP requests with the http.url object"
weight: 1
---

# HTTP clients

The `http` module provides an HTTP client to perform network requests, such as retrieving remote configurations, interacting with web APIs, or dispatching webhooks. Every request begins with the `http.url(address)` constructor, which returns a URL object. You execute requests by calling HTTP verb methods directly on the URL object.

Running scripts that perform outbound HTTP requests requires network permissions. Grant access using the `--permissions=allow-net` flag:

```bash
kite run ./fetch.star --permissions=allow-net
```

## Making requests

Use `http.url(address)` to create a URL object. The object supports standard HTTP request methods: `get()`, `post()`, `put()`, `patch()`, and `delete()`. 

These methods execute the request synchronously and return an `http.response` object containing the following properties:

* **`status_code`**: The integer HTTP status code returned by the server.
* **`status`**: The status text string (e.g., `"200 OK"`).
* **`body`**: The raw response payload as a byte array (`bytes`).
* **`headers`**: A dictionary of response headers.

Use `resp.get_text()` to retrieve the payload as a UTF-8 string, and `resp.get_bytes()` (or the `body` property) to retrieve the raw bytes.

```python
resp = http.url("https://api.example.com/data").get()

# Print the status code
print(resp.status_code)

# Decode and print the body text
print(resp.get_text())
```

## Global client configuration

To configure global settings for all subsequent HTTP requests in the script, use `http.config()`. You can specify a default timeout and a dictionary of default headers (such as authorization tokens) to be included in every request:

```python
http.config(
    timeout="10s",
    headers={"Authorization": "Bearer secret-token"},
)
```

## Request payloads and options

When using methods that transmit payloads (`post()`, `put()`, and `patch()`), pass the request body as the first positional argument. All request methods accept optional keyword arguments to configure the request:

* **`headers`**: A dictionary of HTTP headers to include in the request.
* **`timeout`**: A duration string (e.g., `"5s"` or `"500ms"`) to limit execution time.

```python
# Send a POST request with a raw text body and custom headers
resp = http.url("https://api.example.com/logs").post(
    "Application log entry",
    headers={"Content-Type": "text/plain", "X-App-Id": "starkite-123"},
)

# Send a GET request with a custom header and a timeout limit
resp = http.url("https://api.example.com/status").get(
    headers={"Accept": "application/json"},
    timeout="5s",
)
```

### Auto-JSON serialization for dictionaries

Passing a Starlark dictionary as the body parameter automatically serializes it to JSON and sets the `Content-Type` header to `application/json`:

```python
# The dictionary is serialized to JSON and sent with the appropriate Content-Type header
resp = http.url("https://api.example.com/users").post({"name": "alice"})
```

## HTTP response streaming

For memory-efficient HTTP operations (such as downloading large payloads or piping data directly between services), `http.response` supports streaming reads. Every response represents an active connection, and the script determines how to consume it:

* **`resp.get_reader()`**: Returns an `io.reader` stream wrapping the active response body.
* **`resp.get_text()`** / **`resp.get_bytes()`** / **`resp.body`**: Lazy-load and cache the entire body in memory on-demand.

Once `resp.get_reader()` is called, the stream is opened, and direct properties like `.body` cannot be read (to prevent resource conflicts). If `.body` or `get_text()` is called first, the body is buffered in memory, and subsequent `get_reader()` calls will return a reader over the cached memory buffer.

### Streaming examples

```python
def download_large_payload():
    resp = http.url("https://backups.example.com/archive.tar.gz").get()
    
    # Stream the download directly to a file on disk
    # This avoids buffering the large file in memory
    archive = path("/tmp/archive.tar.gz")
    archive.read_from(resp.get_reader())
```

For more details on managing streams and piping data to files or subprocesses, see [Filesystem Polymorphic Streaming](../files.md#polymorphic-streaming).


## Error handling

Network failures, DNS resolution errors, and timeouts halt script execution. To handle these failures programmatically, use request methods prefixed with `try_` (such as `try_get()` or `try_post()`), which return a `Result` object.

The `Result` object contains the following properties:
* **`ok`**: A boolean indicating if the request succeeded.
* **`value`**: The `http.response` object (available only when `ok` is `True`).
* **`error`**: A string describing the failure (available only when `ok` is `False`).

Because Starlark restricts control flow (like `if` statements) to function bodies, wrap error-handling logic in a function:

```python
def fetch_user_safely():
    result = http.url("https://api.example.com/users/1").try_get()

    if result.ok:
        print("Status code:", result.value.status_code)
        print("Body text:", result.value.get_text())
    else:
        print("Request failed:", result.error)
```

## See also

* [`http` API reference](../../references/api/http.md#httpurl) — Detailed client function signatures and properties.
* [HTTP servers](servers.md) — Expose HTTP endpoints in Starkite.
