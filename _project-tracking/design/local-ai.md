# Local AI — semantic search & auto-tagging

Not a priority right now — no code, no timeline. Written so the design
exists and is tracked before it's needed, same spirit as
[telemetry.md](telemetry.md)/[release.md](release.md). Local-only by design:
no cloud inference, ever, for this feature — personal photo libraries are
sensitive data, and this mirrors the opt-in/local-only stance already taken
on telemetry.

References: `_project-tracking/perf/thumbnailing-and-hardware-acceleration.md` (D4,
D5 — cited heavily below, this feature extends the same reasoning rather
than inventing new architecture), `_project-tracking/backend/impl/07-*`
(the dependency fleet this would plug into).

## What it is

- **Semantic search** — embed each photo (and a text query) into the same
  vector space, rank by similarity. Not keyword/tag search, "find photos
  like this description."
- **Auto-tagging** — a vision-captioning model suggests tags/captions at
  ingest time. **Suggestions only, never auto-committed** — wrong auto-tags
  on someone's real library erodes trust fast; human confirms via the
  existing tag system.

## Models

- **Embeddings: [MobileCLIP2](https://github.com/apple/ml-mobileclip)**
  (Apple, open-sourced). 50-150M params depending on variant, 3-15ms
  inference latency, purpose-built for on-device/private use — not a
  general CLIP checkpoint pressed into service, this is the thing designed
  for exactly this job. Start with MobileCLIP2-S0 or S2.
  [SigLIP2](https://huggingface.co/blog/siglip2) is the credible fallback
  if MobileCLIP2's tooling support proves thinner in practice.
- **Captioning/tagging: Florence-2** (Microsoft, 0.23B/0.77B, MIT-licensed,
  does captioning + detection + OCR in one small model) or **moondream**
  (purpose-built small edge VLM). Either is realistic on consumer hardware.

## Storage: `sqlite-vec`

[`sqlite-vec`](https://github.com/asg017/sqlite-vec) (actively maintained,
Mozilla-backed) — a loadable SQLite extension adding vector KNN search
directly inside SQLite (`CREATE VIRTUAL TABLE ... USING vec0(embedding
float[512])`, query via plain SQL). No separate vector database — stays
inside the same single SQLite file `sqlite`/D3 already owns. Consistent
with D3's "exactly one transactional store" rule; this is a derived/index
structure on top of it, not a second store.

## The real architectural question: cgo vs. subprocess

**This is the part that needs a decision before any code gets written**,
because it directly collides with D4.

The obvious Go binding (`yalue/onnxruntime_go`) is **cgo** — it wraps the
ONNX Runtime C library in-process. D4 rejected exactly this shape for
format decoding, for a reason that applies identically here: a cgo-linked
crash takes down the whole app process, where a subprocess crash is a
one-row error. Vision-model preprocessing on a malformed/adversarial image
is not a hypothetical crash surface, same class of risk D4 was written to
rule out.

**Proposed resolution, extending D4/D5 rather than reopening them:** treat
ONNX inference exactly like `vips`/`exiftool` — a small satellite helper
binary (which may use cgo *internally*, that's fine, it isn't the main
binary) that the `dependency` fleet supervises as a subprocess. This also
resolves the "don't bloat the dependency package" concern raised when this
was discussed: **the main Go module gains zero new packages** — no cgo
binding in `go.mod` at all, just two more fleet descriptor rows pointing at
a downloadable helper executable + model weights, identical shape to how
`exiftool` is already handled.

**Daemon vs. one-shot** (per D5's existing distinction): model *load* time
is the deciding factor, and MobileCLIP2's weights loading into memory
almost certainly costs far more than the ~100ms threshold that put
`exiftool` in daemon mode (vs. `vips`'s ms-level one-shot). This wants the
**exiftool-style daemon pattern** — load once, feed images repeatedly over
the fleet's existing stdin-daemon convention, not a fresh process per photo.

## Cross-platform packaging

Less wrangling than it sounds, and it splits cleanly:

- **The model file is platform-agnostic** — one `.onnx` file per model
  size, same bytes on macOS/Linux/Windows, arm64/x86_64. Doesn't multiply.
- **The runtime (ONNX Runtime's shared library) is per-OS/arch** — same
  shape of problem as `vips`/`ffmpeg` today (`libonnxruntime.dylib`,
  `.so`, `.dll`, each in arm64/x86_64 variants). `yalue/onnxruntime_go`
  dynamically loads the shared lib from a path at runtime, so this drops
  straight into the fleet's existing PATH → app-data → user-override
  discovery, user-consented download with pinned checksum, macOS quarantine
  xattr strip (D5, NFR-6) — no new mechanism needed.
- **Execution providers (CoreML/DirectML) would multiply the matrix** if
  chased — per D5's discovery model and the thumbnailing doc's "small set
  of strategies, not a matrix" principle, start CPU-EP-only. Revisit only
  if inference speed is a measured problem, not preemptively.

## Fleet integration (concrete descriptor additions, D5-shaped)

Two new rows in the dependency descriptor table, nothing new in the
abstraction itself:

1. **ONNX Runtime shared library** — per-platform/arch acquisition, same
   discovery/download/checksum/quarantine treatment as `vips`.
2. **Model weights** (MobileCLIP2 + Florence-2/moondream `.onnx` files) —
   platform-agnostic, still gated behind user-consented download with
   pinned checksum (NFR-6: never silent). Also supports **explicit
   user-specified path** as a discovery tier — same "user override" slot
   the fleet already has, letting someone point at a model file they
   already have instead of downloading.

**App must stay useful with zero tools installed** (D5's existing rule,
applies unchanged): with no runtime/model present, semantic search and
auto-tagging simply don't appear as features — no error state, no degraded
mode, just absent.

## Open questions

- Exact daemon wire protocol (stdin/argfile like exiftool, or a local Unix
  socket/named pipe) — needs a decision, not obviously exiftool's exact
  shape given image bytes (not text) cross the boundary.
- Which MobileCLIP2 variant ships as the recommended default (S0 vs S2 —
  size/speed/accuracy tradeoff) — needs an actual benchmark on realistic
  hardware, not a guess.
- Whether Florence-2 or moondream wins for tagging — needs a side-by-side
  on real photo fixtures, not decided from specs alone.
- Windows arm64 — worth supporting day one or deferred like other
  lower-priority platform variants elsewhere in the fleet.

## Explicitly out of scope

- **Cloud inference of any kind** — not a fallback, not an option, by
  design (see intro).
- **GPU/execution-provider acceleration** — CPU-EP-only until a measured
  need says otherwise.
- **Auto-committing tags without human confirmation** — suggestion-only is
  not a v1-vs-later distinction, it's permanent.
- **Building any of this now** — this doc exists purely so the design isn't
  lost before it's picked up.
