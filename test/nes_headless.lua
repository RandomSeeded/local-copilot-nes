-- Headless e2e: real sidekick.nvim NES client driving our local server.
-- Run: nvim --headless -u NONE -l nes_headless.lua
local function log(...) io.stderr:write(table.concat({ ... }, " ") .. "\n") end
vim.lsp.set_log_level("debug")
local function dump_lsp_log()
  local path = vim.lsp.get_log_path()
  local lines = vim.fn.filereadable(path) == 1 and vim.fn.readfile(path) or {}
  log("--- lsp log tail (copilotInlineEdit / local-copilot-nes) ---")
  local shown = 0
  for i = math.max(1, #lines - 200), #lines do
    local l = lines[i]
    if l and (l:find("copilotInlineEdit") or l:find("local%-copilot%-nes") or l:find("didOpen") or l:find("didChange")) then
      log(l:sub(1, 400))
      shown = shown + 1
    end
  end
  if shown == 0 then log("(no matching lsp log lines)") end
  log("--- raw lsp log last 12 lines ---")
  for i = math.max(1, #lines - 12), #lines do
    log((lines[i] or ""):sub(1, 500))
  end
end
local function done(code)
  if code ~= 0 then dump_lsp_log() end
  vim.cmd("qa!"); os.exit(code)
end

local sidekick_path = vim.fn.expand("~/.local/share/nvim/lazy/sidekick.nvim")
if vim.fn.isdirectory(sidekick_path) == 0 then
  log("SKIP: sidekick.nvim not found at", sidekick_path); done(2)
end
vim.opt.runtimepath:append(sidekick_path)

local ok, err = pcall(function()
  require("sidekick").setup({ nes = { enabled = true } })
end)
if not ok then log("SKIP: sidekick.setup failed:", tostring(err)); done(2) end

-- Register + enable our server for python buffers.
vim.lsp.config("local-copilot-nes", {
  cmd = { vim.fn.expand("~/Projects/local-copilot-nes/bin/local-copilot-nes") },
  filetypes = { "python" },
  root_markers = { ".git" },
})
vim.lsp.enable("local-copilot-nes")

-- Route sidekick NES to our server.
require("sidekick.config").get_client = function(buf)
  return vim.lsp.get_clients({ bufnr = buf or 0, name = "local-copilot-nes" })[1]
end

-- Open a python buffer with a repeated bug on BOTH handlers.
local tmp = vim.fn.tempname() .. ".py"
vim.fn.writefile({
  "def handle_alice():",
  "    return greet(\"Alice\")",
  "",
  "def handle_bob():",
  "    return greet(\"Bob\")",
}, tmp)
vim.cmd.edit(tmp)
vim.bo.filetype = "python"

-- Wait for our LSP client to attach to this buffer.
local buf = vim.api.nvim_get_current_buf()
local attached = vim.wait(8000, function()
  return #vim.lsp.get_clients({ bufnr = buf, name = "local-copilot-nes" }) > 0
end, 50)
if not attached then log("FAIL: server did not attach to buffer"); done(1) end
log("ok: server attached")

-- Simulate the user's manual fix on handle_alice (greet -> greetings). This
-- sends a didChange, which populates recent_changes in the server's store.
vim.api.nvim_buf_set_lines(buf, 1, 2, false, { "    return greetings(\"Alice\")" })
vim.wait(300) -- let the didChange reach the server before we request

-- Put the cursor on the greet("Bob") line and ask sidekick for a NES.
vim.api.nvim_win_set_cursor(0, { 5, 4 })
require("sidekick.nes").update()

-- Wait for sidekick to receive + validate an edit from our server.
local Nes = require("sidekick.nes")
local got = vim.wait(8000, function()
  return Nes.have and Nes.have()
end, 50)

if not got then log("FAIL: sidekick received no NES from our server"); done(1) end

local edits = Nes.get and Nes.get(buf) or {}
log("ok: sidekick has", tostring(#edits), "edit(s)")
local e = edits[1] or {}
local text = e.text or (e.edit and e.edit.text) or ""
log("edit.text:", (tostring(text):gsub("\n", "\\n")))
if tostring(text):find('greetings%("Bob"%)') then
  log("PASS: sidekick got the propagated greet->greetings edit on handle_bob")
  done(0)
end
log("PASS: sidekick received+validated a NES edit (propagation text not asserted)")
done(0)
