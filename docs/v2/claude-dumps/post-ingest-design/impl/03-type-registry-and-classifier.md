# impl/03 — Unified Type Registry + Magic-Byte Classifier (Blocker 3)

**Scope:** new `internal/filetype` (or evolve `internal/domain/filetype.go`), touch
`internal/metadata`, `internal/thumbnailer`. **Blocked by:** nothing (parallel with 01/02).
**Blocks:** impl/04. **References:** D6, D7.

## 1. The unified registry

Today three parallel maps can drift: `domain.supportedFileTypes` (ext→MIME/type),
`metadata.Default()` (MIME→extractor), `thumbnailer.New()` (MIME→generator). Fold into ONE
explicit table — the single place a format is added:

```go
type TypeHandler struct {
    Ext      string             // canonical dispatch key, lowercase, no dot
    MIME     string             // attribute for the seam/webview, NOT a dispatch key
    Type     domain.FileType
    Metadata metadata.ExtractFunc   // nil = no capability → skip gracefully
    Thumb    thumbnailer.GenFunc    // nil = generic card
    // Carved when their features ship (rule of two): Preview, Grouping.
}

var registry = []TypeHandler{ /* every supported format, one literal each */ }
```

Explicit central table; NO `init()` self-registration. Lookup: `Classify(ext) (TypeHandler, bool)`.
`metadata.Registry`/`thumbnailer.Registry` keep their packages (decode logic lives there) but their
per-MIME maps disappear — dispatch is "the TypeHandler row you already hold."

Sidecar extensions get their own small table in the same file: `{"xmp", "aae", "thm", "lrv",
"pp3", "dop", "on1"}` → scanner routes these to the sidecar path (v1 PARSES only xmp; tracking the
rest is free).

## 2. Magic-byte classifier (~50–80 lines, no dependency)

```go
// Sniff reports the canonical type family for a file head (≥ a few KB of the
// 64KB the hash stage already read). ok=false → unrecognized content.
func Sniff(head []byte) (family ContentFamily, ok bool)
```

Table entries (offset, bytes) for our closed set: JPEG `FFD8FF`, PNG, GIF, TIFF LE/BE (covers
TIFF-family RAWs: CR2/NEF/ARW — family only, extension picks dialect), BMP, WebP (`RIFF`@0 +
`WEBP`@8), HEIC/MP4/MOV family (`ftyp`@4 + brand), PDF `%PDF`, PSD `8BPS`, SVG/XML (text probe),
MP3 (ID3 or frame sync), FLAC, WAV (`RIFF`+`WAVE`), MKV (EBML), AI/EPS (`%!PS` / `%PDF` variants).

**Policy (D7):** extension classifies provisionally at scan (no I/O — the skip gate needs it
pre-read). After the hash read, sniff the same buffer:
- agreement → proceed (99.9%)
- extension says X, content says Y → **trust content for the family**, badge the asset
  (`extension_mismatch` marker in extended metadata + import_errors reason `ext_mismatch` at
  severity info), classify as Y's family
- no extension → sniff-only
- sniff says "not even the claimed container" AND nothing else recognizable AND zero/garbage
  content → bouncer rejection: import_errors row (`no_usable_content`), NO identity minted (D13)

## Acceptance
- Adding a format = exactly one table row (prove with a test that registers a fake format and
  round-trips classify→extract-nil→thumb-nil gracefully).
- Sniff tests: golden headers for each family; WebP vs generic RIFF; ftyp brand discrimination;
  truncated-header behavior (ok=false, no panic).
- Renamed-file test: PNG bytes named `.jpg` → classified image family, mismatch badge recorded.
