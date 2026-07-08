# Project Tracking

Everything **in motion**: the backlog, design handoffs, specs, deferred ledgers, working notes.
Durable contributor-facing reference lives in `docs/` (deliberately lean pre-release) — tracking
docs graduate there when their area stabilizes (rule in the master head).

**Start at [`00-START-HERE.md`](00-START-HERE.md)** — the master head of the implementation task
tree: what's next right now, the dependency tree below it, and status at a glance.

Layout:

- [`CONSTANTS.md`](CONSTANTS.md) — cross-cutting load-bearing invariants (C1–C14)
- [`functional-requirements.md`](functional-requirements.md) — the feature backlog (P0–P4)
- `backend/` · `seam/` · `frontend/` — one subdir per area, each owning its own
  `00-START-HERE.md` tracker with the real status and rationale
- `design/` — designs written ahead of their milestone (CI/hygiene, release, telemetry,
  local AI, RAW export dispatch, testing strategy, CONTRIBUTING outline)
- `ops/` · `perf/` — repo-setup and performance working references
- `_scratch/` — raw notes (system-design scratch, file-type lists)

Add an area's subdir when it actually starts accumulating milestones — don't pre-create
empty ones.
