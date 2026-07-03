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

Pre-implementation. Architecture and requirements fully defined. See `/docs` for comprehensive design documentation.

## Documentation

| Document | Contents |
|---|---|
| [Vision & Requirements](docs/01-vision-and-requirements.md) | What we're building, target user, competitive landscape |
| [Architecture Overview](docs/02-architecture.md) | Core architectural decisions and rationale |
| [Database Schema](docs/03-schema.md) | Full SQLite schema with field-level documentation |
| [Domain Model & Interfaces](docs/04-domain-model.md) | Go package structure, domain types, platform interfaces |
| [Ingest Pipeline](docs/05-ingest-pipeline.md) | Import flow, pipeline stages, idempotency, error handling |
| [Watcher Service](docs/06-watcher-service.md) | File watching, network polling, volume monitoring |
| [XMP Sync](docs/07-xmp-sync.md) | Lightroom interop, XMP read/write, conflict resolution |
| [State & Commands](docs/08-state-and-commands.md) | Undo/redo, command pattern, backend state management |
| [Startup Sequence](docs/09-startup-sequence.md) | App initialisation order, migration, graceful degradation |
| [Error Handling](docs/10-error-handling.md) | Error tiers, typed errors, logging strategy |
| [Testing Strategy](docs/11-testing-strategy.md) | Test philosophy, table-driven tests, testutil patterns |
| [Schema Migrations](docs/12-migrations.md) | Migration system, schema evolution principles |
| [Keybindings](docs/13-keybindings.md) | Platform-aware shortcuts, conflict detection, configurability |
| [Deferred Features](docs/14-deferred.md) | P1 and future features with rationale for deferral |

## Tech stack

- **Language:** Go
- **Desktop framework:** Wails v2
- **Database:** SQLite (via WAL mode)
- **License:** GPL v3
