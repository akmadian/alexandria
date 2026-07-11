# Thumbnailing performance & hardware acceleration

**Audience:** Ari + future Claudes. This is the reference for *why the ingest
pipeline spends its time where it does*, what "hardware acceleration" actually
means for this workload, and the architecture that lets us go fast without a
per-format × per-platform code explosion. Written after the impl/04 pipeline
shipped and we tuned it on a real folder.

References: **D4** (subprocess decoders, never cgo), **D5** (the `dependency`
package / external-tool supervisor), **impl/07** (the dependency fleet).

## TL;DR

- The only pipeline stage worth accelerating is **THUMB**, and within it, the
  **decode**. Everything else is I/O- or DB-bound.
- The real win is not the GPU. It's **not decoding the whole image**:
  *shrink-on-load* (JPEG DCT downscale) and, for RAW, *extracting the embedded
  JPEG preview* instead of demosaicing.
- We don't do shrink-on-load today because **Go's stdlib JPEG decoder has no API
  for it**. It arrives via a native subprocess (libvips) in impl/07.
- Avoid per-format accel code by reducing every exotic format to "produce
  decodable pixels" (a tiny per-format step) and funnelling all of it through the
  one shared **raster** thumbnailer. Acceleration is a property of the *tool* we
  shell out to, not of our code.

## Current state (baseline)

Measured on an 8-core Apple Silicon Mac over `June Tide Pooling` (1330 photos):

| Change | Result |
|---|---|
| Default pools (`thumb=2`), `draw.CatmullRom` resize | ~7 files/s, ~2 cores busy |
| `draw.ApproxBiLinear` resize + `--thumb-workers 8` | ~26 files/s, ~5 cores, 415MB RSS |

What the code does today (all pure-Go, zero external tools — the always-available
baseline, an NFR):

- **Decode:** `image.Decode` (stdlib) — a *full* decode into an RGBA buffer. A
  24MP JPEG → ~96MB buffer. This is the CPU and memory cost.
- **Resize:** `resizeKernel.Scale` in `internal/thumbnailer/raster.go`, currently
  `draw.ApproxBiLinear` (fast; dial up to `BiLinear`/`CatmullRom` for sharper).
- **Encode:** `jpeg.Encode` at quality 80. Cheap (output is 512px).
- **Metadata:** `internal/metadata/raster.go` uses `image.DecodeConfig`
  (header-only, no pixels) + an EXIF scan. Already cheap — **we do not
  double-decode.**
- **Pool tuning:** the dev harness defaults EXTRACT/THUMB to `NumCPU*3/4` (leaves
  headroom; can't lock the machine — the OS preempts and `GOMAXPROCS` caps Go at
  NumCPU). Engine default stays a conservative `2`; real per-host tuning is
  machine.json, later.

The `image` → `raster` rename (`GenerateRaster`, `ExtractRaster`) makes explicit
that this processor is for standard decodable formats (JPEG/PNG/GIF), *not* RAW.

## Where time actually goes (the whole pipeline)

| Stage | Bound by | Accelerable? |
|---|---|---|
| SCAN | directory walk (I/O) | no |
| HASH | read 64KB + xxhash | no — xxhash is memory-bandwidth bound already |
| MATCH | 2 indexed SQLite reads (singleton) | no |
| EXTRACT | `DecodeConfig` header + EXIF | no — already header-only |
| **THUMB** | **full decode → resize → encode** | **yes — the entire story** |
| WRITE | batched SQLite commit (singleton, disk) | no |

So the acceleration conversation is *only* THUMB, and mostly its decode.

## The three meanings of "hardware acceleration"

1. **SIMD (CPU vector units).** Not a GPU. NEON (Apple Silicon) / AVX (x86)
   process 8–16 pixels per instruction. Pure-Go `image/jpeg` is *scalar*;
   libjpeg-turbo is SIMD and ~2–6× faster at the same decode. Biggest realistic
   win, no GPU needed.
