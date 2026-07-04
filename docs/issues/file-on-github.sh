#!/usr/bin/env bash
# Promote the markdown issue files in this directory to GitHub issues.
#
# Prereqs (one-time, run by you — auth is interactive):
#   brew install gh
#   gh auth login
#   gh repo create local-copilot-nes --private --source=. --remote=origin --push
#
# Then, from the repo root:
#   bash docs/issues/file-on-github.sh
#
# Title  = the file's first "# " heading. Body = everything after it.
# Cross-references like "#01" in the bodies are human labels, not GitHub links.
set -euo pipefail

cd "$(dirname "$0")"
shopt -s nullglob

for f in [0-9][0-9]-*.md; do
  title="$(grep -m1 '^# ' "$f" | sed 's/^# //')"
  body="$(sed '1{/^# /d;}' "$f")"   # drop the leading title line from the body
  echo ">> filing: $title"
  gh issue create --title "$title" --body "$body"
done

echo "Done. Consider setting #01 as blocked-by nothing and #02–#05 dependencies in the GitHub UI."
