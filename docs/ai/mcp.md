---
title: "Creating MCP servers"
description: "Expose starkite tools, resources, and prompts over the Model Context Protocol"
weight: 20
edition: ai
---

# Creating MCP servers

The `mcp` module implements both sides of the [Model Context Protocol](https://modelcontextprotocol.io). `mcp.serve()` turns a starkite script into an MCP server — exposing tools, resources, and prompts to any MCP client (Claude Desktop, a `genai`-based agent, or another consumer). `mcp.connect()` goes the other way, calling an existing MCP server (covered in [Building agents](agents.md#pattern-4-mcp-integration)).

!!! note "Needs the AI modules"
    The `mcp` module is in the default `kite` binary and the lean `kiteai` edition. See [Editions](../fundamentals/editions.md).

## A minimal server

A tool is a Starlark function. `mcp.serve()` registers tools and blocks, speaking MCP over stdio:

```python
def add(a, b):
    """Add two numbers."""
    return a + b

def hostname():
    """Return the server's hostname."""
    return os.hostname()

mcp.serve(tools=[add, hostname])
```

The function docstring becomes the tool description, and the signature becomes the input schema — the same inference used by [`ai.tool`](agents.md). An MCP client connecting over stdio sees `add` and `hostname` as callable tools.

## Tools, resources, and prompts

`mcp.serve()` accepts three kinds of capability:

```python
mcp.serve(
    tools     = [add, hostname],
    resources = {"config://app": lambda: json.encode({"env": "prod"})},
    prompts   = {"greeting": lambda name: "Say hello to " + name},
)
```

- **Tools** — callable functions the client invokes.
- **Resources** — named, readable content the client can fetch.
- **Prompts** — parameterized prompt templates.

## Running and connecting a client

Run the server script like any other:

```bash
kite run ./my-mcp-server.star
```

A client launches it as a stdio subprocess. For example, configuring Claude Desktop to spawn `kite run /path/to/my-mcp-server.star` exposes its tools to the assistant.

## See also

- [`mcp` API reference](../references/api/mcp.md) — full `serve`/`connect` surface, schema control, resource and prompt registration
- [Building agents — MCP integration](agents.md#pattern-4-mcp-integration) — consuming an MCP server from an agent
