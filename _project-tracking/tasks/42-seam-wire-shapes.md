# 42 — Wire shapes + contract + mock: the mock-first seam for the rail

**Areas:** seam, frontend. **Blocked by:** nothing (shapes ratified by D41 — this is Track B's
root; no engine work).
**References:** D41 (wire shapes, count semantics, folder-add outcomes), C7, C13, C15 (declared
in Go, generated everywhere), C14, DEFERRED §18 (collection projection), the frontend
architecture record (mock parity rules).

## Scope

- **Wire projections in `internal/seam`** (json-tagged, camelCase), generated to TS:
  - `VolumeNode { id, name, kind, connectivity, assetCount, folders []FolderNode }`
  - `FolderNode { id, name, path, syncMode?, assetCount, children []FolderNode }` — `id` is
    the folder row ID for tracked roots, synthetic (`volumeId + ":" + path`) for derived
    nodes; `syncMode` only on tracked roots.
  - `CollectionNode { id, name, parentId?, kind (domain.CollectionKind), assetCount *int }` —
    `nil` = count unavailable, `0` = empty (D41). With smart counts computed for real, `nil`
    is the backend's declared retreat for a count it declined to compute (D41's
    pathological-query escape hatch) — the wire shape keeps that door open.
  - `CreateFolderOutcome` — `created | alreadyTrackedWithin | absorbed | needsConfirmation`
    + the referenced folder IDs and, on `needsConfirmation`, the behavior changes to show
    (mirrors task 40's engine outcomes; quiet-by-default per D41's dated note).
    `createFolder(path, confirm?)` carries the confirm flag for the re-call.
- **`SyncMode` enum in `internal/domain`** (manual|watched|scheduled) — the sanctioned
  forward slice of task 40; declared once here if 40 hasn't landed first.
- **Contract methods** (`AlexandriaAPI`): `getFolderTree(): VolumeNode[]` (one call, whole
  top axis), `listCollections(): CollectionNode[]`, `createFolder(path): CreateFolderOutcome`,
  `removeFolder(id)`, `updateFolder(id, patch)` (sync_mode et al.), `pickDirectory()` (if
  still unminted by another epic — mock fake here, real dialog in 45). This **supersedes
  frontend-import's sketched `listSources`/`createSource` widening**; that epic's folder-pick
  consumes these methods instead.
- **Mock:** seeded volumes (one offline), nested folder trees with honest subtree counts,
  manual + smart collections (smart count computed against the mock catalog through the
  existing `evaluate`), collection membership backing the `inScope` collection stub
  (`mock.ts:388` finally lands), folder-add outcome simulation, fake directory picker.
- `make generate` fresh; no hand-written parallel types (lint holds).

## Out of scope

Go services / wails adapter (45), any feature UI (43/44), event catalog changes (40 owns the
rename; the rail rides existing `catalog/changed`).

## Acceptance

- Contract compiles against generated types only; mock satisfies it; `bun run dev` serves the
  full nav surface with zero backend.
- Mock parity: collection scope narrowing now real (an asset in a mock collection appears
  under that scope and nowhere else); smart counts match what the mock query returns; folder
  subtree counts sum correctly at every level.
- Outcome flows exercisable in the mock: adding a subfolder of a tracked root, a parent of
  two roots (quiet absorb), a parent over a watched root (`needsConfirmation`, then the
  confirmed re-call), an exact duplicate.
- Tests: mock membership/count invariants + outcome table.
