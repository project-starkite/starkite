---
title: "Build an MCP-backed agent"
description: "Connect to an MCP server, wrap each tool, hand them to ai.chat"
weight: 10
---

# Build an MCP-backed agent

[MCP (Model Context Protocol)](https://modelcontextprotocol.io) is an open protocol that lets LLM clients call tools hosted by external servers — filesystem access, database queries, SaaS-API integration, and so on. The MCP ecosystem ships ready-made servers for many common surfaces, so an agent that needs those tools doesn't reimplement them.

Starkite's `mcp` module connects to an MCP server (`mcp.connect`) over stdio or HTTP and exposes each remote tool as a regular Starlark callable. Those callables hand directly to `ai.chat(tools=...)`, where the agent sees them alongside any native Starlark tools and calls them by name. The pattern: open the session, wrap each tool in a small Starlark function (the docstring becomes the tool description, and the wrapper is the natural place for argument coercion or validation), run the agent, close the session. Requires the AI edition (`kiteai` or `kite`).

**Source:** [`ai/examples/agent/mcp_integration.star`](https://github.com/project-starkite/starkite/blob/main/ai/examples/agent/mcp_integration.star)

## Script

```python
#!/usr/bin/env kite

# 1. Connect to an MCP server. stdio subprocess or HTTP — both work.
client = mcp.connect(["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"])
# or:  client = mcp.connect("http://localhost:8080/mcp")

# 2. Wrap each remote tool. Wrapping (vs. calling client.tools.<name> directly)
#    leaves room to add logging, argument coercion, or validation.
def read_file(path):
    """Read a file from the MCP-exposed filesystem."""
    return client.call("read_file", path=path).text

def list_directory(path):
    """List a directory's entries via the MCP server."""
    return client.call("list_directory", path=path).text

# 3. Run an agent that uses those tools. Same shape as a normal ai.chat
#    with native Starlark tools.
chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You help the user inspect files. Use the tools.",
    tools  = [read_file, list_directory],
)

resp = chat.send("What's in /tmp?")
print(resp.text)

# 4. Clean up.
client.close()
```

## Run it

Requires `kiteai` (or `kite`) and `ANTHROPIC_API_KEY` set in the environment:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
kiteai run ai/examples/agent/mcp_integration.star
```

## What's happening

- `mcp.connect(transport)` opens a session. The transport is either a `list[string]` (stdio subprocess argv) or a URL string (HTTP/streamable-HTTP).
- `client.tools.<name>` is the auto-discovered tool surface. Wrapping each in a Starlark `def` gives a place to add cross-cutting concerns and lets `ai.chat` pick up the function's docstring as the tool description.
- `ai.chat(tools=[...])` accepts any list of callables — native Starlark functions, MCP wrappers, or other callable values. The agent loop calls them transparently.

## See also

- [AI > Building agents](../../ai/guides/agents.md) — four agent composition patterns
- [AI > MCP](../../ai/guides/mcp.md) — server side and deeper client patterns
- [`ai` reference](../../references/api/ai.md) — `chat`, `generate`, tools, streaming
- [`mcp` reference](../../references/api/mcp.md) — `serve`, `connect`, full client API
