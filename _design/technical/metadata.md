# Metadata: the facet model

Metadata modeling round, 2026-09-16/17. Replaces the session-minted flat
`FileMetadata` (never ratified; field list carried from the dead repo —
struck). Register: everything below marked RATIFIED is Ari's ruling from
this round; the build shipped 2026-09-17 with a cold design review and a
round review, findings dispositioned on his word.

## The model (RATIFIED)

**Facet composition.** `FileMetadata` is composed of optional facets —
sections by *overlapping nature*, never by file kind: `visual`, `timing`,
`capture`, `media`, `audio`, `authorship`, `location`. A field is declared
exactly once, in the most general facet whose semantics hold (Spotlight's
sharing rule; grounded against kMDItem*, digiKam's schema, exiv2, Fowler's
inheritance mappings, EAV's failure record).

- **Kind routes the extractor; EVIDENCE decides facet presence.** A silent
  screen recording has no `audio` facet; a scan has no camera fields. An
  all-nil facet normalizes to nil — `{}` never lands in a blob.
- **One meaning per field.** Dimensions are ENCODED width/height + the 1–8
  EXIF orientation code for images AND video (the industry storage
  convention); display size derives, never the reverse.
- **`capture` admission** = the file states facts about the creation
  event. `captured_at` alone admits; camera identity is optional within
  the facet — so screen recordings keep a real capture time and the
  capture sort never falls back to copy-time mtime for them.
- **Stream sound props** (`sample_rate`, `channel_count`) live in `media`
  beside `frame_rate`; `audio` is the tagged-audio facet (album, track,
  genre, composer) — named `audio`, not "music": not all audio is music.
- **`authorship` is dual-source, no normalization**: TIFF and IPTC
  variants stored side by side (LrC's model), plus `tiff_software`. No
  precedence is minted anywhere; a normalized layer belongs to the parked
  source-normalization round.
- **Expectation parity** (RATIFIED 2026-09-17): a field both LrC's panel
  and digiKam surface needs a named reason to be absent. This admitted
  exposure_bias/program, metering_mode, white_balance, serial, 35mm-eq.

## Storage (RATIFIED)

- One JSON blob per file (`files.metadata`, per database.md), sectioned by
  facet key. **Every key — facet and field — is an explicit CodingKeys
  spelling and is FROZEN storage contract**: generated columns read them
  by path (`capture_sort` → `$.capture.captured_at`). The keys double as
  stable field identifiers (`capture.iso`) for future display config.
- **Blob dates use `catalogDateFormatter`** (ISO 8601, ms, 'Z') — one
  timestamp form across catalog columns and blob, one sort scale.
  `SubSecTimeOriginal` folds into `captured_at`: burst order is real.
  Encoder and decoder move together, always.
- **`captured_at` is wall-clock, UTC-labelled**; the zone offset is its
  own field. Video parses `com.apple.quicktime.creationdate` to the same
  semantics — photos and clips shot in the same minute sort together.
- **`files.metadata_version`** = the roster version whose extractor
  LOOKED at the file (0 = never attempted); the re-extraction worklist
  key. The blob's NULL-ness carries the yield.
- **The long tail is never stored.** The catalog stores the queryable/
  displayable surface only; a future on-demand exiftool lane answers the
  deep view (both major DAM comparables work this way). Import never
  stores the arbitrary tag dump — first-class facet fields may come from
  any reader, exiftool included (re-narrowed by Ari 2026-09-18; the
  earlier "no exiftool in import, ever" overclaimed the ruling's scope).
  The garbage-tag denylist is a *display* filter of that future view,
  not a storage concept.
- **Promotions stay lazy**: a promoted generated column is minted with
  the capability that reads it, never ahead. `capture_sort` is the only
  one. Filtering of metadata was firmly out of this round's scope.
- **Reset over migration** (pre-release): the facet reshape shipped with
  a schema bump; no blob migration code exists or existed.

## Extraction (RATIFIED shape)

Extractors align with *source read sessions*, not with facets: one
`CGImageSource` open (`ImagePropertiesExtractor`, RAW included) and one
`AVURLAsset` open (`AVPropertiesExtractor`, video AND audio kinds). Each
fills every facet its one open source testifies to; private methods are
facet-shaped. There is deliberately no "CommonPropertiesExtractor" —
common *output* has no common *read path*. Registry wiring is the
existing capability column; audio rows filled their nils.

Best-effort inside: per-load failures log at debug and yield absent
fields; a wholly unreadable source throws → DLQ residue. Every numeric
crosses a finiteness gate before the encoder (a NaN would throw inside
the insert transaction) and before any `Int()` conversion (which traps).

## Deliberately unsettled (markers — nothing returns silently)

- **Stored-tail cache** — only on offline-deep-metadata or tail-search
  evidence.
- **Deep "everything" view + its display denylist** — future round; rents
  exiftool's registry, never rebuilds it.
- **HDR flag / video color vocabulary** — `visual`'s color fields are
  image-only until the primaries/transfer vocabulary is designed.
- **Embedded keywords** — keywording round's call (observed evidence vs
  authored judgment collision).
- **`document` facet** (pageCount etc.) — returns with a docs kind.
- **XMP-only editorial fields, normalized authorship layer, audio
  title/artist home, music-artist precedence** — the parked
  source-normalization round inherits all of these together.
- **First promotion beyond `capture_sort`** — gated on the filter round's
  file-unit quantifier ruling.
- **Tagged-audio fixture** — TestData has no tagged .mp3/.m4a; the
  ID3/iTunes identifier walk is hand-verified only (TODO in
  AVPropertiesExtractor).
