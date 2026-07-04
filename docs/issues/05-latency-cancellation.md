# End-to-end latency & cancellation validation

## Context
The user types very fast. In cursortab, per-keystroke `cursor_moved` events
rejected in-flight requests, cascading into ~3s felt latency. The new design is
structurally better: sidekick **debounces** its trigger events and sends a clean
LSP `$/cancelRequest` (`nes/init.lua:195`) when superseding a request, and our
server aborts the model call via `context` (#02). This issue validates that
empirically.

## Tasks
- **Measure** trigger→suggestion latency (p50/p95) under fast typing, with the
  local model + KV prefix cache (`llama-server ... cache_prompt`, `-np` slots).
- **Confirm no cancel-cascade** — superseded requests must abort the llama-server
  call promptly on `$/cancelRequest`; verify no pile-up of orphaned generations.
- **Tune** sidekick's trigger debounce and the server's `CompletionTimeout`.
  Previously landed on ~150ms debounce for fast typing; re-evaluate for this path.
- **Verify persistence interplay** — moving the cursor must NOT cancel or dismiss
  (that's the whole point vs cursortab); only content change re-triggers.

## Depends on
#02, #03.

## References
- `sidekick/nes/init.lua:190-198` (`M.cancel` → `$/cancelRequest`), `_handler` stale-drop `:208-210`.
- llama-server flags from prior tuning: `-ngl 99 -fa on --spec-type ngram-simple -np 4`.
