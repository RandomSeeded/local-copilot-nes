# 0003 — Reject local zeta-2.1 for Zed-style NES; keep the sweep provider

Status: Accepted (2026-07-05)

## Context

We explored replacing/augmenting the `sweep` provider with Zed's open **Zeta 2.1**
model to get Zed's Next-Edit-Suggestion UX — in particular the headline behavior:
rename/edit in one place, then **jump to a caused edit elsewhere in the file** ("next
jump"). Zeta 2.1 is Zed's *own* open edit-prediction model, so the natural assumption
was "just run it locally and we get Zed's NES."

That assumption is wrong, and the gap is structural, not a tuning problem.

## Decision

**Abandon the local-zeta approach. Keep `sweep` as the local NES engine.** Do not
maintain a zeta2/zeta21 provider, a diagnostics-forwarding channel, or the pluggable
`-provider` scaffolding built for them.

## Why (evidence, from Zed source `github.com/zed-industries/zed@main` + HF model cards)

1. **Zed's far-edit "jumps" are a cloud-only, closed model.** They run on Zed Cloud's
   `POST /predict_edits/v4` with an **8192-token editable context** + whole-file syntax
   ranges + LSP diagnostics, gated behind the `edit_prediction_jumps` feature flag
   (`allow_jump = is_cloud_zeta && has_flag(...)`). No open weights exist for this
   path (the `V0608QwenMultiRegions` / `V0615HashRegions` formats have no published
   model).

2. **Open zeta-2.1 is a small-window model.** `ZetaVersion::Zeta2_1 => V0318SeedMultiRegions`,
   whose `token_limits_for_format` is **(editable 350, context 150)** — a ~35-line
   editable window around the cursor, multi-region markers, and **no diagnostics**
   (confirmed by both the Rust source and the `zed-industries/zeta-2.1` model card,
   Seed-Coder-8B, version `0323-multi-region-filtered-r3`). The model can only emit
   edits *inside* that window; a usage 40+ lines away is unreachable in one request.

3. **Zed itself disables jumps for self-hosted zeta.** When Zed runs a local/Ollama
   zeta model, `is_cloud_zeta` is false, so jumps are off. You get **in-window edits
   at the cursor, no diagnostics, no cross-file jump** — the window simply follows the
   cursor. That is the ceiling for local, and for our purpose it is **no better than
   the existing `sweep` provider**.

4. **Empirically** (Q4_K_M GGUF on llama-server): in-window rename propagation works
   (edit-history driven — e.g. `lcs`→`dp` across ~12 clustered lines), but edits far
   from the cursor no-op — exactly what the small-window design predicts.

## Rejected alternatives

- **Force an 8192-token editable region on zeta-2.1** to get whole-file edits —
  off-distribution (it was trained for the 350-token multi-region format), and not even
  selectable for local models in Zed. Our own whole-file experiment no-op'd on far edits.
- **Diagnostics as prompt context** (V0420Diagnostics shape) — not part of zeta-2.1's
  format at all; only the cloud model consumes diagnostics. This was built and then
  removed as a divergence from what the open model actually does.

## Consequence

Local NES stays on `sweep` (see ADR 0001). Revisit only if Zed publishes open weights
for the large-window/jumps format, or a comparable open next-edit model appears.
Suggestion quality remains bounded by the local model — that ceiling did not move.
