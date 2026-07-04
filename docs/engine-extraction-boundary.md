# cursortab engine-extraction boundary

Goal: extract cursortab.nvim's **model + diff engine** into a standalone Go LSP
server (`local-copilot-nes`) that implements `textDocument/copilotInlineEdit`,
driven by an unmodified **sidekick.nvim** NES client and backed by the local
`llama-server`.

All `server/...` and `lua/cursortab/...` paths below refer to a clone of
`github.com/cursortab/cursortab.nvim`.

## The cut, in one sentence

cursortab's daemon runs:

```
nvim event → prepareCompletionInput → startProviderCompletion
           → provider.Complete → ComputeDiff → stages → Lua callback
```

We cut it to:

```
LSP copilotInlineEdit request → prepareCompletionInput(from LSP doc store)
           → provider.Complete → (tighten range) → LSP response {edits:[...]}
```

— deleting the **event machinery upstream** (loop, state machine, timers,
dismiss-on-move) and the **staging + render + callback machinery downstream**
(sidekick owns lifetime, jump, chaining, and diff rendering).

## Component disposition

| Component | file(s) | Disposition |
|---|---|---|
| nvim msgpack-RPC daemon | `server/daemon.go`, `server/main.go`, `server/buffer/buffer.go` handlers, `server/ipc_*.go` | **DISCARD** → replace with an LSP server front-end |
| Event loop + state machine (dismiss-on-move) | `engine/engine.go` (`eventChan`/`eventLoop`), `engine/events.go` (transition table) | **DISCARD** — sidekick owns triggers + lifetime |
| Debounce / idle timers | `engine/engine.go:327-389` | **DISCARD** — sidekick debounces `trigger.events` |
| Buffer mirror (nvim-synced) | `server/buffer/buffer.go`, `buffer/client.go`, `buffer/diffhistory.go` | **ADAPT** → re-source from an LSP `didOpen`/`didChange` document store |
| Snapshot builder | `engine/input.go:35-58` `buildCurrentSnapshot`, `engine/request.go:42` `prepareCompletionInput` | **ADAPT** → build from the doc store + request position, not the buffer mirror |
| Provider interface + flow | `engine/types.go:56-61`, `provider/flow.go` (`Build`/`Call`/`Parse`, `StartBatch`) | **KEEP** (lift as-is) |
| Concrete providers | `provider/sweep/`, `provider/zeta/`, `provider/fim`, ... | **KEEP** |
| Model wire (OpenAI-compat → llama-server) | `provider/openai.go` | **KEEP** |
| Context windowing | `provider/flow.go:99-107` `TrimContentAroundCursor` | **KEEP but WIDEN / PARAMETERIZE** (Gap 2) |
| Anchor rejection gate | `provider/processors.go:155-188` (`checkAnchorPosition`, `maxRatio=0.25`) | **KEEP but RELAX / DISABLE** (Gap 2) — expose as config, default off / 1.0 |
| Completion shape | `types/types.go:4-8` `Completion{StartLine, EndLineInc, Lines}` | **KEEP** — this is the raw edit |
| Diff (range tightening only) | `engine/completion.go:407-422` `ComputeDiff`, `text/diff.go` | **KEEP minimal** — tighten the replaced range; DISCARD extmark/display grouping (sidekick renders) |
| Staging / stage-splitting | `text/staging.go`, `advanceStagedCompletion`, `CursorPredictionTarget` | **DISCARD for v1**; **candidate to REUSE later** to synthesize multi-edit chains (see D1) |
| Extmark/overlay rendering, jump indicators | `lua/cursortab/ui.lua`, `events.lua` | **DISCARD entirely** — sidekick renders |
| Lua callbacks (`on_completion_ready`, ...) | `lua/cursortab/*.lua` | **DISCARD entirely** |
| Cancellation (context) | `engine/request.go:146-147` (`context.WithTimeout` + `currentCancel`) | **KEEP** — wire to `$/cancelRequest` (see D3) |

## New code to add

1. **LSP server scaffold** (Go LSP lib TBD — see D2). Methods:
   - `initialize` — declare `textDocumentSync` (full or incremental) and `offsetEncoding` (utf-16).
   - `textDocument/didOpen` · `didChange` · `didClose` — maintain the document store (text + version). *Required* so nvim populates `vim.lsp.util.buf_versions[buf]`, which sidekick's persistence gate depends on.
   - `textDocument/didFocus` — no-op (sidekick sends it to any "copilot" client).
   - `textDocument/copilotInlineEdit` — the handler (below).
   - `workspace/executeCommand` — no-op (accept-telemetry hook; chaining is driven by `SidekickNesDone`, not this response).
   - `$/cancelRequest` — cancel the request's context, aborting the llama-server call.
2. **`copilotInlineEdit` handler:** params `(position, textDocument.version)` → build snapshot from doc store → run the extracted engine → get `Completion{StartLine, EndLineInc, Lines}` → convert to LSP `{range, newText}` → return `{ edits: [ { range, text, textDocument:{uri, version}, command } ] }`, **echoing the request's `version`** so sidekick's `edit:valid()` passes.
3. **Completion → TextEdit converter:** line-range replace → LSP range; optionally tighten via `ComputeDiff` for a better jump anchor.

## Main adaptation risk

Re-sourcing the snapshot from an LSP document store instead of cursortab's
nvim-synced `buffer.Buffer`. `buildCurrentSnapshot` reads `buffer.Buffer`
internals; the cleanest path is a small shim that satisfies the same interface
backed by LSP-synced text. This is the bulk of the "ADAPT" work; everything
marked KEEP lifts with little change.
