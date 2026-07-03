# Resolved (2026-07-03, moved into docs)

- New file types without rewrites → MIME/extension → dispatcher map; external tools as subprocesses (exiftool/ffmpeg). See 01 + 04.
- Markdown files → treated as documents (01). Per-asset notes → `assets.note`, in FTS, synced to XMP dc:description (03 + 07).
- Media backup → explicitly out of scope; catalog backup only. Documented in 01.
- Interface granularity → settled: repository interfaces + two dispatch interfaces (Thumbnailer, MetadataExtractor). Don't split further.

# Open

- DaVinci Resolve / After Effects project support: parse project file, link referenced assets. Needs a `project_references`-style table (new table, cheap migration) — NOT asset_groups. Post-v1.
- LOGGING - logs should be rich but concise and readable. Colors should be used to denote log levels and source components. (Note: file logs are JSON via slog; colored text handler for dev mode.)
