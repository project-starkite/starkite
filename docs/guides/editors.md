---
title: "Editor configuration"
description: "Configure VS Code, Neovim, Helix, Zed, and Sublime Text with the Starkite language server"
weight: 10
---

# Editor Configuration Guide for Starkite

Starkite provides full Language Server Protocol (LSP) intelligence directly through the built-in `kite lsp` subcommand. No secondary language server binary or external daemon is required.

---

## 1. Prerequisites: Installing `kite`

Ensure `kite` is installed and available in your system `$PATH`:

```bash
# Option A: Install via Homebrew
brew install project-starkite/tap/kite

# Option B: Install from source
make kite
sudo cp ./bin/kite /usr/local/bin/
```

Verify that the language server is operational and inspecting the runtime catalog:
```bash
kite lsp --probe
```

---

## 2. Visual Studio Code

### Option A: Install Downloaded Extension (.vsix)
The pre-packaged extension is attached directly to every GitHub Release:

1. Download `starkite-<version>.vsix` from the [Starkite Releases](https://github.com/project-starkite/starkite/releases) page.
2. Install via command line:
   ```bash
   code --install-extension starkite-0.11.8.vsix
   ```
   Or install inside VS Code via `Extensions` $\rightarrow$ `...` $\rightarrow$ `Install from VSIX...`.

### Option B: Build from Source
```bash
cd editors/vscode
npm install
npx @vscode/vsce package --skip-license
code --install-extension starkite-*.vsix
```

### Configuration Settings
The extension launches `kite lsp --stdio` automatically. You can customize the server binary path or logging in `.vscode/settings.json`:

```json
{
  "starkite.serverPath": "kite",
  "starkite.trace.server": "off"
}
```

---

## 3. Neovim (`nvim-lspconfig`)

Add the following configuration to your Neovim configuration (`init.lua`):

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- 1. Register Starkite filetype for .star files
vim.filetype.add({
  extension = {
    star = 'starkite',
  },
})

-- 2. Define Starkite language server configuration
if not configs.starkite then
  configs.starkite = {
    default_config = {
      cmd = { 'kite', 'lsp', '--stdio' },
      filetypes = { 'starkite', 'starlark' },
      root_dir = lspconfig.util.root_pattern('mod.yaml', 'mod.lock', '.git'),
      settings = {},
    },
  }
end

-- 3. Attach language server to buffers
lspconfig.starkite.setup({
  on_attach = function(client, bufnr)
    local opts = { noremap=true, silent=true, buffer=bufnr }
    vim.keymap.set('n', 'gd', vim.lsp.buf.definition, opts)
    vim.keymap.set('n', 'K', vim.lsp.buf.hover, opts)
    vim.keymap.set('n', '<leader>rn', vim.lsp.buf.rename, opts)
    vim.keymap.set('n', '<leader>ca', vim.lsp.buf.code_action, opts)
  end,
})
```

---

## 4. Helix

Configure Helix language server support in `~/.config/helix/languages.toml`:

```toml
[[language]]
name = "starlark"
scope = "source.starlark"
injection-regex = "starlark|star"
file-types = ["star", "starlark"]
roots = ["mod.yaml", "mod.lock", ".git"]
language-servers = ["starkite-lsp"]

[language-server.starkite-lsp]
command = "kite"
args = ["lsp", "--stdio"]
```

Verify server detection:
```bash
hx --health starlark
```

---

## 5. Zed

Configure Zed editor support in `~/.config/zed/settings.json` (or `.zed/settings.json`):

```json
{
  "languages": {
    "Starlark": {
      "language_servers": ["starkite-lsp"],
      "format_on_save": "off"
    }
  },
  "lsp": {
    "starkite-lsp": {
      "binary": {
        "path": "kite",
        "arguments": ["lsp", "--stdio"]
      }
    }
  }
}
```

---

## 6. Sublime Text (LSP Package)

Configure Sublime Text LSP package in `Preferences` $\rightarrow$ `Package Settings` $\rightarrow$ `LSP` $\rightarrow$ `Settings`:

```json
{
  "clients": {
    "starkite": {
      "enabled": true,
      "command": ["kite", "lsp", "--stdio"],
      "selector": "source.starlark | source.starkite",
      "schemes": ["file"]
    }
  }
}
```
