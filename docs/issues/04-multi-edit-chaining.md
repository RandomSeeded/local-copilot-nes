# Multi-edit chaining: validate & tune on real code

## Context
The "make these 5 modifications, one Tab at a time" flow. sidekick drives it by
emitting `User SidekickNesDone` after each apply (`nes/init.lua:323`), which
re-requests; the server just needs to return the *next* logical edit given the
post-apply buffer.

**D1 spike (resolved, synthetic):** `sweep-next-edit-1.5B` propagated
`greet→greetings` to the next handler given `recent_changes`, and a re-request
advanced to the handler after that. So chaining can ride sidekick's re-request
loop — **no server-side stage-splitting needed for v1.** Reproduce:
`spikes/d1_chain_test.py` (needs `llama-server` on :8000).

## Tasks
- **Validate on real, non-synthetic code** — non-identical sites, messy diffs,
  mixed languages. Confirm the re-request loop advances correctly and the model
  doesn't stall or loop.
- **Chain-stop policy** — server returns `{edits:[]}` when there's no confident
  next edit; confirm sidekick hides (`SidekickNesHide`) and the chain ends. Guard
  against runaway chains.
- **Recent-changes plumbing** — chaining depends on the `EditHistory` material
  reaching the sweep prompt's `recent_changes` section
  (`sweep.go:106-111`, `formatDiffHistoryOriginalUpdated`). Ensure the
  DocumentStore (#01/#02) records recent edits and the engine forwards them.
- **Output-quality guard** — D1's raw output prepended a hallucinated
  `def greetings(...)` line. After relaxing the anchor gate (#03), keep a light
  anchor/format guard so drift lines don't reach the buffer.

## Fallback (only if real-code chaining is weak)
Port cursortab's stage-splitting (`text/staging.go`, `CreateStages`,
`advanceStagedCompletion`) into the server to synthesize the chain server-side
from one window rewrite, instead of relying on re-request.

## Depends on
#02, #03. (Spike reproducible now against the running server.)

## References
- `spikes/d1_chain_test.py`, `spikes/d1-results.md`
- `sidekick/nes/init.lua:323` (`SidekickNesDone`), `config.lua:16` (trigger.events)
