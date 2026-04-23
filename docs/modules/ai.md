---
title: "ai"
description: "Multi-provider LLM client with chat, tools, and agent primitives"
weight: 28
edition: ai
---

!!! note "AI edition only"
    The `ai` module is only available in the `kite-ai` edition. Install it via `go install github.com/project-starkite/starkite/ai/cmd/kite-ai@latest` or from the release binaries. See [AI Edition](../guides/ai-edition.md) for setup.

The `ai` module wraps Firebase Genkit to provide one-shot generation, multi-turn chat, tool calling, and autonomous agent loops across Anthropic, OpenAI, Google AI, and Ollama.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `ai.config(default_model=, api_keys=, base_urls=, timeout=)` | `None` | Set module-wide defaults for provider credentials and endpoints |
| `ai.generate(prompt, model=, system=, tools=, stream=, schema=, …)` | `Response` or `StreamValue` | Generate a one-shot completion |
| `ai.chat(model=, system=, tools=, history=, …)` | `Chat` | Create a multi-turn chat session |
| `ai.tool(fn, description=, params=)` | `Tool` | Wrap a Starlark callable as an LLM tool |
| `ai.run_until(chat, initial, stop_when=, max_steps=, follow_up=)` | `Response` | Run a chat to completion driven by a stop predicate |

## Model Strings

Every model is identified by a `provider/model-name` string. The provider prefix selects the backend:

| Prefix | Example | Env var for API key |
|--------|---------|---------------------|
| `anthropic/` | `anthropic/claude-sonnet-4-5` | `ANTHROPIC_API_KEY` |
| `openai/` | `openai/gpt-4o-mini` | `OPENAI_API_KEY` |
| `googleai/` | `googleai/gemini-1.5-pro` | `GOOGLE_API_KEY` |
| `ollama/` | `ollama/llama3.2` | *(none — local)* |

The Anthropic, OpenAI, and Google AI plugins read the corresponding env var at first use; set it before invoking `ai.generate()` / `ai.chat()`. For Ollama, the default endpoint is `http://localhost:11434`; override per-call or via `ai.config(base_urls=...)`.

## `ai.config()`

Sets module-wide defaults. Calling `ai.config()` again replaces previously set values.

```python
ai.config(
    default_model = "anthropic/claude-sonnet-4-5",
    api_keys      = {"openai": "sk-..."},
    base_urls     = {"ollama": "http://remote-ollama:11434"},
    timeout       = "60s",
)
```

| Kwarg | Type | Description |
|-------|------|-------------|
| `default_model` | string | Used when `ai.generate(...)` or `ai.chat(...)` omits `model=` |
| `api_keys` | dict | Per-provider override of env-var keys. Keys are lowercase provider names (`openai`, `anthropic`, `googleai`) |
| `base_urls` | dict | Per-provider override of the endpoint URL (primarily for Ollama or OpenAI-compatible proxies) |
| `timeout` | duration string | Global request timeout (e.g., `"30s"`, `"2m"`) |

## `ai.generate(prompt, **kwargs)`

One-shot completion. The `prompt` positional argument is the user message; all other parameters are keyword-only.

```python
resp = ai.generate("Summarize this changelog in 3 bullets", model="anthropic/claude-sonnet-4-5")
print(resp.text)
```

| Kwarg | Type | Description |
|-------|------|-------------|
| `model` | string | `provider/model-name`. Required unless `ai.config(default_model=...)` is set |
| `system` | string | System prompt |
| `temperature` | float | Sampling temperature |
| `max_tokens` | int | Cap on output tokens |
| `top_p` | float | Nucleus sampling |
| `top_k` | int | Top-K sampling (provider-dependent) |
| `stop` | list[string] | Stop sequences |
| `api_key` | string | Per-call override |
| `base_url` | string | Per-call endpoint override |
| `stream` | bool | Returns a `StreamValue` (iterable of `StreamChunk`) when True |
| `schema` | dict | JSON Schema; when set, the response's `.data` is parsed structured output |
| `tools` | list | Tool callables (plain functions or `ai.tool(fn)` wrappers) |
| `max_iterations` | int | Max tool-call rounds before halting (default 10) |
| `on_tool_error` | `"feedback"` or `"halt"` | Behavior when a tool raises (default `"feedback"` — send the error back to the model) |

### `Response` (returned when `stream=False`)

| Attribute | Type | Description |
|-----------|------|-------------|
| `.text` | string | Assistant's text response |
| `.model` | string | Model that produced the response |
| `.usage.input` | int | Input tokens consumed |
| `.usage.output` | int | Output tokens generated |
| `.usage.total` | int | `.input + .output` |
| `.data` | any or None | Parsed structured output (only set when `schema=` was provided) |

### `StreamValue` (returned when `stream=True`)

Iterable of `StreamChunk`:

```python
for chunk in ai.generate("write a haiku", model="...", stream=True):
    print(chunk.text, end="")
```

### `StreamChunk`

| Attribute | Type | Description |
|-----------|------|-------------|
| `.text` | string | The text delta for this chunk. Concatenating `.text` across all chunks reproduces the final response text |

A `StreamChunk` is truthy when `.text` is non-empty and converts to its text via `str()`.

Streaming and `schema=` are mutually exclusive in this version. Streaming with `tools=` is also not yet supported.

## `ai.chat(**kwargs)`

Create a stateful multi-turn conversation. Returns a `Chat` object.

```python
chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You are a concise assistant.",
    tools  = [search_docs, run_query],
)

resp = chat.send("Find all production deployments older than 30 days")
print(resp.text)

resp = chat.send("Now delete the three oldest.")
```

