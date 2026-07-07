# impl/07 — Dependency Fleet (External-Tool Supervisor)

**Status: design complete; needed before deep metadata (exiftool), RAW/video/PDF thumbnails
(dcraw_emu/ffmpeg/ghostscript), and impl/06. Not needed for the ingest milestone.**
**Scope:** new `internal/dependency`. **References:** D4, D5.

Naming note: the package is `dependency` (user's call — same vocabulary as service dependencies
at Amazon: external things we rely on, discover, version-check, and survive the absence of).

## Public surface (keep it this small)

```go
func Run(ctx, tool ToolID, op OpID, args []string, opts ...RunOpt) (stdout []byte, err error)
func Status(tool ToolID) ToolStatus   // found|missing|wrong-version + path + version
func Fetch(ctx, tool ToolID, consent ConsentToken) error  // user-consented download
```

Callers never touch os/exec. **Not pluggable beyond the descriptor table**: extension = a new
descriptor row; new *mechanics* (daemon protocols) = a second concrete code path, never a framework.

## Descriptors

Per tool: binary name(s), version-check invocation + min version, per-platform/arch acquisition
(URL, pinned SHA-256, archive layout), invocation conventions (JSON output flags: `exiftool -j`,
`ffprobe -print_format json`), daemon capability. V1 tools: exiftool, ffmpeg+ffprobe, ghostscript,
dcraw_emu/libraw (per FR thumbnail table).

## Discovery

PATH → app-data tools dir → machine.json override. Cache resolution + verified version. Missing
tool = feature-level graceful degradation (callers check Status; pure-Go path keeps the app useful
with zero tools).

## Acquisition (NFR-6 — never silent)

Detect → in-app prompt with size/purpose ("download ffmpeg 7.x, ~80MB, enables video thumbnails")
→ download → **verify pinned checksum** → macOS: strip `com.apple.quarantine` xattr → place in
app-data. First launch may ASK, never act. No auto-update of tools without the same consent flow.

## Execution policy

Timeout = f(tool, op, fileSize): base + per-GB scaling, per-file-type override capable (registry).
Always exec with argv arrays (never shell strings). Always pass self-limiting flags where they
exist (`ffmpeg -timelimit`). Capture stderr into error records (reason codes for the DLQ).

## Concurrency (NFR-5's physical knob)

One semaphore per tool; sizes from machine.json (defaults: ffmpeg=2, exiftool-oneshot=4,
ghostscript=2). These + pipeline pool sizes are ONE user-facing config surface later
("performance/lightweight" presets, P2).

## Daemon lifecycle (exiftool `-stay_open`)

Spawn with stdin pipe (parent death → EOF → exiftool exits: self-cleaning on crash) · request/
response over stdin with `-execute` markers · health check, restart-on-crash with backoff ·
graceful shutdown on app exit. Start with ONE daemon; a small pool only if measured contention
demands (ponytail ceiling comment it).

## Orphan management (layered, D5)

1. Daemons: stdin-EOF convention (free).
2. One-shots: short timeouts + tool self-limits; an orphan finishes one file and exits.
3. Linux: `SysProcAttr.Pdeathsig = SIGKILL`. Windows: Job Object, kill-on-close.
4. NO startup reaping (rejected — layers 1-3 suffice).

Per-OS specifics in build-tagged files INSIDE this package (`spawn_linux.go` etc.) — no shared
platform package (D19).

## Acceptance

- Kill the parent hard (SIGKILL) with a daemon running → daemon exits within seconds (stdin EOF).
- Timeout on a wedged fake tool (sleep binary) → killed, error row with reason `tool_timeout`.
- Missing tool → Status reports, caller degrades, no error spam.
- Checksum-mismatch download → rejected, nothing placed, clear error.
- Concurrent Run calls beyond semaphore → queued not spawned (assert max live PIDs).
- exiftool daemon: 1k sequential metadata reads ≥ 100/s (daemon amortization works).
