# local-copilot-nes

A **local-model backend for Cursor-style Next-Edit-Suggestions (NES) in Neovim.**

Copilot NES (the "do you also want to make these 5 modifications" flow) requires
a paid Copilot plan and is officially unsupported on Neovim. This project stands
up a small LSP server that implements `textDocument/copilotInlineEdit` — the
exact wire method [sidekick.nvim](https://github.com/folke/sidekick.nvim)'s NES
client speaks — backed by a **local model** (`llama-server` + a next-edit model
such as `sweep-next-edit-1.5B`) and the extracted diff/provider engine from
[cursortab.nvim](https://github.com/cursortab/cursortab.nvim).

Division of labor:

- **sidekick.nvim (unmodified)** provides the client UX: version-gated
  persistence (suggestions survive cursor-move / save, die only on content
  change), jump-to-edit anywhere in the buffer, multi-edit chaining, and diff
  rendering.
- **local-copilot-nes (this repo)** provides the local model behind it.

## Why this shape

See [`docs/adr/0001-*`](docs/adr/0001-reuse-sidekick-client-with-local-nes-lsp-server.md).
In short: the NES *UX* is client policy sidekick already implements correctly;
the local *model + diff* is cursortab's engine. Reuse both — don't fork either.

## Architecture

```
Neovim
  sidekick.nvim (NES client)
    │  LSP: didFocus, copilotInlineEdit, $/cancelRequest
    ▼
local-copilot-nes  (this repo — the only new component)
    │  extracted cursortab engine: snapshot → provider → model → diff
    ▼
llama-server (local next-edit model, OpenAI-compatible)
```

Server selection: sidekick picks its NES server purely by **name-substring
`copilot`**, so this server registers under a `*copilot*` name and a ~3-line
`require("sidekick.config").get_client` shim disambiguates it from copilot.lua's
inline-completion client (which is literally named `copilot`).

## Docs

- [`docs/adr/0001-reuse-sidekick-client-with-local-nes-lsp-server.md`](docs/adr/0001-reuse-sidekick-client-with-local-nes-lsp-server.md) — the architecture decision + rejected alternatives.
- [`docs/engine-extraction-boundary.md`](docs/engine-extraction-boundary.md) — what to KEEP / ADAPT / DISCARD / ADD from cursortab.

## Status

Design + spikes complete. Key unknowns resolved:

- **Redirect** — sidekick will talk to a non-Copilot server (name-based selection). ✅
- **Cancellation/latency** — sidekick sends LSP `$/cancelRequest`; server aborts the model call via `context`. Structurally immune to per-keystroke cancel-cascade. ✅
- **Multi-edit chaining (D1)** — `sweep-next-edit-1.5B` propagates an edit to the next similar site and advances on re-request (synthetic test). Chaining can ride sidekick's re-request loop; no server-side stage-splitting needed for v1. ✅ (needs real-code validation)

Implementation is tracked in issues.
