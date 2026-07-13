# 20 — Cheap signals: sharpness, clipping, phash as enrichment kinds

**Areas:** backend. **Blocked by:** 19-backend-enrichment-thumbnails.md.
**References:** D28, C11 (signals are metadata columns, never verdicts), C7/C15 (new
filterable field = vocabulary row + compiler entry, generated everywhere),
`docs/data-model.md` §2 (column criteria), `epics/frontend-culling-signals.md` residue in
D28 / FR P3 signals block.

Three registry rows on the task-18 engine, each prerequisite: the thumbnail artifact (read the
512px thumb from disk — per-artifact atomicity is worth the single-digit-ms re-decode, D28).

## Scope

- **Columns** (edit `0001_initial_schema.sql` in place): `sharpness` REAL (Laplacian variance,
  normalized), `clipping_highlights` REAL + `clipping_shadows` REAL (histogram %), `phash`
  TEXT/BLOB (64-bit perceptual hash, hex). All derived-class, all NULL = not computed, all
  cleared by the reimport staleness path (19).
- **Job kinds**: `sharpness`, `clipping`, `phash` — separate rows, separate DLQ rows (one kind
  = one artifact). Pure Go, no new deps unless a hash/DCT library genuinely carries load
  (dependency policy: redundancy test, not a ban).
- **Query surface** (C7/C15): vocabulary fields + compiler entries so `sharpness > 0.5`
  filters/sorts through `ast` like everything else; `make generate` refreshes TS unions +
  data dictionary. NULL semantics follow the existing unrated-is-NULL policy.
- Signal fidelity note lands in the package docs: signals read the 512px/q80 thumbnail;
  ranking *within* a burst is the contract, absolute values are not (D28/epic residue).

## Out of scope

Heavy signals (faces/blink/embeddings — future registry rows at P3/P4), phash *clustering* /
burst collapse (grouping deep-dive, open question #7), suggested-rejects UX, threshold-filter
UI annotations ("N not yet scored" — frontend, rides 21's decoration).

## Acceptance

- Fixture images with known character (sharp/blurred, clipped/neutral, near-duplicate pair)
  produce ordered sharpness, sensible clipping percentages, and phash values within hamming
  distance ≤ threshold for the near-dup pair — golden-value tests with tolerance.
- `ast` compiles sharpness/clipping/phash filter + sort tokens (compiler tests); crosswalk
  suite pins the new vocabulary rows; `make generate` diff is committed and freshness-gated.
- Full convergence test: import → thumbnails → all three signals present, via scans alone.
- Re-running a signal kind on an enriched asset is a no-op/harmless overwrite (idempotence).
