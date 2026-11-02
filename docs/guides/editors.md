# Editor Configuration Guide for Starkite

Starkite provides full Language Server Protocol (LSP) intelligence directly through the `kite lsp` subcommand. No secondary language server binary is required.

## 1. Prerequisites: Installing `kite`

Ensure `kite` is installed and available in your system `$PATH`:

```bash
# Option A: Install from source
make kite
sudo cp ./bin/kite /usr/local/bin/

# Option B: Install via Homebrew
brew install project-starkite/tap/kite
```

Verify that the language server is operational:
```bash
kite lsp --probe
```

---

## 2. Visual Studio Code

1. Package and install the `.vsix` extension:
   ```bash
   cd editors/vscode
   npm install
   npx vsce package --no-dependencies
   code --install-extension starkite-0.11.5.vsix
   ```
2. The extension executes `kite lsp` automatically over stdio.
3. To customize binary path or enable JSON-RPC tracing, edit `.vscode/settings.json`:
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
      cmd = { 'kite', 'lsp' },
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
args = ["lsp"]
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
        "arguments": ["lsp"]
      }
    }
  }
}
```
