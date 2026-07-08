# Project Tracking — index

One subdir per area. Each area owns its own tracking doc (a `00-START-HERE.md` or
equivalent) with the real status table and "what's next" — this file just points
to them, it does not duplicate their content.

| Area | Status | Tracker |
|---|---|---|
| Backend | Active — impl/06 (XMP sync) in progress | [`backend/00-START-HERE.md`](backend/00-START-HERE.md) |
| Frontend | Not started (blocked on Wails/Tauri/Electron decision) | — |
| Ops | No active milestone tracking yet (see `../ops/` for reference docs) | — |
| Perf | No active milestone tracking yet (see `../perf/` for reference docs) | — |
| Testing | No active milestone tracking yet (see `../test/` for reference docs) | — |

Add an area's subdir (with its own tracker) when it actually starts accumulating
milestones — don't pre-create empty ones.
