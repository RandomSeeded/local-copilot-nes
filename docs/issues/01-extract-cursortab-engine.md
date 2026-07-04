# Extract cursortab's model+diff engine into a standalone Go library

## Context
cursortab.nvim's Go daemon couples its (good) model+diff engine to an
nvim-msgpack event loop and state machine whose central assumption is
**dismiss-on-move**. We want only the engine — "given a document snapshot +
cursor, produce an edit" — with no event loop, no lifetime policy, no rendering.
sidekick owns all of that.

## Scope
Fork/vendor `github.com/cursortab/cursortab.nvim` `server/` and reduce it to a
library exposing a single entrypoint, roughly:

```go
func Complete(doc DocumentStore, uri string, pos Position) (*Completion, error)
// Completion{StartLine, EndLineInc, Lines}  (types/types.go:4-8)
```

**KEEP (lift as-is):** provider interface + flow (`engine/types.go:56-61`,
`provider/flow.go`), concrete providers (`provider/sweep`, `zeta`, `fim`), the
OpenAI/llama wire (`provider/openai.go`), the `Completion` type, and
`ComputeDiff` (`engine/completion.go:407-422`) for range-tightening only.

**ADAPT:** re-source the snapshot. `buildCurrentSnapshot` (`engine/input.go:35-58`)
and `prepareCompletionInput` (`engine/request.go:42`) currently read cursortab's
nvim-synced `buffer.Buffer`. Introduce a `DocumentStore` interface (text +
version + cursor) and back the snapshot off it instead.

**DISCARD:** event loop + state machine (`engine/engine.go`, `engine/events.go`),
debounce timers, staging (`text/staging.go`, v1), the msgpack daemon
(`server/daemon.go`, `buffer/*` RPC), all Lua.

## Depends on
Nothing. First issue.

## Open question (D4 — resolve early, it sizes this issue)
How tightly does `buildCurrentSnapshot` couple to `buffer.Buffer`, and what does
`RequiredMaterials` (`provider/sweep/sweep.go:38-39`: Diagnostics, Treesitter,
GitDiff, RecentFiles, EditHistory, UserActions) pull? EditHistory/UserActions
drive the recent-changes prompt section that makes chaining work (see #04), so
the DocumentStore must be able to supply recent edits — not just current text.

## References
- `docs/engine-extraction-boundary.md` (KEEP/ADAPT/DISCARD table)
- Sweep prompt format: `provider/sweep/sweep.go:53-160`
