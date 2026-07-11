#!/usr/bin/env bash
# check-docs — the mechanical half of the docs system (D27). Pure greps, zero
# judgment: every finding prints file:line, the rule violated, and the remedy.
# Runs from the pre-commit hook and inside `make check`; CI is the backstop.
set -u
cd "$(dirname "$0")/.."

findings=$(mktemp)
trap 'rm -f "$findings"' EXIT

# Rule 1 — durable docs never point at work items (D27 stability rule).
grep -rn -E '_project-tracking/(epics|tasks|ideation)/' \
    CLAUDE.md docs internal frontend/CLAUDE.md --include='*.md' 2>/dev/null |
  while IFS= read -r hit; do
    printf '%s\n  RULE: durable docs never cite a work item as authority (work items are transient — D27)\n  FIX:  point at docs/, a package README, or a D-number instead\n' \
      "${hit%%:*}:$(printf '%s' "$hit" | cut -d: -f2)" >> "$findings"
  done

# Rule 2 — no status prose; state lives only in the directory tree.
grep -rn -E '✅|\bDONE\b|IN PROGRESS' docs _project-tracking --include='*.md' 2>/dev/null |
  grep -v -E '^(docs/decisions\.md|_project-tracking/DEFERRED\.md):' |
  while IFS= read -r hit; do
    printf '%s\n  RULE: status prose is recorded state and WILL drift (D27) — state = directory, done = deleted\n  FIX:  delete the status line; if the outcome matters, it belongs in docs/decisions.md or DEFERRED.md\n' \
      "${hit%%:*}:$(printf '%s' "$hit" | cut -d: -f2)" >> "$findings"
  done

# Rule 3 — work-item filename contract (area is an attribute in the name).
for f in _project-tracking/tasks/*.md; do
  [ -e "$f" ] || continue
  basename "$f" | grep -qE '^[0-9]{2}-(backend|frontend|seam|ops|perf|docs)-.+\.md$' ||
    printf '%s:1\n  RULE: task filenames are NN-<area>-<slug>.md (NN orders the queue; area ∈ backend|frontend|seam|ops|perf|docs)\n  FIX:  git mv to a conforming name\n' "$f" >> "$findings"
done
for f in _project-tracking/epics/*.md; do
  [ -e "$f" ] || continue
  basename "$f" | grep -qE '^(backend|frontend|seam|ops|perf|docs)-.+\.md$' ||
    printf '%s:1\n  RULE: epic filenames are <area>-<slug>.md (area ∈ backend|frontend|seam|ops|perf|docs)\n  FIX:  git mv to a conforming name\n' "$f" >> "$findings"
done

# Rule 4 — no dead relative markdown links (the phantom-guides class).
{ find docs _project-tracking internal -name '*.md' 2>/dev/null; echo CLAUDE.md; [ -f frontend/CLAUDE.md ] && echo frontend/CLAUDE.md; } |
  while IFS= read -r file; do
    dir=$(dirname "$file")
    grep -noE '\]\([^)]+\)' "$file" 2>/dev/null | while IFS=: read -r line match; do
      target=${match#*(}; target=${target%)}
      target=${target%%#*}
      case "$target" in ''|http://*|https://*|mailto:*|/*) continue ;; esac
      [ -e "$dir/$target" ] ||
        printf '%s:%s\n  RULE: markdown link target does not exist (%s)\n  FIX:  retarget to the moved file or delete the link — never link a doc you have not written yet\n' \
          "$file" "$line" "$target" >> "$findings"
    done
  done

if [ -s "$findings" ]; then
  printf '\n\033[1;31m✗ check-docs — the docs system has drifted:\033[0m\n\n'
  cat "$findings"
  printf '\n%s finding(s). Rules: D27 in docs/decisions.md.\n' "$(grep -c '  RULE:' "$findings")"
  exit 1
fi
printf '\033[1;32m✓ docs system clean\033[0m\n'
