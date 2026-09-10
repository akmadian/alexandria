## File/Asset Model
**One asset, zero files (the bookmark class):**
- Web bookmarks/URLs — your case. A catalog entry that's pure reference. Once one exists, the class exists.
- Cloud placeholder files — iCloud Drive/Dropbox "online-only" files: they appear on disk but are dataless stubs; content isn't local until materialized. The file exists; its bytes don't. This one will bite in practice because users' Desktops are full of them.

**One asset, many files (beyond RAW+JPEG):**
- Image/frame sequences — an EXR or DPX render is one clip made of ten thousand numbered files. VFX-adjacent users have these; a grid showing 10,000 frames instead of one clip is broken.
- Fragmented camera formats — AVCHD spreads one recording across a BDMV directory structure; RED's .RDC folders hold one clip as multiple R3D segments. The camera's "clip" never was one file.
- macOS packages — the sneakiest: a Final Cut library, a Photos library, many app documents are directories masquerading as files. Finder shows one icon; the filesystem sees a tree. Your project-files-as-citizens feature hits this immediately — .fcpbundle is a folder.
- Font families — one "font" spanning several files.

**One file, ambiguous identity:**
- Symlinks, Finder aliases, hardlinks — the same content reachable at two paths, or two directory entries sharing one inode. Is that one asset or two? (Also the degenerate cousin: genuine duplicate copies, which you've already put on the list.)
- Archives — a ZIP of assets: one file, or a container of many?

## Catalog Layer
[[database.md]]
- Need to do a failure-mode research pass. Why do catalogs fail/ corrupt? How can we prevent that? Use LrC threads as canonical example with many forum threads for mining forensic details.
- Use GRDB DatabasePool to swap catalogs while app is running


## Multi-Machine Continuity/ Sync
Idea - any sync model - cloud relay or lan p2p reduces to the same merge problem: to catalogs, same logical library, different recent changes.
Solving this requires two things baked in from day one: stable IDs on everything, and per-judgement/per-field modification timestamps.

Clock Sync
- Trust wall clock (works in almost all cases), or Lamport clocks/ version vectors, or hybrid logical clocks