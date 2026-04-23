---
title: "AI Edition"
description: "kite-ai binary: LLM client, MCP, and agent primitives"
weight: 6
---

The AI edition of starkite adds the [`ai`](../modules/ai.md) module (multi-provider LLM client) and the [`mcp`](../modules/mcp.md) module (Model Context Protocol server + client) to the base edition. Everything else from core is present.

## Installation

```bash
# Build from source
go build -o kite-ai ./cmd/ai/starkite/

# Or install as an edition
kite edition install ai
kite edition activate ai
```

When the AI edition is active, the base `kite` binary automatically delegates to `kite-ai`.

## Provider Credentials

The AI edition supports Anthropic, OpenAI, Google AI, and Ollama. Set the relevant environment variable for any provider you plan to use:

| Provider | Env var | Notes |
|----------|---------|-------|
| Anthropic | `ANTHROPIC_API_KEY` | claude-3-5-sonnet, claude-opus, etc. |
| OpenAI | `OPENAI_API_KEY` | gpt-4o, gpt-4o-mini, etc. |
| Google AI | `GOOGLE_API_KEY` | gemini-1.5-pro, gemini-flash, etc. |
| Ollama | *(none)* | Local; default endpoint `http://localhost:11434`. Override with `ai.config(base_urls={"ollama": "..."})` or per-call `base_url=` |

You can also configure credentials in a script via `ai.config()` — useful for scripts that manage their own credentials:

```python
ai.config(api_keys = {"openai": env("MY_OPENAI_KEY")})
```

## Quick Start

The fastest path to verifying the install uses Ollama (no API key required):

```python
# hello-ai.star
resp = ai.generate("Say hi in 5 words.", model="ollama/llama3.2")
print(resp.text)
```

```bash
kite-ai run hello-ai.star
```

Or with Anthropic once `ANTHROPIC_API_KEY` is set:

```python
resp = ai.generate("Say hi in 5 words.", model="anthropic/claude-sonnet-4-5")
print(resp.text)
```

## Model Strings

Every call identifies the backend by a `provider/model-name` string:

```python
ai.generate(prompt, model="anthropic/claude-sonnet-4-5")
ai.generate(prompt, model="openai/gpt-4o-mini")
ai.generate(prompt, model="googleai/gemini-1.5-pro")
ai.generate(prompt, model="ollama/llama3.2")
```

Set a default so you don't have to repeat it:

```python
ai.config(default_model="anthropic/claude-sonnet-4-5")
resp = ai.generate("...")  # uses the default
```

## What's Included

| Module | Purpose | Reference |
|--------|---------|-----------|
| `ai` | Multi-provider LLM client (generate, chat, tools, agents) | [ai module reference](../modules/ai.md) |
| `mcp` | MCP server (expose Starlark tools over stdio or HTTP) + client (call remote MCP servers) | [mcp module reference](../modules/mcp.md) |

All 27 modules from the base edition (`os`, `fs`, `http`, `ssh`, `json`, `yaml`, `concur`, etc.) remain available. Agents typically compose `ai.chat()` with these for tool implementations — see the [agents guide](agents.md).

## Editions Management

```bash
kite edition list              # List installed editions
kite edition install ai        # Install AI edition
kite edition activate ai       # Set AI as active
kite edition activate base     # Switch back to base
```

## Next Steps

- [AI module reference](../modules/ai.md) — full `ai.generate()` / `ai.chat()` / `ai.tool()` / `ai.run_until()` signatures
- [MCP module reference](../modules/mcp.md) — `mcp.serve()` and `mcp.connect()` with stdio, HTTP, and TLS
- [Building Agents](agents.md) — four composition patterns for multi-turn agents
- [Embedding Starbase](embedding.md#calling-starlark-functions-from-go) — drive agents from Go, with Starlark providing tool bodies
