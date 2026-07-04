# D1 spike: does the local next-edit model chain?

**Question:** given a buffer where the user just made edit-1, does
`sweep-next-edit-1.5B` predict the analogous edit-2 at the next similar site —
and does a re-request advance to the site after that?

**Method:** `d1_chain_test.py` reconstructs cursortab's sweep prompt faithfully
(`<|file_sep|>` sections, `recent_changes` diff, `<|cursor|>` marker, stop tokens),
against `llama-server` serving `sweep-next-edit-1.5B` on `:8000`. Two steps:
1. handler_0 fixed (`greet→greetings`), `recent_changes` carries that edit, cursor on handler_1.
2. handler_0+1 fixed, cursor on handler_2 (simulates applying step 1 then re-requesting).

**Result: YES on both steps.**
- Step 1 — model rewrote handler_1's `greet(name)` → `greetings(name)`. Propagated.
- Step 2 — re-request advanced and rewrote handler_2's `greet` → `greetings`. Chained.

**Implication:** multi-edit chaining can ride sidekick's `SidekickNesDone`
re-request loop; **no server-side stage-splitting needed for v1** (see issue #04).

**Caveats (carried into #04 / #03):**
- Synthetic, maximally-regular input (identical repeated blocks) — the easiest
  propagation case. Proves feasibility, not robustness on messy real code.
- Raw output prepended a hallucinated `def greetings(...)` line before the
  correct edit — the anchor/format guard we plan to relax (Gap 2) exists to
  filter exactly that. Keep a light guard.

Reproduce: `python3 spikes/d1_chain_test.py` (requires llama-server on :8000).
