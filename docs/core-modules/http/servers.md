---
title: "Servers"
description: "Build HTTP servers with routes, handlers, and middleware"
weight: 2
---

# HTTP servers

The `http` module provides an HTTP server to receive and process incoming network requests. This allows Starkite scripts to expose health endpoints, listen for webhook payloads, or build internal APIs that respond to external clients.

Serving HTTP requests requires server-binding permissions. Grant access using the `--permissions=allow-local` flag:

```bash
kite run ./server.star --permissions=allow-local
```

## A minimal server

To start a server:

1. Define a handler function that accepts a request object as its argument.
2. Create a server instance using `http.server()`.
3. Register the handler to a route pattern using `srv.handle(pattern, handler)`.
4. Start the listener using `srv.serve(port)`.

The `srv.serve()` method blocks execution, keeping the script active to handle incoming requests:

```python
def hello(req):
    name = req.query.get("name", "World")
    return {"status": 200, "body": "Hello, " + name}

srv = http.server()
srv.handle("GET /hello", hello)
srv.serve(port=8080)
```

## Routing and route mapping

Route patterns follow the `"METHOD /path"` format, such as `"GET /hello"` or `"POST /users"`. The server matches incoming requests to registered route handlers.

### Routing with path variables

To define dynamic path variables in a route, use curly braces (e.g., `{id}`). The server extracts these variables from the request path and exposes them in the handler through the `req.params` dictionary:

```python
def get_user(req):
    # Retrieve the path parameter 'id'
    user_id = req.params.get("id")
    
    return {
        "status": 200,
        "headers": {"Content-Type": "application/json"},
        "body": json.encode({"user_id": user_id, "status": "active"}),
    }

srv = http.server()
srv.handle("GET /users/{id}", get_user)
srv.serve(port=8080)
```

### Declarative route mapping

For simple servers with short handlers, you can use the `http.serve(route_map, port)` shortcut. This function combines server creation, routing, and starting the listener into a single blocking call:

```python
http.serve({
    "GET /healthz": lambda req: {"status": 200, "body": "ok"},
    "GET /version": lambda req: {"status": 200, "body": "v1"},
}, port=8080)
```


## Request and response objects

Starkite maps the lifecycle of an HTTP transaction directly to Starlark data structures. Each handler function receives a structured request object as its argument, which encapsulates all incoming client data—such as query strings, HTTP headers, path parameters, and request payloads. To respond to the client, the handler must return a Starlark dictionary. The server parses this dictionary to construct the outgoing HTTP response status, headers, and body.

The following example demonstrates a handler reading query parameters and headers from the request object, and returning a formatted response dictionary:

```python
def handle_transaction(req):
    # Read query parameters and headers from the request object
    action = req.query.get("action", "read")
    auth = req.headers.get("Authorization", "none")
    
    # Return a dictionary defining the HTTP response status, headers, and body
    return {
        "status": 200,
        "headers": {
            "Content-Type": "text/plain",
            "X-Server-Engine": "Starkite",
        },
        "body": "Action: " + action + ", Auth: " + auth,
    }

srv = http.server()
srv.handle("GET /transaction", handle_transaction)
srv.serve(port=8080)
```

### Request properties

The request object exposes incoming data through the following attributes:

* **`req.method`**: The HTTP method string (e.g., `"GET"`, `"POST"`).
* **`req.path`**: The request path string (e.g., `"/hello"`).
* **`req.query`**: A dictionary containing query string parameters.
* **`req.params`**: A dictionary containing path variables extracted from route patterns.
* **`req.headers`**: A dictionary of incoming request headers.
* **`req.body`**: The raw request body as a string.

### Response dictionary structure

The dictionary returned by the handler defines the HTTP response:

* **`status`**: The integer HTTP status code (e.g., `200`, `201`, `400`).
* **`body`**: A string representing the response payload.
* **`headers`**: An optional dictionary of HTTP headers to return to the client.

Returning `None` from a handler is a shorthand for returning `{"status": 204}` (No Content).

```python
def create_user(req):
    # Decode the incoming JSON body
    body = json.decode(req.body)
    
    # Return a 201 Created status and JSON payload
    return {
        "status": 201,
        "headers": {"Content-Type": "application/json"},
        "body": json.encode({"created": body["name"]})
    }

srv = http.server()
srv.handle("POST /users", create_user)
srv.serve(port=8080)
```



## Middleware

Middleware functions allow you to wrap every handler to implement cross-cutting concerns, such as logging, authentication, or common header injection.

A middleware function takes a `next` handler function as an argument and returns a new handler function:

```python
def log_requests(next_handler):
    def handler(req):
        log.info("request", {"method": req.method, "path": req.path})
        return next_handler(req)
    return handler

def hello(req):
    return {"status": 200, "body": "Hello World"}

srv = http.server()
srv.use(log_requests)
srv.handle("GET /hello", hello)
srv.serve(port=8080)
```

## Server configuration and TLS

You can configure timeouts, payload size limits, and TLS certificates by passing options to `http.server()`:

* **Timeouts**: Pass `read_timeout`, `write_timeout`, `idle_timeout`, or `shutdown_timeout` as duration strings (e.g., `"30s"`).
* **Payload limits**: Pass `max_body_bytes` as an integer to cap the maximum request payload size.
* **TLS (HTTPS)**: Pass `tls_cert` and `tls_key` as file paths to enable HTTPS.

```python
# Configure a secure server with explicit timeouts
srv = http.server(
    read_timeout="15s",
    write_timeout="30s",
    max_body_bytes=1048576, # 1MB limit
    tls_cert="/etc/ssl/certs/server.crt",
    tls_key="/etc/ssl/certs/server.key",
)
```

## Asynchronous execution and dynamic ports

For integration testing or background tasks, you can start the server asynchronously (non-blocking) using the `srv.start()` method instead of the blocking `srv.serve()` method. 

By passing `port=0`, the operating system automatically allocates an available free port. You can retrieve the assigned port at runtime by calling `srv.port()`:

```python
def hello(req):
    return {"status": 200, "body": "Hello World"}

def main():
    srv = http.server()
    srv.handle("GET /hello", hello)
    
    # Start the server asynchronously on a random free port
    srv.start(port=0)
    
    # Retrieve the dynamically assigned port
    p = srv.port()
    print("Server started in background on port:", p)

    # Query the background server using the HTTP client
    resp = http.url("http://127.0.0.1:" + str(p) + "/hello").get()
    print("Response body:", resp.get_text())

    # Gracefully shut down the server
    srv.shutdown()
    print("Server shut down successfully")
```

## See also

* [`http` API reference](../../references/api/http.md#httpserver) — Detailed server function signatures and properties.
* [HTTP clients](clients.md) — Make outbound HTTP requests in Starkite.
