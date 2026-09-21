# Roadmap

Major system components yet to be added.

## Levers
- Default asset representative selection
- RAW thumbs - the raw itself? Or the embedded preview when available?
- Import perf levers
- What else? 

## P0
- Undo/ redo
- Import/ Disk Sync
    - p0 should support "rescanning" with a "last scanned at" timestamp on the files. This gives a good middle ground between zero rescan support and full disk watcher systems.
- LOUPE
    - Generate larger size thumb for faster loading and offline availability, use that for loupe
    - Add LoupeImageViewer - should have zoom machinery and optionally allow viewing of the full size on-disk asset
- KEYWORDS/ TAGS
    - Inspector Experience
    - Browser Experience
    - Grid Item Decoration
    - Filtering
- Context Menus
- COLLECTIONS
    - Better Collection Addition/ Rename/ Etc Experience
- Open in external app
- Deeper macos integration
    - Catalog extension registration
    - registration of app with "open in" things
    - maybe register app in finder context menus
    - app icon progress bar in dock
    - Share sheets
    - Notifications
    - AppleScript/ Shortcuts support?
    - Support icloud backup of catalog files
    - Dock menu
- GRID
    - Grid grouping
    - Keep iterating on cell design
    - Video Asset - Show play button, allow play in grid, also allow scrubbing through video with mouse position left to right on card, video duration as well. Finder allows playing videos from the little thumbnail, how can we do that?

## P1 (Plus maybe p2ish stuff)
- "About Alexandria" - license, etc. Also open source lib usage and their license statements
- Judgement write through
- Stacks
- Filesystem operations (rename, copy, duplicate, etc)
- Image/ video rotation (rotation correction)
- Files/ Assets lens
- Integrated RAW editing
- Browse and Judge Without Import
- Map Features
    - Map View
    - GPX Track Corellation
    - Vritual asset - pins/ markers? What does mapbox offer?
- Lights Out View
- Fullscreen View
- Settings System
- Favorite folders/ collections?
- Export
- Virtual File Types
	- MD Notes
	- Webpages/ Bookmarks

## Eventually
- Auto updating
- License key system for paid users
- Anonymous opt-in telemetry 
- Browser Extension - grab things from youtube, cosmos, pinterest, etc.
- Catalog backup system, with pruning and all that. Customizable destinations including network destinations
- Tethered Capture
- Printing Layout and Soft Proofing

## BUGS
- Closing the filter bar keeps filters active - either disable when the bar is collapsed, or have a badge on the filter bar button showing that there are active filters
- Collections with child collections aren't union view?
- View state is not persisted
- Inspector rows: inter-row spacing between image and video or populated rows/ unpopulated looks different
- Loupe arrow key nav: changes cursor successfully, but the image doesn't update immediately. It only updates after a small delay on keybind settle.

## Styling Tweaks
- Folder/ collection selection should be neutral color instead of blue?
- Refine UI element of what is dragged from the grid - right now it's a mini snapshot of the entire cell, probably best to have it just be the image, and smaller.
- Star Rating primitive - spacing between icons is variable instead of locked, a little too wide as well. Hovering over a dot seems to shift other icons

## PERF
- Allow usage of more than one thread for import?
