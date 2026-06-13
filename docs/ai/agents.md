---
title: "Build agents"
description: "Compose ai.chat() with libkite modules to build multi-turn agents"
weight: 7
edition: ai
---

!!! note "Needs the AI modules"
    The patterns in this guide use the `ai` module (and sometimes `mcp`), included in the default `kite` binary and in the lean `kiteai` edition. See [AI Edition](../fundamentals/editions.md).

An agent is a loop: a model decides, calls tools, sees the result, and decides again. Starkite-AI gives you the pieces to build that loop yourself rather than a packaged REPL or a blocking `agent.run()` facade. You compose [`ai.chat()`](../references/api/ai.md#aichatkwargs) and [`ai.run_until()`](../references/api/ai.md#airun_untilchat-initial-kwargs) with the libkite modules you already use for UI, I/O, and side effects — `io.prompt`, `fs`, `http`, `k8s`, `ssh`, and the rest. That keeps the `ai` module small and hands the script full control over the UX: the framework drives the model, and you drive everything around it.

Four patterns cover the cases you are likely to hit, and each has a runnable example in [`aikite/examples/agent/`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent). They are not mutually exclusive — a production agent usually combines several. Start with the one whose shape matches your task.

---

## Pattern 1 — Autonomous run-to-completion

Reach for this pattern when the agent gets a task, works it without asking you anything per turn, and stops on its own when some condition fires. It fits headless work: SRE diagnosis, batch processing, research runs. The primitive is `ai.run_until(chat, initial, stop_when=, max_steps=)`.

The loop sends `initial` as the first user message, then re-sends `"continue"` each turn until `stop_when(resp)` returns truthy or `max_steps` is reached. A common arrangement is to have the system prompt instruct the agent to say `"DONE"` when it has finished, and have `stop_when` watch for that word in the response text. The example below builds an SRE agent with two tools and lets it run until it reports done:

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

The agent calls `check_service` and `restart_service` as many times as it judges necessary, and `result.text` holds its final word once `stop_when` trips.

The `max_steps=15` cap is the safety rail that matters here, because tool-driven loops spend tokens on every turn. It bounds worst-case turns, so a `stop_when` predicate that never fires cannot run up an unbounded bill. When a run is long and the budget is tight, gate on cumulative token usage instead of turn count:

```python
def budget_exceeded(resp):
    return resp.usage.total > 100000

ai.run_until(chat, "Research X", stop_when=budget_exceeded, max_steps=50)
```

Now the loop stops the moment cumulative usage crosses the threshold, with `max_steps` left as a backstop. Full example: [`aikite/examples/agent/autonomous_fix.star`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent/autonomous_fix.star).

---

## Pattern 2 — User-in-the-loop REPL

When a human stays in the conversation — an interactive assistant, a CLI tool where questions arrive one at a time — you want the inverse of the autonomous loop: read a line, reply, read the next line. There is no built-in REPL helper, and that is deliberate, since the UX is yours to shape. You build the loop from a plain Starlark `for`, `io.prompt()` for input, and `chat.send()` for each turn:

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

The loop stays this short because `chat.send()` carries the conversation state for you. Each turn appends to `chat.history` automatically, so the next `send()` already sees the full prior context — you never thread messages by hand. Full example: [`aikite/examples/agent/interactive_assistant.star`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent/interactive_assistant.star).

---

## Pattern 3 — History management for long runs

That automatic history is convenient until a long run pushes the conversation past the model's context window. When it does, the fix is periodic summarization: every N turns, compress the full history into a short summary and rebuild the chat with that summary as its seed. Three primitives carry the work — [`chat.history`](../references/api/ai.md#chat-methods-and-attributes) gives you a read-only snapshot, `ai.generate()` runs a cheap model to do the compressing, and [`ai.chat(history=...)`](../references/api/ai.md#aichatkwargs) rebuilds the session from the seed:

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

Every tenth turn the agent collapses its accumulated history into three bullets and starts a fresh chat carrying only that seed, so the context stays bounded no matter how long the run goes. If you would rather keep the same `Chat` object and simply start over from turn 1, `chat.reset()` clears history in place without rebuilding — no seed, no summary. Full example: [`aikite/examples/agent/history_management.star`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent/history_management.star).

---

## Pattern 4 — MCP integration

The first three patterns assume you write the agent's tools as Starlark functions. Often the tools you want already exist in an external MCP server — filesystem access, database queries, SaaS APIs — and reimplementing them would be wasted effort. Instead you connect and wrap. [`mcp.connect()`](../references/api/mcp.md#mcpconnect) opens a session to the server, and a small Starlark `def` wraps each remote tool as a local callable you can hand to [`ai.chat(tools=...)`](../references/api/ai.md#aichatkwargs):

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

To the model, those wrapped functions are indistinguishable from any other Starlark tool — MCP tools compose with `ai.chat()` as ordinary callables, with no special plumbing on the agent side. The wrapper earns its keep as the place to add logging, coerce arguments, or validate input; skip it and `client.tools.<name>` is callable directly. Closing the client when you are done releases the subprocess or connection. Full example: [`aikite/examples/agent/mcp_integration.star`](https://github.com/project-starkite/starkite/tree/main/aikite/examples/agent/mcp_integration.star).

---

## Go embedders

If you are driving the LLM loop from Go rather than Starlark, these same patterns have a mirror image on the Go side, described in the [embedding guide](../references/embedding.md#calling-starlark-functions-from-go). There the Go host owns the LLM client and the tool schemas, and libkite executes the bodies of those tools through `Runtime.Call(ctx, name, args, kwargs)`. The story is identical — a model deciding, tools running, results flowing back — only the driver changes.

---

## Picking a pattern

With the four shapes in hand, match your scenario to a starting point:

| Scenario | Pattern |
|----------|---------|
| Agent runs headless until satisfied | [1 — run_until](#pattern-1-autonomous-run-to-completion) |
| User types questions, agent replies | [2 — REPL](#pattern-2-user-in-the-loop-repl) |
| Conversation grows longer than context window | [3 — history management](#pattern-3-history-management-for-long-runs) |
| Tools live in an existing MCP server | [4 — MCP integration](#pattern-4-mcp-integration) |
| Go code orchestrates, Starlark provides tool bodies | [Embedding guide — Calling from Go](../references/embedding.md#calling-starlark-functions-from-go) |

Treat the table as a starting point, not a partition. The patterns compose freely, and a real agent often runs an autonomous loop (Pattern 1), summarizes its history as it goes (Pattern 3), and reaches for MCP-hosted tools (Pattern 4) — all in a single script.
