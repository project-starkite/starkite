# Editor Configuration Guide for Starkite

Starkite provides full Language Server Protocol (LSP) intelligence via the standalone `kitelsp` binary (or `kite lsp` embedded). This guide details configuration for modern developer editors.

## 1. Prerequisites: Installing `kitelsp`

Install `kitelsp` into your system `$PATH`:

```bash
# Option A: Install from source
make build-kitelsp
sudo cp ./bin/kitelsp /usr/local/bin/

# Option B: Install via Homebrew
brew install project-starkite/tap/kitelsp
```

Verify installation:
```bash
kitelsp --probe
```

---

## 2. Visual Studio Code

1. Package and install the `.vsix` extension:
   ```bash
   cd editors/vscode
   npm install
   npx vsce package --no-dependencies
   code --install-extension starkite-0.11.4.vsix
   ```
2. The extension automatically discovers `kitelsp` on `$PATH`.
3. To customize binary path or enable JSON-RPC tracing, edit `.vscode/settings.json`:
   ```json
   {
     "starkite.serverPath": "kitelsp",
     "starkite.trace.server": "off"
   }
   ```

---

## 3. Neovim (`nvim-lspconfig`)

Add the following configuration to your `init.lua`:

```lua
local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- 1. Register Starkite filetype for .star files
vim.filetype.add({
  extension = {
    star = 'starkite',
  },
})

-- 2. Define kitelsp server configuration
if not configs.kitelsp then
  configs.kitelsp = {
    default_config = {
      cmd = { 'kitelsp' },
      filetypes = { 'starkite', 'starlark' },
      root_dir = lspconfig.util.root_pattern('mod.yaml', 'mod.lock', '.git'),
      settings = {},
    },
  }
end

-- 3. Attach kitelsp to buffers
lspconfig.kitelsp.setup({
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
language-servers = ["kitelsp"]

[language-server.kitelsp]
command = "kitelsp"
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
      "language_servers": ["kitelsp"],
      "format_on_save": "off"
    }
  },
  "lsp": {
    "kitelsp": {
      "binary": {
        "path": "kitelsp",
        "arguments": []
      }
    }
  }
}
```
