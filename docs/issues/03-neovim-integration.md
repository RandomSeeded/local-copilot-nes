# Neovim integration: sidekick redirect shim + Gap 2 relaxation

## Context
Point sidekick's NES client at `local-copilot-nes` and make far-from-cursor
edits expressible, without disturbing copilot.lua's inline ghost text.

## Tasks

### Register the server
```lua
vim.lsp.config("local-copilot-nes", {
  cmd = { "local-copilot-nes" },      -- or absolute path to the built binary
  filetypes = { "python", "lua", "go", ... },
  root_markers = { ".git" },
})
vim.lsp.enable("local-copilot-nes")
```
Confirm it attaches and syncs so `vim.lsp.util.buf_versions[buf]` is populated
(sidekick's persistence gate needs it).

### Disambiguate client selection (the collision seam)
copilot.lua registers a client literally named `copilot`
(`copilot.lua/lua/copilot/client/init.lua:168,223`) that *also* implements
`copilotInlineEdit` (returns empty on Free tier). sidekick's `get_client`
returns the first `copilot`-named client (`config.lua:268`), with no override
hook. Add a ~3-line shim in our nvim config:
```lua
local C = require("sidekick.config")
C.get_client = function(buf)
  return vim.lsp.get_clients({ bufnr = buf or 0, name = "local-copilot-nes" })[1]
end
```
Keeps copilot.lua for **inline** completion; routes **NES** to our server.
Re-enable sidekick NES (`opts.nes.enabled = true`) — currently disabled.

### Gap 2 — far-from-cursor edits
- Relax the anchor gate: providers pass `0.25` to `checkAnchorPosition`
  (`sweep.go:173,189`, `zeta.go:286,301`). Expose `maxAnchorRatio` as config;
  default it high/off — **but keep a light guard** (D1 showed the model can emit
  drift lines; see #04).
- Widen/parameterize the context window `TrimContentAroundCursor`
  (`provider/flow.go:99-107`) so distant edits are expressible at all.

## Depends on
#02 (running server). Gap 2 edits land in the extracted engine (#01).

## References
- `sidekick/lua/sidekick/config.lua:255-268`, ADR 0001 (collision seam).
