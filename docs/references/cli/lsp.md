---
title: "lsp"
description: "Run the embedded Language Server Protocol server for Starkite scripts"
weight: 12
keywords: [kite lsp, lsp, language server, editor, vscode, neovim, helix, zed, completion, diagnostics]
---

# kite lsp

Run the embedded Language Server Protocol server for Starkite scripts over standard input
and output. Editors start this command themselves; you rarely run it by hand.

The server is [starlsp](https://github.com/M31-Labs/starlsp), a Starlark
language server, configured with a Starkite host. starlsp knows the Starlark
specification; Starkite supplies its module registry, its API documentation, its
`load()` rules, and its own conventions. Every editor feature comes from the
shared server, so Starkite gains anything starlsp gains.

```bash
kite lsp
kite lsp --probe
```

## Out-of-the-Box Availability

`kite lsp` is a standard, built-in subcommand of every official `kite` release.

The binary embeds the Tree-sitter engine with a specialized Starlark grammar subset, keeping total binary size compact while providing full syntax tree intelligence.

| Component | Footprint | Purpose |
|---|---|---|
| Core Runtime & Execution Engine | ~22.5 MiB | Sandboxed scripting, CLI, stdlib |
| Kubernetes & Container SDKs | ~38.9 MiB | Cloud-native operations |
| Language Server & Diagnostics (`starlsp`) | ~1.8 MiB | JSON-RPC protocol loop & diagnostic bridge |
| Starlark Tree-sitter Grammar Subset | ~3.2 MiB | Incremental AST parsing & symbol tokens |
| **Total Unified Binary (Stripped)** | **~50.2 MiB** | Complete runtime, CLI, and editor intelligence |

## What the server provides

| Capability | Source |
|---|---|
| Diagnostics | The runtime's own parser and resolver |
| Completion | This binary's module registry |
| Hover | The API reference tables |
| Signature help | The API reference tables |
| Go to definition | The runtime's resolver, and the module loader for `load()` |
| Find references | The resolver's bindings |
| Document highlight | The resolver's bindings |
| Rename | The resolver's bindings, refused for names this file does not bind |
| Document symbols | The syntax tree |
| Workspace symbols | Every `.star` file under the workspace root |
| Semantic tokens | The syntax tree and the module registry |
| Folding ranges | The syntax tree |
| Selection ranges | The syntax tree |
| Document links | `load()` targets resolved through `mod.yaml` and `mod.lock` |

Diagnostics come from `go.starlark.net`, which is the same code `kite run` and
`kite validate` use. An error the editor shows is therefore an error the runtime
reports, and the two cannot disagree.

Completion is read from the registry at start-up rather than from a stored list.
The edition you build decides what you get: `kitecloud` completes the `k8s`
module, `kitecmd` does not, and a module added to the registry appears in
completion with no further change.

## Checking the server without an editor

`--probe` prints what the running binary would offer and exits. Use it to
confirm a build, or to inspect available symbol surfaces.

```console
$ kite lsp --probe
starlsp 0.1.0

host           starlark+starkite
parser         starlark (gotreesitter, pure Go)
globals        127
hooks          members, types, loads, lint, docs

namespaces  33  args base64 concur containers csv fleet fmt fs gzip hash http
                inventory io json k8s log mcp os regexp retry runtime sandbox
                sql ssh strings table template test time uuid vars yaml zip
functions   74  abs all any assert assert_contains assert_equal assert_false
                assert_not_equal assert_true bash bool bytes chdir chr cmdexe cwd
                dict dir enumerate env exec exists exit fail float getattr glob
                groupid hasattr home hostname int len list max min ord path pid
                powershell ppid print printf println range read_bytes read_text
                repr reversed set setenv sh shell skip sorted sprintf str
                temp_dir try_exec tuple type userid username var_bool var_dict
                var_float var_int var_list var_names var_str which write_bytes
                write_text zsh
keywords    16  and break continue def elif else for if in lambda load not or
                pass return while
variables    4  False None True user

members
  args             14
  base64           6
  bytes            1
  concur           6
  containers       26
  csv              4
  dict             9
  fleet            8
  fmt              6
  fs               14
  gzip             6
  hash             6
  http             18
  inventory        12
  io               4
  json             8
  k8s              111
  list             7
  log              16
  mcp              4
  os               42
  regexp           12
  retry            4
  runtime          12
  sandbox          8
  set              12
  sql              4
  ssh              22
  string           35
  strings          16
  table            4
  template         8
  test             16
  time             26
  uuid             2
  vars             14
  yaml             12
  zip              2
```

## Editor setup

### VS Code

Install the extension in `editors/vscode`, then configure your binary path:

```json
{
  "starkite.serverPath": "kite"
}
```

To run the extension from a checkout:

```bash
cd editors/vscode
npm install
code --extensionDevelopmentPath="$PWD" .
```

### Neovim

Neovim 0.11 and later:

```lua
vim.filetype.add({ extension = { star = "starkite" } })

vim.lsp.config.starkite = {
  cmd = { "kite", "lsp" },
  filetypes = { "starkite" },
  root_markers = { "mod.yaml", "mod.lock", ".git" },
}
vim.lsp.enable("starkite")
```

Earlier versions, through `nvim-lspconfig`:

```lua
vim.filetype.add({ extension = { star = "starkite" } })

require("lspconfig").configs.starkite = {
  default_config = {
    cmd = { "kite", "lsp" },
    filetypes = { "starkite" },
    root_dir = require("lspconfig.util").root_pattern("mod.yaml", "mod.lock", ".git"),
  },
}
require("lspconfig").starkite.setup({})
```

### Helix

In `languages.toml`:

```toml
[language-server.kite]
command = "kite"
args = ["lsp"]

[[language]]
name = "starkite"
scope = "source.star"
file-types = ["star"]
comment-token = "#"
indent = { tab-width = 4, unit = "    " }
language-servers = ["kite"]
```

### Zed

In `settings.json`:

```json
{
  "lsp": {
    "kite": {
      "binary": { "path": "kite", "arguments": ["lsp"] }
    }
  }
}
```

## Troubleshooting

**Nothing completes after a dot.** The receiver is a name the server cannot
resolve to a module or an object. Run `kite lsp --probe` and check that the
owner appears in the list. Completion is deliberately silent rather than
offering every global name, because a wrong list is worse than no list.

**A method on an object does not complete.** The server learns object methods by
calling a factory once and reading the result. An object only reachable after
network or database input — an open SQL connection, a live SSH client — is not
probed, so its methods are not offered.

**Diagnostics differ from `kite run`.** They should not. Report it: the server
and the runtime call the same parser, so a difference is a defect.

**The server does not start.** Confirm that `kite` is present in your `$PATH` and executable, or verify functionality with `kite lsp --probe`.

## Starkite-specific diagnostics

Beyond what Starlark itself forbids, the server reports two runtime
conventions:

- A top-level `main()` call, which makes the runtime skip its automatic
  entry-point invocation. The message explains the log line before you meet it.
- A `try_` call whose `Result` is never inspected. A `try_` variant returns a
  `Result` rather than raising, which only helps if something reads `.ok`.
  Binding one and using it as though it were the value fails at run time with a
  confusing message about a `Result` having no such attribute.

## See also

- [validate](validate.md) — check syntax without executing
- [Language](../../fundamentals/language.md) — the `main()` entry point and the `try_` pattern
- [Modules](../../fundamentals/modules.md) — how `load()` resolves
