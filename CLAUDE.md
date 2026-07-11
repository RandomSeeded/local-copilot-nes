## Writing ADRs

- Decisions live in `docs/adr/` as individual files (`0001-slug.md`, `0002-slug.md`, …).
- Before every commit, consider whether an ADR is warranted. Only write one if all three are true:
  1. **Hard to reverse** — changing course carries meaningful cost
  2. **Surprising without context** — a future reader will wonder "why did they do it this way?"
  3. **Real trade-off** — genuine alternatives existed and were rejected for specific reasons
- Each ADR is a short file: title + 1–3 sentences of context, decision, and rationale. Include rejected alternatives when the rejection is non-obvious.
- If a decision would prompt a "why don't we just…" question, it belongs in an ADR.
