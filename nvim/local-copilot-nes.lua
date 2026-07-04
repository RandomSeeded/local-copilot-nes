-- local-copilot-nes — route sidekick.nvim's Next-Edit-Suggestions to a local model.
--
-- INSTALL: copy this file to ~/.config/nvim/lua/plugins/local-copilot-nes.lua
-- and build the server:  cd ~/Projects/local-copilot-nes && go build -o bin/local-copilot-nes ./cmd/local-copilot-nes
-- Requires a local llama-server (or any OpenAI-compatible endpoint) serving a
-- next-edit model — see the project README.
--
-- What this does:
--   1. Registers our LSP server (named with "copilot" so sidekick's name-based
--      NES client selection is eligible to pick it).
--   2. Overrides sidekick's get_client so NES requests route to OUR server
--      rather than copilot.lua's "copilot" client (which also implements
--      copilotInlineEdit but returns empty on the Free tier). copilot.lua stays
--      for inline ghost text.
--   3. Turns sidekick NES back on.

local SERVER_NAME = "local-copilot-nes"
local BIN = vim.fn.expand("~/Projects/local-copilot-nes/bin/local-copilot-nes")

-- 1. Register + enable the LSP server (nvim 0.11+ API).
vim.lsp.config(SERVER_NAME, {
  cmd = { BIN },
  filetypes = { "python", "lua", "go", "javascript", "typescript", "javascriptreact", "typescriptreact", "cs" },
  root_markers = { ".git", "go.mod", "pyproject.toml", "package.json" },
})
vim.lsp.enable(SERVER_NAME)

-- 2. Route sidekick NES to our server (load-order-safe: patch once sidekick exists).
vim.api.nvim_create_autocmd("User", {
  pattern = "VeryLazy",
  callback = function()
    local ok, C = pcall(require, "sidekick.config")
    if not ok then
      return
    end
    C.get_client = function(buf)
      return vim.lsp.get_clients({ bufnr = buf or 0, name = SERVER_NAME })[1]
    end
  end,
})

-- 3. Re-enable sidekick NES (it was disabled while we were on Copilot NES).
return {
  {
    "folke/sidekick.nvim",
    opts = { nes = { enabled = true } },
  },
}