2. **Fixed-function media blocks (ASICs).** Dedicated silicon for specific codecs
   — Apple Silicon's hardware JPEG/HEIC decoder, Intel Quick Sync, NVIDIA
   NVDEC/NVENC. The CPU fires a DMA and gets a decoded buffer back at near-zero
   CPU cost. Reached via OS frameworks: **ImageIO/VideoToolbox** (macOS), Media
   Foundation (Windows), VA-API (Linux).
3. **GPU compute (Metal/CUDA shaders).** General parallel compute; good for
   resize/filter on *large* buffers in bulk, but you pay a transfer + launch tax.
   For 512px thumbnails that overhead usually exceeds the savings. Apple Silicon's
   unified memory softens the transfer cost but it's still not worth it for
   thumbnails.

## The key technique: don't decode the whole image

This matters more than any specific chip.

### Shrink-on-load (JPEG DCT downscale)

A JPEG is stored as 8×8 DCT blocks. An inverse DCT can emit only the low-frequency
1×1, 2×2, or 4×4 samples per block, yielding a 1/8, 1/4, or 1/2 image *as a
byproduct of decoding* — skipping up to 7/8 of the work **and** 7/8 of the memory.
libjpeg exposes `scale_num/scale_denom`; libvips calls it shrink-on-load.
Decoding a 6000px source straight to ~750px is ~5–10× less CPU and ~8× less RAM
than "decode full, then resize" (what we do now). The 415MB RSS would largely
disappear.

### RAW embedded previews

When RAW support lands: **don't demosaic for a thumbnail.** Nearly every RAW file
(CR2/CR3/NEF/ARW/DNG…) embeds a full-resolution JPEG preview the camera made.
`exiftool`/`dcraw` extract it directly, so "RAW thumbnail" becomes "extract an
embedded JPEG" — nearly free, and it reuses the raster backend (below). This is
the single biggest practical win for RAW-heavy libraries.

### Why we're NOT doing shrink-on-load today

Go's stdlib `image/jpeg` has **no shrink-on-load API** — it only does a full
decode. There is no mature pure-Go JPEG decoder with DCT scaling. So shrink-on-load
requires either **cgo** (libjpeg-turbo/govips — rejected by D4) or a **subprocess**
(libvips). We chose the pure-Go full decode as the zero-dependency baseline (works
with no tools installed), and shrink-on-load rides in with the subprocess fleet in
impl/07. It's a deliberate consequence of pure-Go-first, not an oversight.

## Avoiding the per-format × per-platform explosion

The trap: writing accelerated thumbnail code for each file type on each OS →
`N formats × M platforms` of native code. Four ideas keep it bounded:

1. **Reduce every format to "produce decodable pixels," then one shared
   thumbnailer.** Each exotic format's only job is a small "get me pixels" step —
   RAW extracts its embedded preview, video grabs a frame, PDF rasterizes a page,
   vector renders — and all of them hand bytes to the **single** raster backend
   (`GenerateRaster`) for resize + encode. The per-format code is tiny; the heavy,
   shared path is written once. (This is exactly the RAW-preview idea,
   generalized.)
2. **Acceleration is a property of the tool, not our code.** We never write
   ImageIO/Metal/VA-API. We pick tools that are *already* accelerated on their
   platform and let them carry it: **libvips** (libjpeg-turbo + shrink-on-load,
   cross-platform), macOS **`sips`** (ImageIO → hardware JPEG/HEIC decode),
   **ffmpeg** (VideoToolbox/VA-API/NVENC when present). "Per-platform
   acceleration" collapses into the dependency package's per-platform *tool
   discovery* — which we build once regardless (D5).
3. **The capability interface stays format-agnostic.** `assettype.Handler.Thumb`
   is just a `GenFunc`. Whether it's pure-Go `GenerateRaster`, a `vips` subprocess,
   or "extract preview → `GenerateRaster`" is an implementation detail behind one
   interface. Adding a format = adding a registry row that points `Thumb` at one
   of a *small, bounded* set of strategies, not new accel code.
4. **A small set of strategies, not a matrix.** The whole space is roughly:
   `pure-Go raster` · `vips-thumbnail` (covers most formats in one tool) ·
   `extract-embedded-preview → raster` · `video-frame → raster` · `render → raster`.
   That's ~5 strategies total, each format picks one. Never `N×M`.

