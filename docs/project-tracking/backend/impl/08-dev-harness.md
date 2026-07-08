# impl/08 — Dev Harness (`cmd/dev`)

**Status: core DONE with impl/04 (2026-07-07); observability server deferred.** Built:
`cmd/dev` with `import` / `reconcile` / `errors` / `sessions` / `rebuild fts`; `--catalog
<dir|:memory:>` (prints the browsable `catalog.db` path); `--debug [--debug-addr]` mounting stdlib
**pprof + expvar** at `localhost:6060`; `--hash-workers` / `--extract-workers` / `--thumb-workers`
(the CPU-bound pools default to `NumCPU*3/4`); debug-level logging on by default. It replaced the
retired `internal/main.go`.

Deferred (build when a real workload needs them): the richer `--debug` surface from the table below —
**statsviz**, the custom **`/state`** JSON snapshot, and the `go:embed` HTML dashboard — plus
`--json` output. The stdlib pprof goroutine dump already gives the live pipeline picture, which is
what impl/04 acceptance needs.

## What it is

A runnable CLI inside the module (`cmd/dev/main.go`), with **full access to `internal/*`** — the
frontend seam constrains the product, not the toolbox. It exercises the engine end-to-end, exposes
its state, and doubles as the manual rig for impl acceptance criteria.

## Subcommands (stdlib `flag`, one file to start; grow with milestones)

```
dev import <path>      --catalog <dir|:memory:> --hash-workers N --thumb-workers N --batch-size N --json
dev reconcile          (walk mode over a registered source)
dev errors             (dump the DLQ: path, stage, reason_code, attempts)
dev sessions           (import_sessions history incl. per-extension skip tallies)
dev rebuild fts|thumbs (derived-store rebuild functions)
dev watch              (arrives with impl/05 — live hint/debounce/verdict feed)
dev deps status        (arrives with impl/07)
```

## Observability: `--debug <addr>` mounts a debug HTTP server

| Mount | Source | Gives |
|---|---|---|
| `/debug/pprof/*` | stdlib `net/http/pprof` | goroutine dump = live pipeline picture (who's blocked on which channel), heap/CPU/block profiles |
| `/debug/vars` | stdlib `expvar` | channel fill per stage, per-stage counters, batch commits, dirty-set size |
| `/debug/statsviz` | `github.com/arl/statsviz` (only new dep, tiny) | live browser charts: heap, GC, goroutines, scheduler |
| `/state` | custom | JSON snapshot of domain state: pipeline stage stats, current session counts, watcher unit modes, DLQ tail |
| `/` | one `go:embed`-ded HTML page | polls `/state` + `/debug/vars`, renders the dashboard. ~150 lines, no framework |

**Placement rule (keeps the engine clean):** expvar publishes and a `Stats()` snapshot method are
legitimate *engine* code — cheap, and the `Stats()` shape is the same data the status bar and P3
health panel will need (dev omniscience doubling as product surface). The mounts, the HTML page,
and anything pprof/statsviz live in the harness's debug server package, imported ONLY by `cmd/dev`.

**Catalog state needs no code:** it's a SQLite file — document in the harness help text that
`sqlite3`, Datasette, or DB Browser give a full GUI over judgments/observations/DLQ for free.

Rejected: Prometheus/Grafana, OpenTelemetry (ops stacks for a laptop process), TUI frameworks,
config files (flags only).

## Acceptance

- `dev import` over the fixture tree prints determinate progress, throughput (files/s), and a
  session summary matching the DB row.
- With `--debug`, mid-import goroutine dump shows the expected stage topology; `/state` reflects
  live counts; killing the import mid-run leaves the fully-processed invariant intact (impl/04's
  sacred test, run manually here).
- `dev errors` after a corrupt-fixture import shows the reason-coded rows.
