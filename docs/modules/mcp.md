---
title: "mcp"
description: "Model Context Protocol server and client over stdio, HTTP, and TLS"
weight: 29
edition: ai
---

!!! note "AI edition only"
    The `mcp` module is only available in the `kite-ai` edition. See [AI Edition](../guides/ai-edition.md).

The `mcp` module exposes [Model Context Protocol](https://modelcontextprotocol.io) primitives. Scripts can **serve** their own MCP server (expose Starlark tools, resources, and prompts over stdio or HTTP) and **connect** to remote MCP servers (call tools, read resources, fetch prompts) as a client.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `mcp.serve(name=, tools=, resources=, prompts=, port=, host=, path=, tls_cert=, tls_key=, version=)` | `None` | Start a blocking MCP server (stdio or HTTP) |
| `mcp.connect(transport, timeout=)` | `MCPClient` | Connect to a remote MCP server |

## `mcp.serve()`

Starts an MCP server that blocks until interrupted (SIGINT/SIGTERM). Returns `None` on clean shutdown; returns an error if the server fails to bind or exits abnormally.

### Stdio transport (default)

Omit `port=` to serve over stdin/stdout — the standard MCP transport:

```python
def search_docs(query):
    """Search internal documentation."""
    return [f.name for f in fs.glob("docs/**/*.md") if query.lower() in fs.read_text(f).lower()]

def run_query(sql):
    """Run a read-only SQL query."""
    return [row for row in db.query(sql)]

mcp.serve(
    name    = "docs-helper",
    tools   = [search_docs, run_query],
)
```

Connect to it from any MCP client (Claude Desktop, etc.) by pointing the client at this script.

### HTTP transport

Set `port=` to expose the server over streamable HTTP:

```python
mcp.serve(
    name  = "team-tools",
    tools = [search_docs, run_query],
    port  = 8080,
    host  = "0.0.0.0",         # default: 127.0.0.1
    path  = "/mcp",            # default: "/"
)
```

### HTTPS transport

Set `tls_cert=` and `tls_key=` alongside `port=` for TLS:

```python
mcp.serve(
    name     = "secured",
    tools    = [search_docs],
    port     = 8443,
    tls_cert = "/etc/certs/server.crt",
    tls_key  = "/etc/certs/server.key",
)
```

Both TLS kwargs must be set together; setting only one errors at validation time.

### `mcp.serve()` kwarg reference

| Kwarg | Type | Description |
|-------|------|-------------|
| `name` | string | Server identifier (required) |
| `version` | string | Server version (default `"0.1.0"`) |
| `tools` | list | Tool callables (plain functions or `ai.tool(fn)` wrappers). Schema auto-inferred from signature/docstring — same rules as `ai.tool()` |
| `resources` | dict | `{uri: fn}` mapping URIs to read handlers. `fn` takes the URI string and returns text |
| `prompts` | dict | `{name: fn}` mapping prompt names to templates. `fn` takes optional arguments and returns the rendered prompt string |
| `port` | int | Listen port; omit or 0 for stdio |
| `host` | string | Bind address (default `"127.0.0.1"`); only valid when `port` is set |
| `path` | string | HTTP path to mount the MCP handler (default `"/"`); only valid when `port` is set |
| `tls_cert` | string | Path to PEM certificate; must be paired with `tls_key` |
| `tls_key` | string | Path to PEM private key; must be paired with `tls_cert` |

### Resource and prompt handlers

Resources expose read-only content identified by URI:

```python
def pod_status(uri):
    ns = uri.split("/")[-1]
    return json.encode([{"name": p.name, "phase": p.status.phase}
                        for p in k8s.list("pods", namespace=ns)])

mcp.serve(
    name      = "cluster-reader",
    resources = {"k8s://pods/default": pod_status},
)
```

Prompts return rendered template text:

```python
def incident_report(severity="low", service="unknown"):
    """Generate an incident report template."""
    return "Severity: %s\nService: %s\nSteps:\n1. " % (severity, service)

mcp.serve(
    name    = "incident-tools",
    prompts = {"incident_report": incident_report},
)
```

## `mcp.connect()`

Connects to a remote MCP server. The first positional argument is the transport:

| Arg shape | Transport |
|-----------|-----------|
| `list[string]` | stdio subprocess: `mcp.connect(["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"])` |
| `string` (http:// or https://) | HTTP streaming: `mcp.connect("http://localhost:8080/mcp")` |

```python
client = mcp.connect(["mcp-server-sqlite", "--db", "./data.db"])
```

| Kwarg | Type | Description |
|-------|------|-------------|
| `timeout` | duration string | Cap on the initial handshake (default `"30s"`). Subsequent calls do not inherit this |

### `MCPClient`

| Attribute | Type | Description |
|-----------|------|-------------|
| `.tools` | namespace | Dynamic: `client.tools.tool_name(**kwargs)` calls a remote tool. Only identifier-safe names are attribute-accessible |
| `.call(name, **kwargs)` | method | Explicit call. Required for tools with hyphens or other non-identifier chars in their names |
| `.close()` | method | Shuts down the session and subprocess (if any). Also auto-run at garbage collection, but explicit close is recommended |

Each entry in `client.tools` is an `MCPTool`:

| Attribute | Type | Description |
|-----------|------|-------------|
| `.name` | string | The tool's name as reported by the server |
| `.description` | string | The tool's description as reported by the server (may be empty) |

Calling an `MCPTool` (e.g., `client.tools.read_file(path="/tmp")`) returns an `MCPResult`:

| Attribute | Type | Description |
|-----------|------|-------------|
| `.text` | string | Concatenated text content (for text-only responses) |
| `.content` | list | Full list of content items; see content shapes below |
| `.is_error` | bool | `True` if the remote tool reported an error |

#### `MCPResult.content` shapes

Each item in `.content` is a dict keyed by content type:

| Type | Keys |
|------|------|
| Text | `{"type": "text", "text": "..."}` |
| Image | `{"type": "image", "data": bytes, "mime_type": "image/png"}` |
| Audio | `{"type": "audio", "data": bytes, "mime_type": "audio/wav"}` |

Unknown or future content types fall back to `{"type": "<server_reported>"}`.

### Using MCP tools with `ai.chat`

Wrap remote MCP tools as local callables, then pass them to a chat session:

```python
client = mcp.connect(["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"])

def read_file(path):
    """Read a file from the MCP-exposed filesystem."""
    return client.call("read_file", path=path).text

def list_directory(path):
    """List a directory's entries via the MCP server."""
    return client.call("list_directory", path=path).text

chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You help the user inspect files. Use the tools.",
    tools  = [read_file, list_directory],
)

resp = chat.send("What's in /tmp?")
print(resp.text)

client.close()
```

See [agents guide — Pattern 4](../guides/agents.md#pattern-4-mcp-integration) for more.

## Examples

### Stdio server exposing a single tool

```python
#!/usr/bin/env kite-ai

def pod_logs(namespace, name, lines=50):
    """Fetch the last N log lines from a pod."""
    return k8s.logs(name, namespace=namespace, tail=lines)

mcp.serve(
    name  = "k8s-ops",
    tools = [pod_logs],
)
```

Register this with an MCP client (e.g., Claude Desktop) by pointing at the script:

```json
{
  "mcpServers": {
    "k8s-ops": {
      "command": "kite-ai",
      "args": ["run", "/path/to/k8s-ops.star"]
    }
  }
}
```

### HTTPS server

```python
mcp.serve(
    name     = "public-tools",
    tools    = [check_url],
    port     = 8443,
    host     = "0.0.0.0",
    path     = "/mcp",
    tls_cert = env("TLS_CERT_PATH"),
    tls_key  = env("TLS_KEY_PATH"),
)
```

### Client connecting to an HTTP MCP server

```python
client = mcp.connect("https://team-mcp.example.com/mcp")
result = client.call("search", query="deploy rollback")
print(result.text)
client.close()
```
