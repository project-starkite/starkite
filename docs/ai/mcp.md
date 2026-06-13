---
title: "Creating MCP servers"
description: "Expose starkite tools, resources, and prompts over the Model Context Protocol"
weight: 20
edition: ai
---

# Creating MCP servers

The Model Context Protocol is how an MCP client — Claude Desktop, a `genai`-based agent, or any other consumer — discovers and calls tools that live outside it. The `mcp` module implements both sides of that protocol, so a starkite script can be either end of the connection. `mcp.serve()` turns a script into an MCP server, publishing its tools, resources, and prompts for a client to use; `mcp.connect()` runs the other direction, calling an existing server as a client (covered in [Building agents](agents.md#pattern-4-mcp-integration)). This page is about serving.

!!! note "Needs the AI modules"
    The `mcp` module is in the default `kite` binary and the lean `kiteai` edition. See [Editions](../fundamentals/editions.md).

## A minimal server

Start with the smallest server that does something. A tool is nothing more than a Starlark function; you hand the functions you want to expose to `mcp.serve()`, and it registers them and blocks, speaking MCP over stdio until the process is stopped:

```python
def add(a, b):
    """Add two numbers."""
    return a + b

def hostname():
    """Return the server's hostname."""
    return os.hostname()

mcp.serve(tools=[add, hostname])
```

You wrote two plain functions and a one-line call, and a client connecting over stdio now sees `add` and `hostname` as callable tools. The description and the schema were not declared anywhere — `mcp.serve()` infers them: the function docstring becomes the tool description, and the signature becomes the input schema, the same inference [`ai.tool`](agents.md) uses. The cost of that convenience is that the docstring and parameter names are the contract the client reads, so write them as documentation, not as notes to yourself.

## Tools, resources, and prompts

Tools are only one of three things a server can offer, and `mcp.serve()` accepts all three through separate keyword arguments:

```python
mcp.serve(
    tools     = [add, hostname],
    resources = {"config://app": lambda: json.encode({"env": "prod"})},
    prompts   = {"greeting": lambda name: "Say hello to " + name},
)
```

Each argument answers a different question the client might ask. **Tools** are callable functions the client invokes to make something happen. **Resources** are named, readable content the client fetches by URI — configuration, status, a document. **Prompts** are parameterized templates the client renders to build a message. You expose whichever of the three your server has reason to; passing only `tools=` is common.

## Running and connecting a client

A server script is still just a script, so you run it the way you run any other:

```bash
kite run ./my-mcp-server.star
```

You rarely run it by hand, though. Because the default transport is stdio, the client launches the script as a subprocess and talks to it over the process's standard input and output — so configuring Claude Desktop to spawn `kite run /path/to/my-mcp-server.star` is what actually exposes the script's tools to the assistant. The client owns the process lifecycle; your script's job is to register its capabilities and block.

One thing the server needs that a pure-compute script does not is permission. Serving over MCP is a gated capability: `mcp.serve()` is granted by the `allow-local` profile, so a server started under a tighter profile fails at the call. Run it with at least that rung — see [Permission](../fundamentals/security/permission.md) for the full ladder.

## See also

- [`mcp` API reference](../references/api/mcp.md) — full `serve`/`connect` surface, schema control, resource and prompt registration
- [Building agents — MCP integration](agents.md#pattern-4-mcp-integration) — consuming an MCP server from an agent
