# Roadmap

Major system components yet to be added.


## P0
- Undo/ redo
- ASSETS
    - Add representative selection lever
    - Add lever for RAW thumbnailing being on the raw itself, or the embedded preview when available
- LOUPE
    - Generate larger size thumb for faster loading and offline availability, use that for loupe
    - Add LoupeImageViewer - should have zoom machinery and optionally allow viewing of the full size on-disk asset
- KEYWORDS/ TAGS
    - Inspector Experience
    - Browser Experience
    - Grid Item Decoration
    - Filtering
- Context Menus
- Keybind System
- Context menus
- COLLECTIONS
    - Smart Collections
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
    - Video Asset - Show play button, allow play in grid, also allow scrubbing through video with mouse position left to right on card, video duration as well

## P1 (Plus maybe p2ish stuff)
- Judgement write through
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
- Virtual File Types
	- MD Notes
	- Webpages/ Bookmarks

## Eventually
- Auto updating
- License key system for paid users
- Anonymous opt-in telemetry 
- Browser Extension - grab things from youtube, cosmos, pinterest, etc.
- Catalog backup system, with pruning and all that. Customizable destinations including network destinations

## BUGS
- Arrow keys don't work for grid browsing unless the grid is clicked on first - in general the entire keybinds/ targeting system is wonky.
- Grid doesn't respond correctly when window resized. Resizing window stretches gap between columns until there's enough space for a new one, then the new one pops in. Column count and gap size should be constant, cells can resize. LrC behavior.
- Sorting by capture time doesn't work, videos always get grouped into their own chunk instead of also being sorted by capture time.
- Closing the filter bar keeps filters active - either disable when the bar is collapsed, or have a badge on the filter bar button showing that there are active filters
- Arrow keys don't work in loupe mode.
- Collections with child collections aren't union view?
- View state is not persisted
- Inspector rows: inter-row spacing between image and video or populated rows/ unpopulated looks different

## Styling Tweaks
- Folder/ collection selection should be neutral color instead of blue?
- Refine UI element of what is dragged from the grid - right now it's a mini snapshot of the entire cell, probably best to have it just be the image, and smaller.
