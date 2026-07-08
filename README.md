# Alexandria

A local-first, cross-platform Digital Asset Manager for creative professionals.

Alexandria is named after the Library of Alexandria — the ancient world's greatest archive of human knowledge. It is also, appropriately, a reminder of why backups matter.

## What it is

Alexandria is a catalog-first reference DAM. It indexes your creative assets wherever they live — local drives, external drives, network shares — and gives you a single, fast, beautiful place to browse, search, tag, and organise them. It does not move or copy your files. It does not require cloud storage. It works when your NAS is offline.

It is designed for solo creative professionals: photographers, designers, videographers, and anyone who has ever lost an hour searching for a file they know they have somewhere.

## What it is not

- A photo editor
- A cloud sync tool
- A team collaboration platform
- A managed library (it does not own your files)

## Status

Active development, pre-release. **Not currently accepting external contributions.**

## Documentation

Design rationale lives in documents, not code comments. Two trees, split by durability:

- [`_project-tracking/`](_project-tracking/00-START-HERE.md) — **everything in motion**: the master
  task-tree head (start there), the feature backlog, design handoffs and decision logs per area
  (`backend/`, `seam/`, `frontend/`), designs awaiting their milestone (`design/`), repo/perf
  working references (`ops/`, `perf/`), and scratch (`_scratch/`).
- [`docs/`](docs/) — **durable reference** for someone new to the repo. Deliberately lean while
  pre-release: coding guidelines today; tracking docs graduate here as areas stabilize.

## Tech stack

- **Backend:** Go, SQLite (WAL mode)
- **Frontend:** React, TypeScript, Vite (package management via Bun)
- **Desktop runtime:** Wails v2
- **License:** GPL v3
