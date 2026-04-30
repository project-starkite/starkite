#!/usr/bin/env kite
# mcp_integration.star — agent using tools discovered from an MCP server.
#
# Pattern: connect to an external MCP server (stdio subprocess or HTTP), wrap
# each discovered tool in a small Starlark def so ai.chat can use it as a
# regular tool, then run an agent that calls those tools as needed.
#
# This demonstrates Phase 2 (mcp.connect / client.tools) composing with
# Phase 1 (ai.chat / ai.tool) — no special-case plumbing needed.

# 1. Connect to an MCP server. Use whichever transport you need:
client = mcp.connect(["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"])
# or:  client = mcp.connect("http://localhost:8080/mcp")

# 2. Wrap each remote tool in a local function that calls through client.call.
#    (client.tools.<name> is also callable directly, but wrapping via def
#     gives you a chance to add logging, argument coercion, or validation.)
def read_file(path):
    """Read a file from the MCP-exposed filesystem."""
    return client.call("read_file", path=path).text

def list_directory(path):
    """List a directory's entries via the MCP server."""
    return client.call("list_directory", path=path).text

# 3. Run an agent that has access to those tools. It looks exactly the same
#    as a normal ai.chat with native Starlark tools.
chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You help the user inspect files. Use the tools.",
    tools  = [read_file, list_directory],
)

resp = chat.send("What's in /tmp?")
print(resp.text)

# 4. Clean up.
client.close()
