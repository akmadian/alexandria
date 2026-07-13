# Color space as an observation — ingest + preview custody

Half-thought from the 2026-07-12 design-system round: the frontend's color doctrine
("chrome is consistent, content is custodied") implies a small amount of backend work.
Parked here for review; nothing below is committed design.

## The doctrine this serves (frontend side, for context)

- Alexandria never grades or modifies pixels; color-critical editing is deferred to the
  user's external editor. Our whole color job is **fidelity of preview and custody of
  pixels**: never emit an untagged raster, never convert twice, never let the UI touch
  image color (no CSS filters/blends/tinted overlays on assets — selection shades the
  cell, never the photo).
- Every derived raster (thumbnail, preview) is baked by the engine to **one canonical
  space — sRGB for v1** (safe in both WKWebView and WebView2). Wide-gamut (P3) previews
  are a later upgrade; since thumbnails are derived state with a registered rebuild
  path, "re-bake everything to P3" is a rebuild, not a migration.
- Untagged source files are assumed sRGB, as stated doctrine.
- Monitor calibration is the OS's job (display ICC profile applied to correctly tagged
  content). We only have to not destroy information.

## Proposed backend residue

1. **Observation column(s):** per-asset color profile, extracted at decode time during
   ingest/enrichment — same writer class and lane as dimensions/camera metadata
   (observation, never judgment). Cheap shape: profile description string (e.g.
   "Display P3", "Adobe RGB (1998)") plus ICC profile hash for exact identity;
   `NULL` = untagged (assumed sRGB).
2. **Conversion point:** the thumbnail/preview bake already decodes the source; honor
   the embedded profile there and convert to the canonical space. The observation is
   captured at the same moment for free.
3. **Vocabulary field (later):** color space as a filterable capability — new
   vocabulary field + compiler entry per C7/impl-13 ("find everything Adobe RGB"),
   not a new query method.
4. **Open questions:** which decoders reliably surface ICC data across the supported
   file-type matrix (raw vs raster vs video vs PDF); whether video color (rec.709 /
   HLG / PQ) is stored in the same column or deferred entirely; what the assettype
   registry's "has color profile" capability row looks like.
