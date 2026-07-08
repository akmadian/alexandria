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

Alexandria's design rationale lives in `docs/`, not in code comments — start here:

- [`docs/v2/project-tracking/backend/00-START-HERE.md`](docs/v2/project-tracking/backend/00-START-HERE.md) — the current design handoff and decision log (the "why" behind the architecture)
- [`docs/v2/functional-requirements.md`](docs/v2/functional-requirements.md) — the feature backlog and roadmap, prioritized
- [`docs/coding-guidelines.md`](docs/coding-guidelines.md) — Go conventions used throughout the codebase

## Tech stack

- **Backend:** Go, SQLite (WAL mode)
- **Frontend:** React, TypeScript, Vite (package management via Bun)
- **Desktop runtime:** not yet finalized — an open decision, not a commitment
- **License:** GPL v3