Net: the pure-Go raster path is the fallback; a single multi-format accelerated
tool (vips) or the platform tool (via the fleet descriptor) is the fast path; and
the number of things we actually write stays tiny.

## Fleet supervision (D5 / impl/07) — daemon vs one-shot

Two different subprocess lifecycles, chosen by the tool's startup cost:

- **exiftool → daemon.** exiftool is Perl; process startup is ~100ms, far more
  than a per-file extraction. So the fleet keeps **one long-lived** process
  (`-stay_open True -@ argfile`), feeds requests over stdin, and reads results
  back. The fleet owns its lifecycle: spawn on first use, stdin-EOF to shut down,
  `pdeathsig` (Linux) / Job Object (Windows) so it dies with the parent.

- **libvips / ffmpeg → one-shot with a self-timeout.** Important correction:
  **libvips is a library and `vips` the CLI is one-shot** — there is no
  `stay_open` daemon mode like exiftool. But `vips` is C with ~ms startup, so
  one process per file is fine; the spawn cost is negligible next to decode.
  The fleet runs it as a one-shot with `timeout = f(tool, operation, file size)`
  and kills a runaway (a malformed file that hangs the decoder). If per-spawn
  overhead ever shows at very high throughput, the options are (a) a tiny
  long-lived "thumbnail worker" wrapper we own, or (b) accept it — but do not
  reach for that until a benchmark demands it.

- **Crash isolation is the whole point (D4).** Decoders parsing untrusted files
  *will* segfault on a crafted input. As a subprocess, that death is one per-file
  error (a DLQ row), not an app crash. `kill -9` always works; a runaway can't
  take the engine down.

- **Concurrency knob = per-tool semaphore, not our goroutine pool.** In
  subprocess mode the fleet caps *how many `vips` processes run at once* (a
  semaphore, NFR-5's physical throttle). THUMB then acquires a fleet slot instead
  of relying only on the `--thumb-workers` goroutine pool. Same idea (bounded
  parallelism), moved to the process level so we don't fork 200 `vips` at once.

- **Discovery & distribution (D5).** Descriptor per tool (identity, version
  constraints, per-platform acquisition, invocation conventions) → discovery
  (PATH → app-data → user override) → *user-consented* download with a pinned
  checksum (never silent, NFR-6; strip macOS quarantine xattr). The app must stay
  useful with **zero** tools installed — that's why the pure-Go raster path is
  permanent, not a stopgap.

## GPU verdict

Still not worth it for thumbnails. The win is in the *decode* (fixed-function
block or SIMD), not the resize the GPU would accelerate — and shrink-on-load makes
the resize trivial anyway. Even Apple Silicon's unified memory (no copy cost)
doesn't change the math for 512px outputs. GPU earns its place only for a
*different* workload: large preview rendering, batch color/filter ops, or video
scrubbing — sustained work on big buffers where the transfer amortizes. If those
features land, revisit; for ingest thumbnailing, no GPU code.

## Roadmap for this hotspot

1. **Done:** pure-Go baseline; `ApproxBiLinear` resize; harness pool tuning;
   confirmed no double-decode; `image`→`raster` rename.
2. **impl/07:** `dependency` package + fleet; a `vips`-backed `GenerateRaster`
   alternative selected when the tool is present (pure-Go stays the fallback);
   this brings shrink-on-load for free.
3. **RAW milestone:** RAW handler extracts the embedded JPEG preview (exiftool)
   and feeds it to the raster backend — no demosaic for thumbnails.
4. **Only if measured:** hardware-decode path (macOS `sips`/ImageIO) if
   libjpeg-turbo-via-vips isn't enough; a persistent thumbnail-worker if per-spawn
   overhead ever dominates. Do not build ahead of a benchmark.

The likely end state: with decode cheap, THUMB stops being CPU-bound and the
pipeline becomes **I/O-bound** (reading files off disk) — a good problem, with a
much higher ceiling than today's 26/s.
