# The DAM landscape: what exists, and why

**Status: RESEARCH** (web survey, 2026-09-09). Grounding for the requirements
definition — nothing in here is a requirement. The synthesis and positioning at
the end are PROPOSED; the requirements session decides.

## PLATFORMS
- Over 70% of creative professionals use MacOS as their primary OS.
- 60-75% of Graphic design studios use Mac
- 65-80% of Professional Video Editors Use Mac
- Broader video editing market has Windows at ~45% - includes gaming/ youtuber mass market which skews windows hard.

## The field

### Lightroom Classic — the incumbent library

The catalog model Alexandria's split-truth design descends from: [originals stay
on disk (anywhere — internal, external, network); the catalog records where files
live plus ratings, keywords, labels, edits, and previews](https://helpx.adobe.com/lightroom-classic/help/lightroom-catalog-basics.html).
Organization = folders (observed) + collections/smart collections (authored) +
keywords + ratings/flags/labels. Why people use it: [search thousands of images
instantly, smart collections, projects tracked across years](https://narrative.so/blog/lightroom-catalogs-101) —
the *library over a career* job. Previews make offline volumes browsable.

### Capture One — sessions vs catalogs

The only major player offering [both a per-project mode (Sessions: a portable
mini-filing-system per shoot, edits in per-image settings files, no central DB)
and a library mode (Catalogs: one searchable access point across all
work)](https://support.captureone.com/hc/en-us/articles/30041173920029-Sessions-vs-Catalogs-in-Capture-One-how-they-work-and-where-to-store-them).
Pros commonly [work in a Session per active job, then archive completed sessions
into a master catalog](https://fstoppers.com/capture-one/why-you-should-be-using-both-sessions-and-catalogs-capture-one-pro-310162).
The insight worth keeping: **the active-job workflow and the career-archive
workflow are different shapes**, and forcing one tool-shape onto both creates
friction.

Why sessions are *loved*, specifically (follow-up survey): [a new session
auto-creates the whole job's folder structure — capture, selects, output — and
tethered shots land in it instantly](https://fstoppers.com/capture-one/how-set-your-capture-one-session-improve-your-tethered-shooting-workflow-250210);
[the session folder contains everything, so moving a job between machines or
into the archive is copying one folder](https://www.captureone.com/blog/take-control-of-your-image-organization-with-sessions-or-catalogs).
A session can also [browse any folder on any drive without importing — the
System Folders panel makes a session usable as a plain file browser, with edits
stored in `.cos` sidecar files beside the images](https://support.captureone.com/hc/en-us/articles/360003115137-The-Complete-Guide-to-Sessions-in-Capture-One),
so a session is C1's browse-without-import mode, not only its tethering mode.

### Photo Mechanic — the speed benchmark

Pure browser, no catalog. [2–3x faster culling than Lightroom because it renders
the embedded JPEG preview already inside every RAW instead of decoding the
RAW](https://imagen-ai.com/valuable-tips/photo-mechanic-vs-lightroom-culling/) —
no import wait, no preview lag, instant keystroke-to-keystroke flow. Also the
ingest reference: card ingest with renaming, metadata stamping, multi-destination
backup. Why it survives next to Lightroom: it is the fastest path from card to
selects, and [the industry-standard workflow is literally PM for ingest/cull,
then Lightroom for everything after](https://imagen-ai.com/valuable-tips/photo-mechanic-vs-lightroom-culling/).
**The embedded-preview trick is the single most load-bearing performance fact in
this survey.**

### Apple Photos — the managed anti-model

Monolithic managed library; files swallowed into an opaque bundle. Rejected as a
model in the reset discussion (disk-space disrespect, lock-in, unworkable at
multi-TB). Its lesson is what *polish* looks like: zero-setup, instant search,
faces/places for free.

### digiKam — the open-source maximalist

[Albums + tags + ratings + faces + GPS, EXIF/IPTC/XMP standards throughout,
100k+-image libraries, batch processing, RAW + video](https://www.digikam.org/about/).
Proof that the feature surface is well understood and commoditized; its weakness
is exactly the thing it can't list — coherence and feel. A feature checklist
does not make a product.

### Eagle — the adjacent-market surprise

Not a photographer's DAM — a *creative-asset* library (design refs, screenshots,
fonts, 3D, video, audio; [81+ formats, tagging, color search, browser capture,
big libraries that stay fast](https://www.producthunt.com/products/eagle)).
Why it matters: it demonstrated a market for **fast, pleasant, local,
every-filetype** asset management priced as a one-time purchase — the users
Lightroom ignores because their assets aren't photos.

Complaint survey (follow-up): published reviews are [largely positive — speed at
scale, tagging, browser capture all praised; complaints are practical: no
mobile/iPad, limited photo-grade features, tag/sidebar organization friction,
and the library system consuming disk space](https://www.capterra.com/p/184384/Eagle/reviews/).
That last one is load-bearing: Eagle *copies files into a managed library* —
the disk-space complaint is the managed-model tax, and the referenced model
avoids it structurally. Ari's own critique (2026-09-09, first-hand): overly
glossy; everything-is-a-plugin for basic features; confused midpoint between
file properties and catalog entities; capability a mile wide and an inch deep;
opaque provenance. What Eagle proves anyway: dump-everything-in breadth and
strong search are what users of this category actually value — and browser/web
capture (bookmarks as first-class assets) is loved, which matches a feature
Ari independently wants.

### Peakto / Mylio — the AI and sync angles

[Peakto: AI auto-categorization, aesthetic/technical scoring, conversational
search, and meta-cataloging *over other apps' libraries*](https://cyme.io/photographer-blog/best-image-organizer-comparison/).
Mylio: cross-device sync as the headline. Both answer "what's newer than
Lightroom's model" — neither has displaced the catalog incumbents.

### Kyno / CatDV — the video side

Video people have the same split we found in photo: [Kyno = the Photo Mechanic
of video (browse any storage in place, flat filterable view of mixed media,
tag/log, transcode, verified multi-destination camera-card backup — no central
catalog)](https://www.richardlackey.com/kyno-review-media-management-for-video-creators/);
CatDV = the enterprise catalog (logging database, AI tagging, broadcast-scale).
The gap: nothing serves the solo/small creative working in *mixed* photo+video —
photo DAMs treat video as an afterthought, video MAMs are enterprise-priced.

The closest thing to "Lightroom for video" was Kyno — and it died of acquisition neglect, not competition: Signiant bought it in 2021, halted sales, stalled development, and redirected the team to enterprise; working editors in the comment sections called it "indispensable" and report finding nothing to replace it. Adobe killed Prelude the same year. And the single most quotable data point: a small-production-company owner, reacting to exactly those two deaths, describing "a hole in the market the size of the Grand Canyon for a simple, cost-effective asset management system for small shops... a TON of one-man-band shops struggling with asset management and NOTHING out there." That's your user, in their own words, asking for your product category.

## Pain points in the incumbents (requirements in disguise)

- **Lightroom performance decay and catalog corruption.** [Multi-minute loads,
  8-hour imports of 60 JPEGs, RAM ballooning](https://imagen-ai.com/valuable-tips/why-lightroom-classic-slow-fix/);
  [catalogs corrupting — for some users daily — from crashes and from cloud-sync
  software touching a live database file](https://helpx.adobe.com/lightroom-classic/kb/troubleshoot-corrupt-catalog.html).
  Durability and sustained performance aren't features here; they're the wound.
- **Photo Mechanic has no library.** Unplug the disk and your work doesn't
  exist; no cross-shoot search. (The reason file rows earned their place.)
- **Capture One's power = complexity tax**; the sessions/catalogs fork itself
  confuses newcomers for years.
- **Eagle isn't metadata-serious** (no EXIF-grade schema, no XMP round-trip);
  photographers bounce off it.
- **Subscription fatigue** is a stated reason people shop for Lightroom
  alternatives at all.

## Synthesis: the jobs a DAM is hired for

Every product above is some subset of six jobs. The "why" column is the user
need that makes the job exist — requirements should trace to these, not to
competitor feature lists.

| Job | Why it exists | Reference implementation |
|---|---|---|
| **Ingest** | Get media off cards/disks safely; rename/stamp/backup at entry, because entry is the only moment the whole batch is in hand | Photo Mechanic, Kyno |
| **Cull** | Thousands shot, dozens kept; the sooner the losers leave the pipeline, the cheaper everything downstream | Photo Mechanic (embedded previews) |
| **Judge & organize** | Ratings/flags/labels/groups/collections — the user's *work product* layered over their files | Lightroom (collections + smart collections) |
| **Find** | "That shot from 2019" across TBs and offline volumes, instantly | Lightroom catalog; Eagle for feel |
| **Metadata** | Standards round-trip (EXIF/IPTC/XMP) so the work outlives the tool | digiKam, Photo Mechanic |
| **Hand off** | Media leaves for an editor/export/delivery; the DAM is a hub, not a terminus | C1 sessions, PM→LR workflow |

Alexandria's ratified stance already places it: split truth + referenced files +
index-not-copy = the *Lightroom library model*, aimed to be executed with
*Photo Mechanic's speed* (embedded previews, no import wall), across *Eagle's
breadth* (photo + video + audio as first-class), native Mac, with durability as
a product promise (the anti-Lightroom-corruption position — WAL SQLite, single
writer).

## Open questions the requirements session must answer

1. **Who exactly is the user?** "Creative professionals" spans a sports shooter
   (ingest/cull speed is everything) to a design hoarder (Eagle's user). The
   feature cut changes with the answer.
2. **Editing scope.** None (pure DAM, hand off to editors — the PM position)?
   Basic adjustments? The old repo's file-based editor-loop idea is archaeology
   available if wanted.
3. **The active-job vs archive split.** Does Alexandria acknowledge C1's
   sessions insight (a lightweight per-shoot mode) or is one catalog the answer?
4. **AI features.** Faces, aesthetic scoring, semantic search are now table
   stakes in marketing (Peakto, CatDV, Imagen) — in or out for v1, and
   local-only?
5. **Multi-catalog?** One catalog per user, or many (job-based)?
6. **Mobile/sync.** Mylio's territory. Presumably out entirely; state it.
7. **Performance targets as functional requirements.** The 40k-NAS-import
   scenario needs numbers (grid interactive in Ns, scroll never drops frames,
   cull at PM keystroke speed).
