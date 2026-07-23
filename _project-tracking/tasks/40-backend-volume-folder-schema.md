# 40 — Volume/Folder schema split: the D24 rekey, the resolver, the rename ripple

**Areas:** backend. **Blocked by:** nothing.
**References:** D24 (the split + "compare keys, open bytes" + real copies), D41 (the round's
rulings), D20 (nothing auto-mutates identity), C7, C13, C15, DEFERRED §1 (split assessment:
"migration + keying off a new field, not a gut-rewrite"), §8 (PathKey wiring — same schema
event, key-to-key design note).

## Scope

`Source` retires. `sources` splits in `0001_initial_schema.sql` **in place** (pre-release
policy): **no migration code is written; dev catalogs re-import.**

- **`volumes`** — the identity/portability anchor: `id, name [jdg], kind
  (local|external_drive|smb|nfs) [jdg], host [jdg], share_name [jdg], filesystem_uuid [obs],
  disk_serial [obs], volume_label [obs], connectivity [obs], created_at, updated_at`.
- **`folders`** — the tracked root: `id, volume_id (FK), path (volume-relative) [jdg],
  name [jdg], sync_mode (manual|watched|scheduled) [jdg], scan_recursively [jdg],
  enabled [jdg], poll_interval_secs [jdg], last_scanned_at [syn], created_at, updated_at`.
- **The split principle is identity vs. tracking scope, NOT writer class** (D41): both tables
  carry mixed writer classes; enforcement stays per-column via the catalog writer interfaces,
  exactly as today.
- **Assets rekey to `(volume_id, relative_path)`** (volume-relative, re-based from the old
  source-relative paths); `sidecar_files` likewise. Keep `ON DELETE RESTRICT` on
  `assets.volume_id`. Unique identity index moves to `(volume_id, path_key)`.
- **`path_key`** — stored NFC-normalized key column (`domain.PathKey`) on `assets` +
  `sidecar_files`; every path equality/match/dedup compares key-to-key (§8's design note:
  never per-query normalization). Derived → **registered rebuild function** (alongside
  `rebuild fts`).
- **The path→volume resolver** — absolute path → mount point → filesystem UUID →
  find-or-create `volumes` row → `(volume_id, relative_path)`. The volume monitor's probe is
  the source. Callers: folder-add, the importer, watcher event mapping.
- **Folder-add semantics, engine-side** (D41 graceful merge + its quiet-by-default dated
  note; disjoint roots invariant): `CreateFolder(path, confirm bool)` returns an outcome,
  never a bare overlap error —
  `created` | `alreadyTrackedWithin(existingFolderID)` (subfolder of a tracked root; exact
  duplicate → self) | `absorbed(replacedFolderIDs)` (parent of existing roots: they fold into
  the new wider root — performed QUIETLY when no behavior changes) |
  `needsConfirmation(replacedFolderIDs, behaviorChanges)` (a watched/scheduled root would
  change sync behavior under the new parent: NOTHING mutates; the caller re-calls with
  `confirm=true` to proceed as `absorbed`).
- **The rename ripple** (mechanical, complete in this task): domain nouns + catalog
  interfaces; the ast token `source` → `volume`, folder-scope payload `sourceId` → `volumeId`
  (the one camelCase-derivation exception dies); event string `sourceStatus` →
  `volumeStatus`; the `poll.go:48` connectivity marker resolves to the volume column;
  `settings` IOTokens cap becomes per-volume (≈ per-device — record the improvement at
  DEFERRED §11, build nothing); `cmd/dev`; `make generate` + crosswalks green.
- `sync_mode` values are D41's sync_mode ruling; the enum may already exist in
  `internal/domain` (task 42's forward slice) — consume, don't redeclare.

## Out of scope

Deriver + counts (41), seam services/bind (45), any UI (43/44), watcher supervision
(DEFERRED §2), per-subtree sync overrides (DEFERRED §19), volume-row GC (harmless
pre-release).

## Acceptance

- `make check` green: the full rename compiles, crosswalks + generated TS fresh, no `Source`
  noun survives outside git history and sanctioned frozen snapshots.
- Import a fixture tree end-to-end (`cmd/dev import`): assets land keyed
  `(volume_id, relative_path)` with correct `path_key`; re-import is idempotent.
- Resolver: two folders on one filesystem yield ONE `volumes` row; an NFD-named fixture file
  matches its NFC query form via `path_key` (the §8 phantom-identity case closes — assert no
  spurious new asset + review pair).
- `CreateFolder` outcomes: subfolder-of-tracked → `alreadyTrackedWithin`; parent-of-tracked
  with uniform sync behavior → quiet `absorbed`, prior roots' assets re-based under the new
  root, no asset identity churn (D20: same file ≠ new asset); parent-of-tracked where a
  watched root would change behavior → `needsConfirmation` with NO mutation, then `absorbed`
  on the confirmed re-call; disjointness holds after every outcome.
- `path_key` rebuild function registered and reproduces identical keys.
- Logging: resolver decisions + folder-add outcomes at Info, per-path at Debug.
