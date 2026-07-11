# Metadata interop with external applications

Not a priority right now — no code, no timeline. Written so the design exists
before it's needed. Extends decisions already made rather than inventing new
architecture; see `../../docs/decisions.md` (D6 registry
capabilities, D8 data classification, D9 identity, D15 XMP sync) and
`_project-tracking/functional-requirements.md` (the "Timestamped video clip
annotations" entry this builds on).

Origin: 2026-07-09/10 design conversation. It started as "shared collections
for contractors," concluded that the collaboration framing was speculative
(see `../ideation/backend-open-questions.md` #11), and landed on the feature that was
actually underneath it: **annotate assets in Alexandria, have that survive
into whatever application opens them next.** The contractor is optional. The
common case is one user, one machine, two applications.

## Where truth lives — the three buckets

Alexandria spans every creative format. The applications it talks to do not
agree on where metadata belongs, and that disagreement is the entire design
constraint:

| Bucket | Truth lives in | Formats / applications |
|---|---|---|
| **File-as-truth** | the file, as embedded XMP (or a sidecar) | JPEG, PNG, TIFF, GIF, PDF, EPS, MP3, **PSD, AI, INDD, SVG**; RAW via `.xmp` sidecar |
| **Project-as-truth** | the application's own database | Premiere, Resolve, Final Cut Pro; Affinity and Apple's suite (**unverified** — see open work) |
| **No metadata layer** | nowhere | fonts (name tables only), LUTs |

Bucket one is large and friendly: most of the Adobe design surface already
speaks XMP natively, and D15 already handles it. Bucket two is where all the
difficulty is. Bucket three gets an honest "not supported."

Alexandria being the one tool that spans all three is the reason XMP cannot be
its internal representation — that would import bucket one's worldview onto
two-thirds of the library.

## The registry: keyed `(asset type, target)`

Per CLAUDE.md's "registries, not hierarchies": one explicit table per dispatch
concern, add a capability = add a row.

The interop target is **its own dispatch concern**, orthogonal to `assettype`.
Do not put targets inside `assettype` rows — one asset type (video) has three
mutually incompatible destinations that share no serialization, and `assettype`
would grow a video×3 problem. Key the table on the pair.

Each row declares:

- where truth lives for this pair (the bucket above)
- `inbound` — can Alexandria learn judgments back from this target?
- `outbound` — can Alexandria push judgments to this target?
- the serializer (they share nothing: embedded RDF/XML, FCPXML, CSV)
- whether `outbound` requires mutating the user's master file

A missing or false capability is not an error. It surfaces through the existing
degraded lane (`seam/impl/15-method-surface.md`, C10's one-fallback rule), the
same way `exiftool_missing` does today. The UI says *why*, and whose fault it is.

## XMP is a serialization, never the internal representation

Timecoded annotations are **judgments** (D8). They live in Alexandria's own
schema, written by the user-action path, bumping `judgment_modified_at` like
every other judgment. XMP, FCPXML, and CSV are things produced at export time
by a target's serializer.

This is overdetermined by the facts: the three video targets want embedded
`xmpDM` RDF, FCPXML, and a filename-matched CSV respectively. There is no
shared wire format to promote to internal truth even if we wanted one.

## Per-target findings

All verified 2026-07-09/10; sources at the bottom.

| Target | Inbound | Outbound | Notes |
|---|---|---|---|
| **Premiere** | yes — reads embedded XMP it wrote | partial — requires container mutation | `xmpDM:Tracks`/`markers`, embedded only. Will **not** read a sidecar when the container supports embedding. |
| **Resolve** | partial — user exports CSV | yes — CSV (clip metadata) / EDL (markers) / FCPXML | "Resolve doesn't interpret XMP" — no sidecar, no embedded, no writeback to source. |
| **Final Cut Pro** | partial — user exports FCPXML | yes — FCPXML | Never writes metadata to media files. Everything lives in the `.fcpbundle` library DB. |
| **RAW stills** | yes | yes | `.xmp` sidecar; already shipped (D15/impl/06). |
| **PSD / AI / INDD / SVG** | yes | yes | Embedded XMP, no sidecar needed. Not yet exercised. |
| **Fonts / LUTs** | no | no | No layer exists. Say so. |

Two known lossy edges: `xmp:Label` carries a **localized color name** ("Verde",
"Rood"), so labels are a decades-old interop bug between LrC and Capture One;
and Resolve flattens FCP's range-based keywords onto the whole clip.

## Sync exists only where a writeback channel exists

**Two of the three NLEs emit nothing.** Final Cut Pro and Resolve keep metadata
in their own databases and offer no path back to the file. This is a property of
those applications, not a gap in Alexandria's design, and no architecture fixes
it. The loop closes only when the user exports FCPXML or CSV and Alexandria
ingests that file — a round trip with a human in the middle.

Be transparent about this in the UI rather than papering over it.

**Premiere is the only genuine writeback**, and its inbound direction is nearly
free: Premiere embeds `xmpDM` markers into the `.mov`/`.mp4`, the watcher
notices, exiftool reads it, and the values land through the existing
`SyncWriter` — the D8 class that writes judgment values but never bumps
`judgment_modified_at`. No new writer class, no new invariant.

There is no video-world analog to LrC's `Metadata → Save Metadata to File`.
Premiere offers an automatic preference and nothing manual; Resolve and FCP
offer nothing at all.

## Container writes — the one trust gamble

**Policy: sidecar wherever the target reads one.** More transparent, less
destructive, and it costs nothing for the entire file-as-truth bucket.

The rule has teeth, and the price should be paid knowingly: **no NLE reads an
XMP sidecar next to a video.** Premiere's rule is that a container capable of
embedding never gets a sidecar. So sidecar-only means *Premiere outbound does
not ship*, while FCP and Resolve outbound ship for free as new files that touch
nothing. That is probably the correct trade.

If outbound Premiere is ever built, it is an **opt-in, per-target exception**,
and the protocol is:

- **exiftool writes, not us.** Its default `-overwrite_original` is
  temp + fsync + `rename(2)` in the same directory — atomic, so no reader ever
  sees a partial file and a crash leaves the master untouched. Do **not** use
  `-overwrite_original_in_place` (exiftool's own docs: slower, less safe); the
  only thing it buys is inode/xattr preservation.
- **Never remux.** Demux/remux rebuilds the whole file. exiftool surgically
  edits the container's box structure and leaves stream packets untouched.
- **Verify, don't trust.** `ffmpeg -i f -map 0 -c copy -f streamhash -hash sha256 -`
  gives per-stream payload hashes at stream-copy speed. Hash before, write,
  hash after, require equality. (ffmpeg's docs caveat that streamhash is for
  content-validation, not remux-validation — this needs the empirical check
  below before it's load-bearing.)
- **Echo suppression already exists.** Record the post-write file hash so the
  watcher doesn't mistake our own write for an external edit — D15's file-level
  hash echo check, unchanged.
- **Writes are limited to the QuickTime family.** exiftool is read/write for
  MOV and MP4, read-only for MKV, AVI, MXF. Premiere reads embedded XMP from
  MOV and MP4. The scope closes on itself: the formats we can't write are the
  formats nobody wanted written.

### Identity: streamhash is a receipt, not an ID

**Do not make the stream hash the identity or dedup key.** D9 stays as it is —
file content hash. Otherwise you have to answer "is a 4K master the same asset
as its remuxed proxy," and the answer is obviously no. Stream hashing is a
write-time verification receipt with a blast radius of one function.

Known consequence, confirmed in the wild: writing XMP into a container changes
the file's checksum. A shipping asset manager (Projective/Strawberry) documents
Premiere's XMP setting causing exactly this — "the Media Library ... will believe
that the old clip has been deleted and a new one added." **Alexandria survives
this where they don't**, because D9 is path-primary: same path + new hash reads
as "file modified," not delete+add. But the hasher and dedup stages will churn
on every annotation write, and that should be a decision rather than a surprise.

## Final Cut Pro: FCPXML in, never `.fcpbundle`

`CurrentVersion.flexolibrary` and `CurrentVersion.fcpevent` are SQLite
databases, so reading them is technically possible. Don't.

The format is Apple-proprietary and unpublished; it changes across FCP releases;
FCP holds the package open and writes to it while running. And it yields the same
rows that **FCPXML** — which Apple documents, versions, and supports as *the*
interchange format — hands over in one menu item, carrying markers, keywords,
ratings, and roles.

This is the `.lrcat` decision again, and it resolves the same way:
`functional-requirements.md` already specifies a one-shot `.lrcat` import rather
than live parsing. Mirror it. Plan for the newer `.fcpxmld` bundle variant
(FCP 10.6+) as well as plain `.fcpxml`. If the export ever needs to be driven
without the user clicking, the sanctioned route is an FCP Workflow Extension,
not reading the database behind its back.

## Open design work

- **The streamhash invariance experiment** (`04-open-questions.md` #15) — the
  cheapest item here and it decides the riskiest question. Until it passes,
  container writes stay theoretical.
- **Affinity and Apple's creative suite** — assumed project-as-truth, actually
  unverified. Could not find documentation either way.
- **What a timecoded annotation is in the schema.** A note plus a timecode is the
  obvious guess; ranges (FCP keyword ranges) may not fit it.
- **Resolve inbound matching rules** — its CSV import matches on filename and
  clip start/end timecode, with several fallbacks. Which do we emit?
- **Whether outbound Premiere ships at all.** See the trade above.

## Explicitly out of scope

- Building any of this.
- **Bundle export / merge-back** (`04-open-questions.md` #11) — a different
  feature with a different justification. Do not entangle.
- **Peer-to-peer or live sync between two Alexandria instances.** Considered and
  rejected 2026-07-09: no local-first DAM does it; every one of them either
  ships a server or ships a file that travels. Creative handoff is asynchronous,
  so a direct peer connection needs store-and-forward, which is a server. It also
  breaks "one cook" (two writers) and D20 (a sync engine must auto-mutate identity
  or stall). The demand in this market is asymmetric — publish, mark up, absorb —
  not symmetric co-editing.

## Sources

- Premiere embeds `xmpDM` markers into `.mov`/`.mp4`, changing the checksum:
  [Projective/Strawberry Premiere XMP settings](https://manuals.projective.io/strawberry-manuals/v64/en/topic/adobe-premiere-pro-cc-recommended-settings-for-xmp-metadata),
  [Creative Impatience](https://www.creativeimpatience.com/premiere-pro-clip-markers-solved/)
- Premiere won't use a sidecar when the container can embed:
  [Creative COW](https://creativecow.net/forums/thread/import-of-xmp-metadata-from-sidecar-files/)
- Premiere clip metadata lives in the project file:
  [Adobe](https://helpx.adobe.com/premiere-pro/using/metadata.html)
- `xmpDM:Tracks` / Marker types: [Adobe XMP docs](https://developer.adobe.com/xmp/docs/xmp-namespaces/xmp-dm/)
- Resolve doesn't interpret XMP; no writeback to source:
  [Blackmagic forum](https://forum.blackmagicdesign.com/viewtopic.php?f=21&t=66127)
- Resolve media pool metadata CSV import/export + EDL markers:
  [Resolve manual](https://www.steakunderwater.com/VFXPedia/__man/Resolve18-6/DaVinciResolve18_Manual_files/part4015.htm)
- FCP stores metadata in the library bundle, never the media:
  [Larry Jordan](https://larryjordan.com/articles/a-final-cut-pro-x-library-is-a-collection-of-stuff/)
- FCP library is proprietary/unpublished; bundle contains SQLite:
  [Apple Community](https://discussions.apple.com/thread/7453326), [fcp.cafe](https://fcp.cafe/developers/librarybundle/)
- FCPXML is the supported interchange format: [Apple Developer](https://developer.apple.com/documentation/professional-video-applications/fcpxml-reference);
  `.fcpxmld` from 10.6: [Philip Hodgetts](http://www.philiphodgetts.com/2021/11/final-cut-pro-10-6s-xml-package-explained/)
- exiftool writable formats (MOV/MP4 r/w; MKV/AVI/MXF read-only): [exiftool.org](https://exiftool.org/)
- `-overwrite_original_in_place` is slower and less safe: [exiftool man page](https://linux.die.net/man/1/exiftool)
- ffmpeg cannot write XMP to the MP4 `uuid` box (exiftool is the only path):
  [ffmpeg-user list](https://lists.ffmpeg.org/pipermail/ffmpeg-user/2023-November/057091.html)
- `streamhash` muxer: [ffmpeg formats docs](https://ffmpeg.org/ffmpeg-formats.html)
- Formats carrying embedded XMP (incl. PSD, AI, INDD, SVG):
  [XMP (Wikipedia)](https://en.wikipedia.org/wiki/Extensible_Metadata_Platform),
  [Adobe XMP Spec Part 3](https://dl.photoprism.app/pdf/specifications/20120101-Adobe_XMP_Specification_Part_3.pdf)
- `xmp:Label` localized-color-name interop bug:
  [FastRawViewer](https://www.fastrawviewer.com/node/395),
  [Capture One community](https://support.captureone.com/hc/en-us/community/posts/360009396057-Lightroom-migration-bug-Color-labels-not-read-from-xmp)
