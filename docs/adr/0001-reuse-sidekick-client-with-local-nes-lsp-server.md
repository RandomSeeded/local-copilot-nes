# 1. Reuse sidekick.nvim as the NES client; put a local model behind a custom `copilotInlineEdit` LSP server

Status: Accepted

## Context

We want Cursor-style Next-Edit-Suggestions on Neovim — suggestions that
**persist** across cursor movement and saves, let you **jump** to an edit
anywhere in the file, **chain** through several related edits, and show a
**rich diff** — driven by a **local model** (no paid plan).

- Copilot NES delivers that UX but requires a paid Copilot plan and is
  officially unsupported on Neovim (returns empty edits on the Free tier).
- cursortab.nvim runs a local model but **dismisses on any cursor move**
  (`engine/events.go` `cursor_moved → reject`) and **rejects far-from-cursor
  edits** (`provider/processors.go`, `maxRatio 0.25`).
- sidekick.nvim already implements the full NES *client* UX — version-gated
  persistence, jump-to-hunk, `SidekickNesDone` chaining, and diff rendering —
  and selects its LSP server **purely by name-substring "copilot"**
  (`config.lua:255-268`), with no capability or binary check.

## Decision

Build a standalone Go LSP server, `local-copilot-nes`, that implements
`textDocument/copilotInlineEdit` backed by cursortab's **extracted model + diff
engine** and the local `llama-server`. Point sidekick's NES client at it with a
~3-line `require("sidekick.config").get_client` shim in our own nvim config.
Keep **sidekick unmodified**; keep **copilot.lua** for inline ghost text.

## Rationale

The NES UX we want is *client policy sidekick already implements correctly*; the
local model + diff is *cursortab's engine*. Reusing both — rather than forking
either — is the least new code and avoids inverting cursortab's dismiss-on-move
state machine. The wire contract is tiny and fully characterized; the redirect
is free and name-based.

## Rejected alternatives

- **Fork cursortab (both halves).** Invert its Go state machine to version-gate
  persistence, relax the anchor gate, and neuter the Lua teardown. Rejected:
  most invasive (fights the daemon's central dismiss-on-move assumption),
  highest ongoing maintenance (deep fork of an actively-developed repo), and it
  rebuilds persistence/jump/chain/diff that sidekick already ships.
- **Clean-slate LSP shim (no cursortab).** Write the model call, context
  windowing, and diff→edit from scratch behind sidekick. Rejected: reimplements
  cursortab's tested provider + diff engine for no benefit; extraction reuses it.
- **Stay on Copilot NES.** Rejected: requires a paid plan and is officially
  unsupported on Neovim.

## Consequences

- Suggestion **quality is bounded by the local model** (Sweep/Zeta
  window-rewrite), not Copilot. This architecture matches NES *behavior*, not
  its model — the one axis a local setup cannot buy.
- Maintenance surface = a small server we own + a config shim; sidekick and
  copilot.lua stay stock.
- One integration seam: copilot.lua registers a client literally named
  `"copilot"` that also implements `copilotInlineEdit`, so both it and our
  server match sidekick's `is_copilot`. The `get_client` shim disambiguates
  (select our server by exact name), keeping copilot.lua for inline completion
  and routing NES to us.
