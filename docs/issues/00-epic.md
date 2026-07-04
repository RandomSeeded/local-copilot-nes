# Epic: local-model NES for Neovim (sidekick client + extracted cursortab engine)

Deliver Cursor-style Next-Edit-Suggestions in Neovim driven by a **local model**,
by pairing sidekick.nvim's NES *client* (unmodified) with a small LSP server that
wraps cursortab's extracted model+diff *engine*.

Architecture: see [`README.md`](../../README.md) and
[`docs/adr/0001`](../adr/0001-reuse-sidekick-client-with-local-nes-lsp-server.md).
Extraction map: [`docs/engine-extraction-boundary.md`](../engine-extraction-boundary.md).

## Child issues
- #01 Extract cursortab's model+diff engine into a standalone Go library
- #02 Implement the `local-copilot-nes` LSP server (`copilotInlineEdit`)
- #03 Neovim integration (sidekick redirect shim + Gap 2 relaxation)
- #04 Multi-edit chaining: validate & tune on real code
- #05 End-to-end latency & cancellation validation

## Resolved unknowns (spikes)
- **Redirect** — sidekick selects its NES server by name-substring `copilot` only (`sidekick/config.lua:255-268`); a `*copilot*`-named server + a 3-line `get_client` shim wins it. ✅
- **Cancellation/latency** — sidekick sends LSP `$/cancelRequest` (`nes/init.lua:195`); server aborts the model call via `context`. Structurally avoids the per-keystroke cancel-cascade. ✅
- **Chaining (D1)** — `sweep-next-edit-1.5B` propagates an edit to the next similar site and advances on re-request (synthetic). Chaining rides sidekick's `SidekickNesDone` loop; no server-side stage-splitting for v1. ✅ (needs real-code validation → #04)

## The one thing none of this fixes
Suggestion **quality** is bounded by the local next-edit model, not Copilot. This
architecture reproduces NES *behavior*, not Copilot's *model*.
