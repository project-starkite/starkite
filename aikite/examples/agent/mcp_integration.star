#!/usr/bin/env kite
# mcp_integration.star — Discover and call tools from an MCP server.
#
# Usage:
#   kite run ./mcp_integration.star --permissions=allow-local

# 1. Connect to an MCP server (stdio subprocess or HTTP endpoint):
client = mcp.connect(["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"])
# or: client = mcp.connect("http://localhost:8080/mcp")

# 2. Inspect available tools exposed by the remote server:
printf("Connected. Available tools:\n")
for t in client.tools:
    printf("  - %s: %s\n", t.name, t.description)

# 3. Call a remote tool directly:
result = client.call("list_directory", path="/tmp")
print("\nDirectory contents:")
print(result.text)

# 4. Clean up connection:
client.close()
