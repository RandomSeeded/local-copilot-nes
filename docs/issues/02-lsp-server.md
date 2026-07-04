# Implement the `local-copilot-nes` LSP server (`copilotInlineEdit`)

## Context
cursortab speaks nvim msgpack-RPC, not LSP. sidekick's NES client speaks
standard `vim.lsp`. We need a real LSP server that implements the small method
set sidekick uses, wrapping the extracted engine (#01).

## Decision needed (D2)
Pick the Go LSP scaffold:
- `go.lsp.dev/protocol` (typed, full), or
- `tliron/glsp` (lighter), or
- hand-rolled JSON-RPC 2.0 over stdio (smallest surface — we implement ~6 methods).

Given the tiny method set, hand-rolled or `glsp` is likely enough. Decide first.

## Method set (all derived from `sidekick/nes/init.lua`)
- `initialize` — advertise `textDocumentSync` (full is fine), `positionEncoding`/`offsetEncoding` = utf-16. **Required** so nvim populates `vim.lsp.util.buf_versions[buf]`, which sidekick's persistence gate depends on.
- `textDocument/didOpen` · `didChange` · `didClose` — maintain the `DocumentStore` (text + version). Must track recent edits too (feeds #04's recent-changes prompt).
- `textDocument/didFocus` — no-op; must not error (sidekick sends it to any copilot client, `nes/init.lua:106`).
- `textDocument/copilotInlineEdit` — the handler: params `(position, textDocument.version)` → `engine.Complete(...)` → convert `Completion{StartLine,EndLineInc,Lines}` to LSP `{range,newText}` → return `{ edits: [ { range, text, textDocument:{uri,version}, command } ] }`. **Echo the request's `version`** so `edit:valid()` passes (`nes/edit.lua:43-47`).
- `workspace/executeCommand` — no-op (accept-telemetry; chaining is driven by `SidekickNesDone`, not this response).
- `$/cancelRequest` — cancel the in-flight request's `context`, aborting the llama-server call (reuse cursortab's `context.WithTimeout`/`currentCancel` pattern, `engine/request.go:146-147`). Load-bearing for latency (#05).

## Response shape (exact)
```
{ "edits": [ { "range": <lsp.Range>, "text": <newText>,
              "textDocument": { "uri": <uri>, "version": <echoed> },
              "command": <benign lsp.Command> } ] }
```
Return `{ "edits": [] }` for "no suggestion".

## Depends on
#01 (the engine library).

## References
- Client contract: `sidekick/lua/sidekick/nes/init.lua` (`M.update`, `_handler`, `M.cancel`), `nes/edit.lua`.
- `docs/engine-extraction-boundary.md` ("New code to add").
