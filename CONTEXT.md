# Context / glossary

Canonical terms for this project. Definitions are conceptual — no implementation
details. When a term below conflicts with how code or docs use it, the code/docs
are wrong; fix them, not this file.

- **NES (Next-Edit-Suggestion):** a suggested edit at or beyond the cursor that
  the user reviews and accepts. Distinct from *inline completion*.
- **Inline completion:** ghost-text continuation at the cursor. A separate
  feature, owned by copilot.lua — not this project.
- **Persistence (version-gated):** an NES survives cursor movement, window
  switches, and file saves; it is invalidated *only* when the buffer's content
  (version) changes. The defining property vs. an ephemeral suggestion.
- **Jump:** moving the cursor to an NES's location — possibly off-screen —
  before applying it.
- **Chaining:** after applying one NES, presenting the next related NES so the
  user walks a sequence of edits one acceptance at a time.
- **The Client:** sidekick.nvim's NES module (unmodified). Owns lifetime,
  persistence, jump, chaining, and diff rendering.
- **The Server:** `local-copilot-nes` (this repo). Answers the Client's
  suggestion requests. Owns nothing about lifetime or rendering.
- **The Engine:** the component inside the Server that, given a *Snapshot*,
  produces a single candidate edit. Model + diff. No lifetime, no rendering.
- **Snapshot:** the immutable input the Engine computes against — the document
  text, its version, the cursor, and *recent changes*.
- **Recent changes:** the record of the user's latest edits, supplied to the
  Engine so it can propose the analogous next edit. Load-bearing for *chaining*.
- **Suggestion quality:** how good the proposed edits are. Bounded by the local
  model, independent of the Client/Server harness.
