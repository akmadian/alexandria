# Requirements, Distilled

Source of truth for features: `_project-tracking/functional-requirements.md` (P0–P4). This doc is the
*compression* — the constraints that actually drive architecture. Downstream docs cite NFRs by number.

## Problem statement

A local-first, keyboard-driven catalog that indexes creative files **where they already live**,
layers fast search and triage metadata over them, and interops with Lightroom Classic via XMP —
without ever owning, moving, or modifying the files (the *reference model*).

Functional surface, compressed to verbs: **ingest** (idempotently), **watch** (converge with
external changes), **organize** (rate/tag/flag/collect/note), **search** (<200ms), **browse**
(60fps grid, loupe), **sync** (XMP bidirectional), **stay out of the way** (open-in-external-app).

## The NFRs

- **NFR-1 — Scale envelope: 500k assets, one user, one machine.** This is *small data*: the whole
  catalog is 0.5–2GB, fits on one SSD, mostly in page cache. There is no distributed problem, no
  throughput problem. The pressure is latency and coexistence. Interview trap: importing
  big-system patterns into a problem that fits in RAM.
- **NFR-2 — Latency budgets:** search <200ms @ 500k · grid 16ms/frame @ 10k+ assets · cold start
  <3s @ 100k · ingest ≥500 JPEG/min (~8.3/s) *including* hash + extract + thumbnails.
- **NFR-3 — Consistency ≫ availability, but only for one class of data.** Two durability classes:
  **irreplaceable** (ratings, tags, flags, collections, notes — hours of human judgment) vs
  **rebuildable** (thumbnails, extracted metadata, FTS, hashes — re-derivable from files).
  Crash-safety/backup paranoia applies to the first; the second needs only a rebuild path.
- **NFR-4 — The environment is semi-hostile.** LrC, Photoshop, Finder mutate the filesystem and
  XMP sidecars concurrently, forever. Drives unmount mid-scan; NAS shares vanish; files are
  caught mid-write. The system must **converge, not control**.
- **NFR-5 — Coexistence.** Never saturate CPU/disk; the user's creative apps have priority.
  Background work yields (bounded worker pools + per-tool semaphores are the physical knob).
- **NFR-6 — Zero-ops, zero-network.** No server to run, no network dependency, nothing leaves the
  machine (telemetry strictly opt-in; even helper-binary downloads require explicit consent).
  macOS/Linux first-class, Windows third.

## The essence (write this on the whiteboard first)

Alexandria is a **reconciliation engine between two-and-a-half sources of truth**:

1. The **filesystem** owns the bytes.
2. The **catalog** owns the judgment.
3. **XMP sidecars** are a *shared* half-truth co-written by Lightroom.

Nearly every hard feature — idempotent ingest, move detection, the watcher, missing-file handling,
XMP conflicts — is the same problem in different hats: *detect divergence between these stores and
converge safely*. The architecture's job is to make that boundary explicit (the data-classification
system in `03-data-model.md`) so features fall out of it instead of fighting it.

Kubernetes analogy that held up all session: judgments = `spec`, observations = `status`,
ingest/watcher = controllers, derived stores = computed views.

## User/market facts established during the session

- **Library shape:** heavy mixed media — RAW, raster, video, audio, design docs (PSD/AI/Affinity),
  fonts, LUTs. Format diversity is first-class, not a tail; multi-GB files are normal.
- **Multi-catalog is a real minority workflow** (per-client/per-job catalogs; cf. Capture One
  sessions) → supported first-class as a *self-containment* property, not a UX surface.
- **Contractor handoff** (small creator outsourcing edits) → solved by **bundle export/merge-back**
  (LrC "Export as Catalog" model), NOT by server mode.
- Stack constraints: Go engine (résumé + preference, fixed), React UI (fixed-ish), SQLite (chosen
  on merits). UI runtime (Wails/Tauri/Electron) genuinely open.
