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

## Build & run

```sh
go build -o bin/local-copilot-nes ./cmd/local-copilot-nes
```

Then copy `nvim/local-copilot-nes.lua` to `~/.config/nvim/lua/plugins/`, ensure a
local model is serving (e.g. `llama-server -hf sweepai/sweep-next-edit-1.5B --port 8000`),
and restart nvim. Flags: `-url`, `-model`, `-max-tokens`, `-context-size`,
`-anchor-max-ratio` (Gap 2).

## Tests

- `go test -race ./...` — unit + integration (the engine/model tests skip if no llama-server on :8000).
- `python3 test/chain_e2e.py` — multi-edit chaining through the real binary + model.
- `python3 test/latency_cancel_e2e.py` — latency + mid-flight `$/cancelRequest`.
- `nvim --headless -u NONE -l test/nes_headless.lua` — **real sidekick.nvim NES driving the server in nvim**.

## Status — implemented & e2e-validated

All architecture unknowns resolved and all five issues shipped (TDD; see git history):

- **#02 LSP server** — stdio JSON-RPC, concurrent dispatch, `$/cancelRequest` cancellation.
- **#01 Engine** — vendored cursortab sweep provider behind `nes.Engine`; local model, no telemetry.
- **#03 nvim integration** — `get_client` redirect shim (name-based selection confirmed in live nvim) + Gap 2 anchor relaxation.
- **#04 Chaining** — `DocumentStore` derives `recent_changes` from `didChange`; model propagates + advances on re-request.
- **#05 Latency/cancellation** — median ~188ms; mid-flight cancels abort the model call cleanly.

End-to-end proven in **headless nvim with real sidekick.nvim**: the server attaches, an
edit + request yields a validated NES, and the model propagates `greet→greetings` to the next site.

The one axis this cannot change: **suggestion quality is bounded by the local model**, not Copilot.
