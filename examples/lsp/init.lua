-- init.lua - Neovim configuration for Starkite Language Server (kite lsp)
--
-- Prerequisites:
--   1. Neovim 0.8+ with nvim-lspconfig installed
--   2. 'kite' binary available in system $PATH (run 'kite lsp --probe' to verify)

local lspconfig = require('lspconfig')
local configs = require('lspconfig.configs')

-- 1. Register Starkite filetype association for .star files
vim.filetype.add({
  extension = {
    star = 'starkite',
    starlark = 'starkite',
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

-- 3. Configure keybindings and attach language server to buffers
lspconfig.starkite.setup({
  on_attach = function(client, bufnr)
    local opts = { noremap = true, silent = true, buffer = bufnr }

    -- Navigation and definition lookup
    vim.keymap.set('n', 'gd', vim.lsp.buf.definition, opts)

    -- Hover documentation lookup
    vim.keymap.set('n', 'K', vim.lsp.buf.hover, opts)

    -- Symbol renaming
    vim.keymap.set('n', '<leader>rn', vim.lsp.buf.rename, opts)

    -- Diagnostic navigation
    vim.keymap.set('n', '[d', vim.diagnostic.goto_prev, opts)
    vim.keymap.set('n', ']d', vim.diagnostic.goto_next, opts)
  end,
})
