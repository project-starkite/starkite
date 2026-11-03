# Starkite Language Server Protocol (LSP) Examples

This directory provides runnable examples, editor configurations, and agent integration harnesses for the built-in Starkite language server (`kite lsp`).

---

## 1. Quick Verification: `kite lsp --probe`

Before configuring editors or agents, verify that the language server is operational and introspecting the runtime catalog:

```bash
kite lsp --probe
```

Example output:
```text
Starkite LSP diagnostic probe
  • Server status: ready (pure-Go Tree-sitter Starlark)
  • Catalog introspection: 31 modules, 290 functions, 14 types registered
  • Active dialect linters: L001, L002, L003, L004, L005
```

---

## 2. Editor Configuration Files

This directory includes ready-to-use configuration files for popular modern code editors:

| Editor | Configuration File | Description |
| :--- | :--- | :--- |
| **VS Code** | [`.vscode/settings.json`](.vscode/settings.json) | Configures language server binary and trace level |
| **Neovim** | [`init.lua`](init.lua) | Lua configuration using `nvim-lspconfig` |
| **Helix** | [`languages.toml`](languages.toml) | TOML configuration mapping `*.star` to `kite lsp` |
| **Zed** | [`languages.json`](languages.json) | JSON settings for the Zed editor LSP bridge |

To launch the server manually over standard I/O (as editors do):
```bash
kite lsp --stdio
```

---

## 3. Demonstration Script: `demo_lsp.star`

The companion file [`demo_lsp.star`](demo_lsp.star) contains annotated Starlark code designed to test your editor's LSP capabilities:

1. **Autocompletion (`textDocument/completion`)**:
   - Place cursor after `fs.` or `json.` to inspect module members.
   - Place cursor after a `try_*` result variable (e.g., `res.`) to see exported fields (`ok`, `value`, `error`, `code`).
2. **Hover Documentation (`textDocument/hover`)**:
   - Hover over `http.get`, `fs.path`, or `concur.map` to view signatures and docstrings.
3. **Diagnostics & Linting (`textDocument/publishDiagnostics`)**:
   - The script intentionally contains commented examples of convention violations (`L001`, `L002`) to illustrate real-time linter warnings.

---

## 4. AI Agent Integration: `agent_lsp_client.py`

Starkite's LSP server is not limited to human editors—it is designed to serve as a real-time intelligence provider for autonomous AI coding agents.

### Why Connect an AI Agent to `kite lsp`?
* **Zero Hallucinations**: The agent queries completions at cursor positions to obtain exact member names and parameter signatures.
* **Context Token Efficiency**: Rather than stuffing entire API manuals into the LLM system prompt, the agent queries `textDocument/hover` on demand.
* **In-Memory Self-Correction**: The agent opens a virtual document in memory (`textDocument/didOpen`); the LSP streams back syntax errors and linter warnings (`L001`–`L005`) in real time, allowing the agent to self-heal code before writing it to disk.

### Running the Agent Demonstration Client

The provided [`agent_lsp_client.py`](agent_lsp_client.py) is a standalone Python 3 script that launches `kite lsp --stdio` as a subprocess, performs the JSON-RPC 2.0 handshake, queries completions and hover docs, and processes diagnostics:

```bash
python3 agent_lsp_client.py
```
