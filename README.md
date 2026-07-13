# Alexandria

A local-first, cross-platform Digital Asset Manager for creative professionals.

Alexandria is named after the Library of Alexandria — the ancient world's greatest archive of human knowledge. It is also, appropriately, a reminder of why backups matter.

## Ethical Tenets
- Alexandria is free, in terms of both freedom and money (if you choose), forever. Features will never be paywalled.
- Your work is yours. Alexandria does not collect your data, it does not train AI models on your work. Files remain in your posession ONLY. No walled gardens where you lose your work if you stop paying.
- Telemetry is inherently opt-in and always transparent. Alexandria can be completely cut off from the internet, and you will notice no difference. Your data (should you choose to share it) guides our roadmap - we don't want to waste time building features you don't need or want.


## Features


### What We Do Not Support, And Why

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


## Setup

Dev dependencies:
```
brew install go golangci-lint
```

Run all checks (same as CI):
```
make check
```