| Kwarg | Type | Description |
|-------|------|-------------|
| `model` | string | `provider/model-name`. Required unless `ai.config(default_model=...)` is set |
| `system` | string | System prompt (applied to every turn) |
| `tools` | list | Tools available for every `.send()` |
| `history` | list[dict] | Seed the chat with prior messages (same dict shape `chat.history` returns). Enables resume and forking |
| `temperature`, `max_tokens`, `top_p`, `top_k`, `stop`, `api_key`, `base_url`, `max_iterations`, `on_tool_error` | — | Same meaning as on `ai.generate()`; set as per-chat defaults |

### `Chat` methods and attributes

| Member | Description |
|--------|-------------|
| `.send(msg, stream=, schema=, tools=, …)` | Advance the conversation. Returns a `Response`. Per-call kwargs override chat defaults |
| `.history` | Read-only snapshot list of message dicts. Mutating the list does not change chat state |
| `.reset()` | Clear history. Defaults (model, system, tools) are preserved |

### History dict shape

Each entry in `chat.history` is a dict with these keys:

| Key | Present on | Value |
|-----|------------|-------|
| `role` | always | `"user"`, `"assistant"`, or `"tool"` |
| `content` | always (may be empty on assistant tool-request turns) | string |
| `tool_name` | assistant tool-request and tool response | string |
| `tool_input` | assistant tool-request | arbitrary (JSON-convertible) |
| `tool_output` | tool response | arbitrary |
| `tool_error` | tool response when the Starlark tool raised | string |

Round-trip example:

```python
old_chat = ai.chat(model="...", system="...")
old_chat.send("Hello")
# later...
new_chat = ai.chat(model="...", system="...", history=old_chat.history)
```

## `ai.tool(fn, description=, params=)`

Wrap a Starlark callable as an LLM tool. In most cases you don't need to call `ai.tool` explicitly — passing a plain function to `ai.chat(tools=[...])` or `ai.generate(tools=[...])` auto-wraps it with inferred metadata.

```python
def check_url(url):
    """Check whether a URL responds with 2xx."""
    r = http.url(url).get(timeout="5s")
    return {"status": r.status_code, "ok": r.status_code < 400}

# Either pass directly (auto-inferred):
chat = ai.chat(model="...", tools=[check_url])

# Or wrap explicitly for overrides:
tool = ai.tool(check_url,
    description = "Returns the HTTP status of a URL.",
    params = {
        "type": "object",
        "properties": {"url": {"type": "string"}},
        "required": ["url"],
    },
)
chat = ai.chat(model="...", tools=[tool])
```

### Inference rules (when kwargs are omitted)

| Piece | Source |
|-------|--------|
| `name` | Function name (not overridable) |
| `description` | Docstring (first line); empty if no doc |
| `params` | Inferred from the function signature: parameter names become required properties; types guessed from default values (`""` → string, `0` → integer, `True` → boolean, `[]` → array, `{}` → object) |

Non-Starlark callables (e.g., builtins) have no introspectable signature — you must supply both `description=` and `params=` explicitly.

### `Tool` attributes

| Attribute | Description |
|-----------|-------------|
| `.name` | The tool's name (from `fn.__name__`) |
| `.description` | The description string |

## `ai.run_until(chat, initial, **kwargs)`

Drive a chat to completion with a stop predicate. Sends `initial` as the first user message, then re-sends `follow_up` (default `"continue"`) each turn until `stop_when(resp)` returns truthy or `max_steps` is reached. Returns the final `Response`.

```python
chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You are an SRE. Say DONE when finished.",
    tools  = [check_service, restart_service],
)

resp = ai.run_until(chat,
    "The 'api' service is reportedly down. Diagnose and fix.",
    stop_when = lambda r: "DONE" in r.text,
    max_steps = 15,
)
print(resp.text)
```

| Kwarg | Type | Description |
|-------|------|-------------|
| `stop_when` | callable | `lambda resp: bool`. When truthy, return `resp` immediately. If omitted, run until `max_steps` |
| `max_steps` | int | Max turns (default 10). Safety cap; prevents runaway loops from unbounded spend |
| `follow_up` | string | User message sent on every turn after the first (default `"continue"`) |

See the [agents guide](../guides/agents.md) for patterns built around `ai.run_until`.

## Examples

### Structured output

```python
schema = {
    "type": "object",
    "properties": {
        "title":   {"type": "string"},
        "bullets": {"type": "array", "items": {"type": "string"}},
    },
    "required": ["title", "bullets"],
}

resp = ai.generate(
    "Summarize this RFC: ...",
    model  = "openai/gpt-4o-mini",
    schema = schema,
)
print(resp.data["title"])
for b in resp.data["bullets"]:
    printf("- %s\n", b)
```

### Streaming

```python
for chunk in ai.generate("write a haiku about kubernetes",
                         model="ollama/llama3.2",
                         stream=True):
    print(chunk.text, end="")
```

### Chat with tools

```python
def list_pods(namespace):
    """List pods in a Kubernetes namespace."""
    return [p.name for p in k8s.list("pods", namespace=namespace)]

chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You help operators investigate clusters.",
    tools  = [list_pods],
)

resp = chat.send("Which pods are running in 'default'?")
print(resp.text)
```

### Resuming a chat from history

```python
saved = json.decode(fs.read_text("chat.json"))
chat  = ai.chat(model="anthropic/claude-sonnet-4-5", history=saved)
resp  = chat.send("Continue where we left off.")
fs.write_text("chat.json", json.encode(chat.history))
```
