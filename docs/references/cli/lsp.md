---
title: "lsp"
description: "Run the language server for Starkite scripts"
weight: 12
keywords: [lsp, language server, editor, vscode, neovim, helix, completion, diagnostics]
---

# kite lsp

Run a Language Server Protocol server for Starkite scripts over standard input
and output. Editors start this command themselves; you rarely run it by hand.

The server is [starlsp](https://github.com/M31-Labs/starlsp), a Starlark
language server, configured with a Starkite host. starlsp knows the Starlark
specification; Starkite supplies its module registry, its API documentation, its
`load()` rules, and its own conventions. Every editor feature comes from the
shared server, so Starkite gains anything starlsp gains.

```
kite lsp
kite lsp --probe
```

## Build requirement

The language server is behind a build tag. A default `kite` does not contain it,
so that the runtime stays small on init containers, edge nodes, and CI runners.

```bash
# Default build — no language server, no size change.
go build -o ./bin/kite ./kite

# With the language server.
go build -tags lsp -o ./bin/kite ./kite

# With the language server and only the Starlark grammar embedded.
go build -tags 'lsp,grammar_subset,grammar_subset_starlark' -o ./bin/kite ./kite
```

The third form is the one to use. The parser library ships more than 200
grammars, and the subset tag keeps all but Starlark out of the binary.

| Build | Size | Added |
|---|---|---|
| Default | 131.6 MB | — |
| `lsp`, every grammar | 158.8 MB | 27.2 MB |
| `lsp`, Starlark only | 140.3 MB | 8.7 MB |

A binary built without the tag reports `unknown command "lsp"`.

The Starlark-only build needs `gotreesitter` v0.50.1 or later, which `libkite`
already requires.

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
confirm a build, or to find out why a name does not complete.

```console
$ kite lsp --probe
starlsp 0.1.0

host           starlark+starkite
parser         starlark (gotreesitter, pure Go)
globals        119
hooks          members, types, loads, lint, docs

namespaces  30  ai base64 concur csv fmt fs gzip hash http inventory io json k8s
                log mcp os regexp retry runtime sql ssh strings table template
                test time uuid vars yaml zip
types       10  bool bytes dict float int list range set str tuple
...

members
  fs                2
  fs.path          84
  http              8
  http.url         10
  string           35
  ...
```

## Editor setup

### VS Code

Install the extension in `editors/vscode`, then point it at your binary:

```json
{
  "starkite.serverPath": "/usr/local/bin/kite"
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

**The server does not start.** Confirm the binary carries the tag:

```console
$ kite lsp --probe
Error: unknown command "lsp" for "kite"   # built without -tags lsp
```

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
