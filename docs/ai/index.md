---
title: "AI"
description: "LLM clients, MCP, and agent primitives in starkite"
weight: 60
---

# AI

Starkite's AI edition ships two modules: [`ai`](../references/api/ai.md) for multi-provider LLM access (Anthropic, OpenAI, Google AI, Ollama) and [`mcp`](../references/api/mcp.md) for the Model Context Protocol — both a server (`mcp.serve()`) and a client (`mcp.connect()`). Install with `kitecmd edition use ai`, or use the all-in-one `kite` binary.

<div class="grid cards" markdown>

-   :material-robot:{ .lg .middle } __Building agents__

    ---

    Compose `ai.chat()` with tools, history, and the `ai.run_until()` driver for multi-turn agent loops.

    [:octicons-arrow-right-24: Read more](guides/agents.md)

-   :material-connection:{ .lg .middle } __MCP__

    ---

    Expose starkite tools over MCP, or call existing MCP servers from `kiteai` agents.

    [:octicons-arrow-right-24: Read more](guides/mcp.md)

-   :material-api:{ .lg .middle } __API reference__

    ---

    The full `ai` and `mcp` module surfaces — generate, chat, tool registration, streaming, structured output.

    [:octicons-arrow-right-24: ai](../references/api/ai.md) · [:octicons-arrow-right-24: mcp](../references/api/mcp.md)

-   :material-folder-open:{ .lg .middle } __Examples__

    ---

    Runnable agent and MCP scripts demonstrating working patterns.

    [:octicons-arrow-right-24: Browse](examples.md)

</div>

## Quick taste

The fastest path uses Ollama (no API key required):

```python
# hello-ai.star
resp = ai.generate("Say hi in 5 words.", model="ollama/llama3.2")
print(resp.text)
```

```bash
kiteai run hello-ai.star
```

Or with Anthropic once `ANTHROPIC_API_KEY` is set:

```python
resp = ai.generate("Say hi in 5 words.", model="anthropic/claude-sonnet-4-5")
print(resp.text)
```

Every call identifies the backend with a `provider/model-name` string — `anthropic/claude-sonnet-4-5`, `openai/gpt-4o-mini`, `googleai/gemini-1.5-pro`, `ollama/llama3.2`. Set a default to skip the prefix on every call:

```python
ai.config(default_model="anthropic/claude-sonnet-4-5")
resp = ai.generate("...")  # uses the default
```

## Provider credentials

| Provider | Env var | Notes |
|----------|---------|-------|
| Anthropic | `ANTHROPIC_API_KEY` | claude-3-5-sonnet, claude-opus, etc. |
| OpenAI | `OPENAI_API_KEY` | gpt-4o, gpt-4o-mini, etc. |
| Google AI | `GOOGLE_API_KEY` | gemini-1.5-pro, gemini-flash, etc. |
| Ollama | *(none)* | Local; default endpoint `http://localhost:11434`. Override via `ai.config(base_urls={...})` |

`ai.config()` configures credentials in-script, useful when a script manages its own keys:

```python
ai.config(api_keys = {"openai": env("MY_OPENAI_KEY")})
```

## Install the AI edition

```bash
# From source — produces ./bin/kiteai
make build-ai

# Or via the edition manager (downloads from GitHub Releases)
kitecmd edition use ai
```

If you already have the all-in-one `kite` binary, the AI modules are bundled in — no separate install needed. See [Editions](../fundamentals/editions.md) for the full edition model.

## Compose with base modules

All 27 base modules (`os`, `fs`, `http`, `ssh`, `json`, `yaml`, `concur`, …) remain available in the AI edition. Agents typically combine `ai.chat()` with these for tool implementations — see the [agents guide](guides/agents.md) and [Embedding](../fundamentals/embedding.md#calling-starlark-functions-from-go) for driving agents from Go with Starlark tool bodies.
