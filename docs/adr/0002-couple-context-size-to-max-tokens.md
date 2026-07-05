# 2. Couple the rewrite-window budget (`ContextSize`) to `MaxTokens`

Status: Accepted

## Context

The sweep next-edit model rewrites a **window** spanning from the cursor to the
end of that window, and the server diffs the rewrite against the original to
extract the minimal edit. The window is sized by `ContextSize` (a token budget
fed to `TrimContentAroundCursor`); the model's output is capped by `MaxTokens`.

Gap 2 (see `docs/issues/03-neovim-integration.md`) widened `ContextSize` to
`8192` so distant edits would be *expressible* — but left `MaxTokens` at `256`.
That is a 32:1 ratio, and it broke the common case in two compounding ways,
both confirmed by driving the real binary + model:

- **Truncation.** With a large window, the cursor→end-of-window span exceeds
  256 output tokens, so the rewrite is cut off (`finish_reason: length`). A
  truncated window can't be aligned back, so the server silently returns **zero
  edits with no error** — indistinguishable from "no suggestion."
- **Echo-mode.** Even when not truncated, asking the model to reproduce a whole
  ~366-line file makes the one-token change non-salient; the model copies the
  window verbatim and proposes nothing.

The failure is gated by *lines below the cursor*, not file size, which is why it
looked intermittent. The vendored engine's own default is telling: when
`ContextSize == 0` it falls back to `MaxTokens` (`provider/flow.go:95-96`) — a
**1:1** coupling. The absolute `8192` was a fork choice that the design never
contemplated.

## Decision

Default `ContextSize == MaxTokens` (both `256`), restoring the engine's intended
1:1 coupling. The rewrite window stays small enough that the cursor→end span
fits in `MaxTokens` (no truncation) and the edit stays salient (no echo-mode).
Broad *reference* context (`±150` lines, `sweep.go:18-19`) is separate and
unaffected — the model still *sees* wide context; it only *rewrites* a tight
band.

If more edits-per-request are wanted, raise **both** together (e.g. `512/512`),
keeping the ratio near 1:1 — never widen `ContextSize` alone.

## Rejected alternatives

- **Raise `MaxTokens` instead, keep `ContextSize=8192`.** Verified insufficient:
  at `MaxTokens=4096` the real-file rename still returned 0 edits (echo-mode),
  and on large files it just converts the silent drop back into a hard
  `exceeds context size` error. Treats the symptom, not the coupling.
- **Bound only the *trailing* window (asymmetric).** Plausible future
  refinement (reach backward far, rewrite forward little), but more invasive;
  the 1:1 coupling fixes the observed failure with a one-line default.

## Consequences

- Edits must land within ~±5 lines of the cursor; "action at a distance" needs a
  jump-to-site first (the intended workflow). This narrows Gap 2's reach.
- Fewer edits proposed per request → more sidekick chaining round-trips.
- Prompt shrinks ~3.5× → faster and further from the llama context ceiling.
