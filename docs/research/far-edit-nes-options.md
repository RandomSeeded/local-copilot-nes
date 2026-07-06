# Far-edit NES: options & the open-model landscape (research synthesis)

Research note answering: *how do people run "next edit suggestion" (Cursor-Tab-style)
locally, why does our local implementation miss "far edits," and is there an open,
locally-runnable model that predicts edit **location** (not just rewrites the cursor
region)?* Compiled 2026-07; sources inline.

## TL;DR

- **"Far edits" split into two mechanisms.** *Approach A* = one model **rewrites a
  bounded editable region** around the cursor; a "far" edit is just a change it makes
  elsewhere **inside that region**, inferred from your recent-edit diffs. *Approach B*
  = a dedicated step predicts **where** the next edit is (a jump target, possibly
  cross-file), feeding a content model.
- **Every open, editor-ready edit model is Approach A** (Zed Zeta, Continue Instinct,
  Sweep). Our `sweep-next-edit-1.5B` is Approach A with a ~21-line window.
- **Genuine open Approach-B models exist only as research artifacts** and are modest
  quality: **CoEdPilot** (small project-wide file+line locator) and
  **NextEditPrediction/lurf21** (3B/7B, PositionMatch ~65%@7B). Neither is packaged
  for an editor.
- **The moat is data, not architecture.** True Approach-B (Copilot Long-Distance NES,
  Cursor Fusion) needs millions of real **intra-session edit trajectories** with
  accept/reject labels + an online-RL selectivity loop. No large open dataset of that
  exists. Even **Zed does not use a learned locator** — its cross-location "jump" is
  driven by **LSP diagnostics** (language-server flags the call site → Zeta rewrites it).
- **Practical takeaway:** the far-edit UX is reachable *without* the model that doesn't
  exist — pair a strong Approach-A rewriter with a **heuristic locator** (LSP
  references / diagnostics / tree-sitter / next-diff), which is exactly what Zed and
  cursortab do.

## Why our far edits fail — three stacked ceilings

| Ceiling | Where it lives | Consequence |
|---|---|---|
| 1. Model capability | `sweep-next-edit-1.5B` | 1.5B can't infer "rename intent → propagate." Content only. |
| 2. Structural reach | `ContextSize=MaxTokens=256` (ADR 0002); ~21-line window | An edit far from the cursor is **unrepresentable** in one response. No model swap fixes this. |
| 3. No locator | We only request/rewrite at the cursor window | We have **zero** Approach-B mechanism — not even the heuristic Zed uses. |

## Open edit-prediction models (what's runnable locally)

