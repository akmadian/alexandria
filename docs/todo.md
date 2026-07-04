# Resolved (2026-07-03, moved into docs)

- New file types without rewrites → MIME/extension → dispatcher map; external tools as subprocesses (exiftool/ffmpeg). See 01 + 04.
- Markdown files → treated as documents (01). Per-asset notes → `assets.note`, in FTS, synced to XMP dc:description (03 + 07).
- Media backup → explicitly out of scope; catalog backup only. Documented in 01.
- Interface granularity → settled: repository interfaces + two dispatch interfaces (Thumbnailer, MetadataExtractor). Don't split further.

# Open

- DaVinci Resolve / After Effects project support: parse project file, link referenced assets. Needs a `project_references`-style table (new table, cheap migration) — NOT asset_groups. Post-v1.
- LOGGING - logs should be rich but concise and readable. Colors should be used to denote log levels and source components. (Note: file logs are JSON via slog; colored text handler for dev mode.)
- AI auto-tagging?
- AI face detection and grouping?
- AI assisted culling? - probably not, this feels more like a LrC or photo management specific task.
- Maybe allow user to select "LrC is source of truth" or "alexandria is source of truth" for metadata. This will give a clean and well understandable choice as to which program owns raw and rasterized image metadata.

## UI
- Logging - important. Want users to be able to upload a log file in case they're having an issue
- Heirarchical folder sidebar component with Folders nested inside sources.
- Heirarchical collection sidebar component
- Heirarchical tag sidebar component
- Loupe view with full size render of asset
- Map view to see photo geolocations on a map? The whole location mapping thing is interesting - how do we generalize coordinates to a town or area that someone might search for?
- Metadata editing
- User should be able to select UI color scheme beyond just dark and light - users will be using alexandria for color sensitive work, so having neutral grey as an option is important and should likely be the default.
- The UI should be spartan, but nice to look at. Compact, respectful of limited screen space. Clean, generally flat colors with clear text heirarchy, retrofuturistic inspiration and design elements. The UI should be nice to look at, but get out of the way of the assets.
- Accessibility and multi language support is important.

### UI Refresh
- System pieces
    - i18n
    - Logging and observability
    - Testing system - unit tests are absolute minimum. Also want test coverage data.
    - Scss or sass type thing - i want to have better styling features than just basic css
    - Do we need or want some frontend state management? I would try to lean away from this where possible. State management is a headache.
    - Linting - eslint probably
    - Any others you can think of?
- Styling and Page Structure
    - I want to focus on building out a unique style and the page structure, I don't want to focus on responsiveness, scaling, etc. We will not support mobile or tablet. Only desktop for now.
    - How can we use a system to give us responsiveness and scaling, as well as grids and such, without a massive headache of handling it ourselves?
- Components (Non exhaustive, just an example of structure and such)
    - Want to have components for domain concepts and bits of decoration.
    - Asset - something that populates onto the grid, represents the asset domain model
    - Tree - Resuable component for displaying heirarchichally structured data. Folder structures, collections, tags, etc. Selector at top. See reference image.
    - Modal - Wrapper around some inner content - could be for user settings, could be for smart collection definitions.
    - Button
    - Tag
    - InputField
    - Views
        - GridView - The actual grid view. Shows assets and asset groups
        - InspectorView - The inspector panel - all information about a single asset or asset group.
        - BrowserView - The tree component, the left sidebar thing.