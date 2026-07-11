# 06 — XMP sync remainder: caption/title inbound + `alexandria:Flag`

**Areas:** backend. **Blocked by:** nothing.
**References:** D15 (`docs/decisions.md`), `internal/xmp/README.md` (the implemented system —
field map, conflict grid, loop prevention), `docs/data-model.md` §1,
`ideation/backend-open-questions.md` #8/#14.

The XMP sync core shipped (inbound judgment apply, keyword union, outbound merge-write,
settings consumers, triggers, per-asset debounce — all present-tense in the package README).
Two field-map rows remain unwired:

## 1. Caption/title inbound — needs a sparse observation writer

`dc:description` → `caption` and `dc:title` → `title` cannot apply today because
`AssetObservationWriter.ApplyFilePatch` rewrites the file-fact columns wholesale — there is no
way to set caption/title alone without clobbering observation state the sidecar knows nothing
about.

- Add a narrow observation-metadata write (e.g. `ApplyMetadataPatch` with overlay-non-nil
  semantics, mirroring `FilePatch`'s style) on the observation writer — caption/title only,
  never file facts, never judgment (writer classes hold: observation column, observation
  writer).
- Wire it into `Syncer.SyncSidecar`'s inbound path beside the `TriagePatch` apply, same tx.
  Wholesale-clear semantics match the judgment fields (a sidecar omitting `dc:description`
  clears `caption` under `xmp_wins`); tags remain the union exception.
- Outbound already reads caption/title — verify round-trip in the acceptance tests.

## 2. `alexandria:Flag` custom namespace — best-effort, empirically gated

Read + write `alexandria:Flag` through the same explicit tag set. Before relying on it, run the
open-question #8 empirical test: does LrC preserve the unknown namespace when it rewrites a
sidecar? Record the result in the decision log (it decides whether flag survival is a promise
or a caveat). NEVER auto-map flags onto ratings/labels (lossy mappings are an opt-in P3 toggle).

## Acceptance

- LrC fixture with description+title → inbound sets caption/title; no `judgment_modified_at`
  bump; file-fact columns untouched (assert unchanged); second pass no-op.
- Sidecar omitting description under `xmp_wins` clears a previously-synced caption; under
  `catalog_wins` it does not.
- Flag round-trips through our own write/read; the LrC-rewrite survival result is recorded.