| Model | Approach | Open + local? | Size | Notes |
|---|---|---|---|---|
| **Sweep next-edit** | A | Yes, Apache-2.0, GGUF | 0.5/1.5/**v2-7B** | What we run. "tab-to-jump" is in-window. Larger "good" Sweep is a hosted/closed model. |
| **Continue Instinct** | A | Yes, GGUF/Ollama | 7B (Qwen2.5-Coder FT) | Beats Zeta on their own LLM-judge eval. |
| **Zed Zeta / Zeta2** | A | Yes | 7B / 8B | Open weights are the quality frontier for *open* edit models; still below Cursor. (We explored Zeta; open weights unimpressive.) |
| **Microsoft NextCoder** | applies an instructed edit, not location | Yes, GGUF | 7/14/32B | Not next-edit-from-history. |
| **JetBrains Mellum** | FIM completion only | Yes, Apache-2.0 | 4B / 12B MoE | Not NES. |
| **CoEdPilot** | **B** (file+line locator) | Yes, HF | ~220M each | Genuine location prediction; weak generator (~42% EM). Best reusable open *locator*. |
| **NextEditPrediction (lurf21)** | **B** (location+content) | Yes, HF (no GGUF) | 3B/7B/14B/32B | PositionMatch ~65%@7B; no editor tooling. |
| **Copilot NES / Cursor Fusion** | **B** | No (closed) | — | The quality bar; not reproducible locally. |

## The data gap (why you can't just train Approach B)

Public edit datasets are tiny or synthetic: zed-industries/zeta ~400, continuedev
instinct-data ~9k (mostly synthetic), lurf21 ~3k (commit-derived), codefuse ~61,
microsoft NextCoderDataset ~381k (synthetic, no trajectories). **No large open corpus
of real intra-session editing trajectories with accept/reject or next-location labels.**
CoEdPilot's approach (small locator trained on line-diff format derived from commit
histories — no keystroke data) is the most replicable open blueprint.

## Options (with pros/cons)

**Lever 1 — upgrade the content model (Approach A).**
- *Sweep v2-7B* — likely same `<|file_sep|>` format → minimal change; local/free. Con: 7B latency ~0.8–1.5s on M1–M3; verify v2 prompt format.
- *Continue Instinct 7B* — beats Zeta on their eval; editable-region format close to our `zeta` provider. Con: match its exact template; still Approach A.
- *Point our `zeta` instruction-provider at a fast free cloud model* (Groq/Gemini-Flash-free/DeepSeek) — the `zeta` provider emits a plain Alpaca instruction a general model follows zero-shot → no fine-tune; big reasoning jump; sub-100ms TTFT on Groq. Con: needs a `/v1/chat` adapter; code leaves the machine; markers emitted imperfectly (Parse has a fallback).

**Lever 2 — fix the reach cap (model-independent).** Decouple `ContextSize` from
`MaxTokens`; adopt Zeta-2.1 "multi-region" (emit only changed sub-regions) to widen
reach without latency blowup; evaluate the vendored `zeta2` provider. Con: multi-region
marker parsing is fiddly; still single-file.

**Lever 3 — add a heuristic locator = real Approach-B behavior (the Zed trick).**
- *3a Rename/identifier propagation (deterministic):* on a rename in `recent_changes`,
  scan buffer/open buffers (tree-sitter/ripgrep) for other occurrences, return NES edits
  at those far ranges (invert the `anchorMaxRatio` gate for this path). Catches the #1
  far-edit case for free.
- *3b Diagnostics-driven jump (mirror Zed):* after an edit, read the LSP diagnostics for
  new errors your edit caused and request/rewrite at those locations. Compiler-verified
  far edits; fully local; no training.

**Lever 4 — learned locator / fine-tune.** Wire CoEdPilot's open locator, or fine-tune a
small edit model on **our own logged edit traces** (our LSP server already sees the full
`didChange` stream → we can build the scarce trajectory dataset). Highest local ceiling;
most effort; won't reach Cursor without online-RL.

**Lever 5 — escape hatch / calibration.** GitHub Copilot free tier (real NES, drivable in
nvim) as a benchmark; not local.

## Recommendation (by leverage)

1. Config-only: decouple the 256 cap (Lever 2) + swap in Sweep v2-7B / Instinct 7B
   (Lever 1); run the *ceiling test* by pointing the `zeta` provider at a fast free
   cloud model. If far-edit quality jumps, the **model** was the limit, not the harness.
2. The real win: build the **heuristic locator** (Lever 3) — deterministic rename
   propagation first, then diagnostics-driven jumps. Rides sidekick's jump + chaining.
3. Only if you love it: fine-tune on your own logged trajectories (Lever 4).

**Bottom line:** stop hunting for an open Approach-B *model* — it isn't there, and even
Zed doesn't use one. The best local far-edit setup is *a strong Approach-A rewriter + a
heuristic (diagnostics/reference) locator*, and our harness is ~80% of the way there.

## Key sources

- Zed edit prediction / Zeta: zed.dev/blog/edit-prediction · zed.dev/blog/zeta2 ·
  zed.dev/blog/zeta2-1 · huggingface.co/datasets/zed-industries/zeta
- Copilot NES + Long-Distance NES: githubnext.com/projects/copilot-next-edit-suggestions ·
  code.visualstudio.com/blogs/2026/02/26/long-distance-nes ·
  github.blog/ai-and-ml/github-copilot/evolving-github-copilots-next-edit-suggestions-through-custom-model-training
- Cursor Tab/Fusion + online RL: cursor.com/blog/tab-update · cursor.com/blog/tab-rl ·
  lexfridman.com/cursor-team-transcript
- Open models: blog.sweep.dev/posts/oss-next-edit · huggingface.co/sweepai/sweep-next-edit-v2-7B ·
  blog.continue.dev/instinct · arxiv.org/abs/2508.10074 (NextEditPrediction/lurf21) ·
  arxiv.org/abs/2408.01733 (CoEdPilot) · huggingface.co/JetBrains/Mellum-4b-base ·
  huggingface.co/microsoft/NextCoder-7B
- Local serving (Apple Silicon): github.com/ggml-org/llama.vim ·
  github.com/milanglacier/minuet-ai.nvim
