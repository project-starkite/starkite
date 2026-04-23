---
title: "Building Agents"
description: "Compose ai.chat() with starbase modules to build multi-turn agents"
weight: 7
edition: ai
---

!!! note "AI edition only"
    The patterns in this guide use the `ai` module (and sometimes `mcp`), available only in the `kite-ai` edition. See [AI Edition](ai-edition.md).

Starkite-AI does **not** ship a packaged REPL or a blocking `agent.run()` facade. Instead, scripts build agents by composing [`ai.chat()`](../modules/ai.md#aichatkwargs) + [`ai.run_until()`](../modules/ai.md#airun_untilchat-initial-kwargs) with the existing starbase modules for UI, I/O, and side effects (`io.prompt`, `fs`, `http`, `k8s`, `ssh`, …). This keeps the `ai` module small and gives scripts full control over the UX.

This guide documents four patterns. Each comes with a complete runnable example in [`ai/examples/agent/`](https://github.com/project-starkite/starkite/tree/main/ai/examples/agent).

---

## Pattern 1 — Autonomous run-to-completion

**When to use:** the agent gets a task, calls tools as needed, and stops when some condition fires. No user interaction per turn. Fits SRE diagnosis, batch processing, research tasks.

**Primitive:** `ai.run_until(chat, initial, stop_when=, max_steps=)`.

The loop sends `initial` as the first user message, then re-sends `"continue"` each turn until `stop_when(resp)` returns truthy or `max_steps` is reached. System prompt typically instructs the agent to say `"DONE"` when finished; `stop_when` detects that in the response text.

```python
def check_service(name):
    """Ping a service's health endpoint."""
    resp = http.url("http://localhost:8080/health/" + name).get(timeout="5s")
    return {"service": name, "ok": resp.status_code == 200}

def restart_service(name):
    """Restart a service via the local-ops CLI."""
    result = local.exec("systemctl restart " + name)
    return {"restarted": name, "exit_code": result.exit_code}

chat = ai.chat(
    model  = "anthropic/claude-sonnet-4-5",
    system = "You are an SRE. Diagnose and fix service outages. Say DONE when finished.",
    tools  = [check_service, restart_service],
)

result = ai.run_until(chat,
    "The 'api' service is reportedly down. Diagnose and fix.",
    stop_when = lambda r: "DONE" in r.text,
    max_steps = 15,
)
print(result.text)
```

**Safety rails:** `max_steps=15` caps worst-case turns, so a misbehaving `stop_when` predicate can't cause unbounded spend. For longer runs with tight budgets, gate on `resp.usage.total` instead:

```python
def budget_exceeded(resp):
    return resp.usage.total > 100000

ai.run_until(chat, "Research X", stop_when=budget_exceeded, max_steps=50)
```

Full example: [`ai/examples/agent/autonomous_fix.star`](https://github.com/project-starkite/starkite/tree/main/ai/examples/agent/autonomous_fix.star).

---

## Pattern 2 — User-in-the-loop REPL

**When to use:** interactive assistants, CLI tools where the user asks questions turn by turn. The agent reads a line, replies, reads the next line, and so on.

**Primitive:** a plain Starlark `for` loop with `io.prompt()` for input and `chat.send()` for each turn. Ther's no built-in REPL helper — you compose one yourself with exactly the UX you want.

```python
def read_file(path):
    """Read a text file and return its contents."""
    return fs.read_text(path)

def list_dir(path):
    """List files in a directory."""
    return [e.name for e in fs.ls(path)]

chat = ai.chat(
    model  = "openai/gpt-4o-mini",
    system = "You are a helpful filesystem assistant. Use the tools to answer questions about local files.",
    tools  = [read_file, list_dir],
)

print("Filesystem assistant — type 'exit' to quit.")
for _ in range(1000):  # generous cap; user Ctrl-C to exit in practice
    user_msg = io.prompt("You: ")
    if user_msg == None or user_msg.lower() in ("exit", "quit"):
        break
    if user_msg == "":
        continue

    resp = chat.send(user_msg)
    printf("Agent: %s\n\n", resp.text)
```

The pattern is trivial because `chat.send()` does all the history management. Each turn automatically appends to `chat.history`; the next `send()` sees full context.

Full example: [`ai/examples/agent/interactive_assistant.star`](https://github.com/project-starkite/starkite/tree/main/ai/examples/agent/interactive_assistant.star).

---

## Pattern 3 — History management for long runs

**When to use:** long-running agents where the conversation will eventually exceed the model's context window. The fix is periodic summarization: every N turns, compress the full history into a short summary and rebuild the chat with that summary as a seed.

**Primitives:** [`chat.history`](../modules/ai.md#chat-methods-and-attributes) (read snapshot), `ai.generate()` (for the cheap summarizer model), and [`ai.chat(history=...)`](../modules/ai.md#aichatkwargs) (rebuild with a seed).

```python
MAX_TURNS_BEFORE_SUMMARIZE = 10

def build_chat(seed_history = None):
    return ai.chat(
        model   = "openai/gpt-4o-mini",
        system  = "You answer user questions about their codebase.",
        history = seed_history,
    )

def summarize(history):
    """Use a cheap model to compress prior turns into a single summary."""
    transcript = "\n".join([m["role"] + ": " + m.get("content", "") for m in history])
    resp = ai.generate(
        "Summarize this conversation in 3 bullet points, preserving key facts:\n\n" + transcript,
        model = "openai/gpt-4o-mini",
    )
    return resp.text

chat = build_chat()
turn = 0

for q in questions:
    resp = chat.send(q)
    turn += 1
    if turn % MAX_TURNS_BEFORE_SUMMARIZE == 0:
        summary = summarize(chat.history)
        chat = build_chat(seed_history = [
            {"role": "user",      "content": "Here is a summary of our prior conversation:"},
            {"role": "assistant", "content": summary},
        ])
```

Alternative: `chat.reset()` clears history in place without rebuilding — useful if you want to keep the same Chat object but start fresh from turn 1.

Full example: [`ai/examples/agent/history_management.star`](https://github.com/project-starkite/starkite/tree/main/ai/examples/agent/history_management.star).

---

## Pattern 4 — MCP integration

**When to use:** the agent needs tools that live in an external MCP server (filesystem access, database queries, SaaS APIs, etc.). Don't reimplement — connect and wrap.

**Primitives:** [`mcp.connect()`](../modules/mcp.md#mcpconnect) to open a session, then a small Starlark `def` that wraps each remote tool as a local callable for [`ai.chat(tools=...)`](../modules/ai.md#aichatkwargs).

```python
# 1. Connect to an MCP server (stdio subprocess or HTTP)
client = mcp.connect(["npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp"])

# 2. Wrap each remote tool. The wrapper gives you a spot to add logging,
#    argument coercion, or validation. client.tools.<name> is also callable
#    directly if you don't need that.
def read_file(path):
    """Read a file from the MCP-exposed filesystem."""
    return client.call("read_file", path=path).text

def list_directory(path):
    """List a directory's entries via the MCP server."""
    return client.call("list_directory", path=path).text

# 3. Run an agent that has access to those tools.
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

No special plumbing is required — MCP tools (Phase 2) compose with `ai.chat()` (Phase 1) as regular Starlark callables.

Full example: [`ai/examples/agent/mcp_integration.star`](https://github.com/project-starkite/starkite/tree/main/ai/examples/agent/mcp_integration.star).

---

## Go embedders

If you're driving the LLM loop from Go rather than Starlark, the mirror of these patterns lives in the [embedding guide](embedding.md#calling-starlark-functions-from-go). The Go host owns the LLM client and tool schemas; starbase executes the bodies of tools via `Runtime.Call(ctx, name, args, kwargs)`. Same underlying story — different driver.

---

## Picking a pattern

| Scenario | Pattern |
|----------|---------|
| Agent runs headless until satisfied | [1 — run_until](#pattern-1--autonomous-run-to-completion) |
| User types questions, agent replies | [2 — REPL](#pattern-2--user-in-the-loop-repl) |
| Conversation grows longer than context window | [3 — history management](#pattern-3--history-management-for-long-runs) |
| Tools live in an existing MCP server | [4 — MCP integration](#pattern-4--mcp-integration) |
| Go code orchestrates, Starlark provides tool bodies | [Embedding guide — Calling from Go](embedding.md#calling-starlark-functions-from-go) |

The patterns compose. A production agent often combines Pattern 1 (autonomous loop) with Pattern 3 (summarization) and Pattern 4 (MCP tools) in a single script.
