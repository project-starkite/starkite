---
title: "MCP"
description: "Model Context Protocol — server and client"
weight: 20
---

# MCP

The `mcp` module gives starkite both sides of the [Model Context Protocol](https://modelcontextprotocol.io): use `mcp.serve()` to expose tools, resources, and prompts to an LLM client (such as Claude Desktop or your own `genai`-based agent), or use `mcp.connect()` to consume an existing MCP server.

!!! info "Coming soon"
    A full walkthrough — exposing a starkite script as an MCP server and consuming it from a `kiteai` agent — is in progress.

## See also

- [`mcp` API reference](../../references/api/mcp.md)
- [Agents](agents.md)
