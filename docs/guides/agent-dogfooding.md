# Autonomous AI Agent Dogfooding Guide: "Code as Action"

Starkite is engineered as the primary automation engine for autonomous AI coding agents (Antigravity, Robo, Claude Code, Cursor).

## The Core Problem with Micro-Tool Chattering

Traditional Model Context Protocol (MCP) tool servers expose granular operating system primitives as independent JSON-RPC endpoints (`read_file`, `write_file`, `exec_command`, `parse_json`). This architecture introduces significant drawbacks:

1. **Context Pollution**: Exposing 30+ tool schemas consumes 8,000–15,000 tokens of system prompt context on *every turn*.
2. **Chattering Latency**: A multi-step workflow requires 10–20 round trips across the LLM connection.
3. **Fragile State**: Intermediate string data must be held and parsed within the agent prompt window.

## The Starkite Paradigm: "Code as Action"

With Starkite, the agent operates via a single execution primitive:

```starlark
# 1 Turn: Read, filter, transform, and execute at native machine speed
data = json.decode(fs.read("config.json"))
filtered = [item for item in data["services"] if item["enabled"]]

for svc in filtered:
    res = os.try_exec(["systemctl", "restart", svc["name"]])
    if not res.ok:
        log.error("Failed to restart %s", svc["name"])
```

## The 4-Stage Closed-Loop Self-Correction Pattern

Agents authoring Starkite scripts must adhere to the following execution sequence:

1. **Consult Skill Specification**: Consult `.agent/skills/starkite/SKILL.md` (or `starkite-cloud`, `starkite-k8s`).
2. **Draft Script**: Write the script using idiomatic Starkite patterns (automatic `main()`, `try_` Result checking, auto-loaded modules).
3. **Pre-flight Gate**: Run `kite validate --lint --json <script.star>`.
4. **Self-Heal**: If diagnostics (`L001`–`L005`) are returned, parse the exact line and column numbers, apply deterministic fixes, and re-validate before running.
5. **Hermetic Run**: Execute `kite run <script.star>` inside the kernel sandbox.

## Diagnostic Error Remediation Reference

| Code | Finding | Autonomous Remediation Strategy |
|---|---|---|
| `L001` | Top-level `main()` call | Remove the explicit `main()` call statement at the bottom of the script. |
| `L002` | Discarded or unchecked `try_` Result | Insert `if res.ok:` guard before accessing `res.value`, or call plain variant to raise. |
| `L003` | Deprecated member (e.g. `os.system`) | Replace with modern alternative: `os.exec(["bin", "arg"])` or `os.sh("cmd")`. |
| `L004` | Sandbox policy warning | Verify script sandbox arguments or supply `--sandbox-exec-allow` / `--sandbox-path-write`. |
| `L005` | Bare `.star` load path | Add `./` prefix to local import: `load("./helper.star", "greet")`. |
