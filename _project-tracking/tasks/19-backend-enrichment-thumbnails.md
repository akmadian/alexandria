# 19 — Thumbnails leave ingest: the D25 pipeline change

**Areas:** backend. **Blocked by:** 18-backend-enrichment-engine.md.
**References:** D25, D28 (`docs/decisions.md`); `internal/importer/README.md`;
`internal/thumbnailer`; `docs/seam-events-jobs.md` (content-addressed thumb URLs).

Implement D25: ingest becomes `SCAN → HASH → MATCH → EXTRACT → WRITE`; thumbnail generation
becomes the first real enrichment kind on the task-18 engine.

## Scope

- Delete `stage_thumb.go` and its pipeline wiring/pool (`poolSizes.thumb`,
  `Workers.Ingest.Thumb` migrates to `Workers.Enrichment.thumbnail`); EXTRACT already yields
  dimensions/orientation for a correctly-shaped placeholder cell — verify, don't add.
- Register the `thumbnail` job kind: applicability via the `assettype` Thumb capability,
  no prerequisites, weight-by-size acquisition, RAW preview path through the exiftool daemon
  (delegation is permanent per D28).
- Post-commit enqueue: WRITE's post-commit hook nudges the dispatcher (hint, not truth — the
  scan remains the authority).
- **Clear-on-reimport staleness (D28):** the `actionReimport` transaction clears derived
  columns (including `thumbnail_at`) — but keeps the thumbnail file on disk so the grid shows
  the outdated-but-real thumb until regeneration overwrites it.
- **Re-derive the impl/04 cancel invariant:** "committed = fully processed" becomes
  "committed = identity + observation complete; enrichment converges." Rewrite the pipeline
  tests that assert thumbnails exist at commit; add the convergence-after-cancel test (cancel
  an import mid-run; already-committed assets get thumbnails via the scan with no re-import).

## Out of scope

Cheap signals (20), seam/grid states (21), thumbnail size tiers (ponytail marker,
`thumbnailer.go`), orphaned-thumb GC (DEFERRED §4).

## Acceptance

- Import of a fixture tree commits assets with no thumbnails; the enrichment scan then
  converges every eligible asset to thumbnailed, without re-import.
- Ingest throughput test (existing bench/fixture) shows the pipeline no longer pays decode
  cost; `import started` log no longer reports a thumb pool.
- Reimport of an edited file clears derived columns in the same tx (assert) and the file's old
  thumbnail survives on disk until the new one lands.
- Thumbnail failure → `enrichment_errors` row, asset committed and browsable regardless
  (D13 self-heal doctrine).
- `make check` green; importer README updated to the five-stage diagram in the same commit.
