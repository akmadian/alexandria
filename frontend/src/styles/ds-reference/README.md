# Alexandria Design System — dist (v1.0.0)

GENERATED distributable — do not edit. Regenerated from the DS source each release.
This is the self-contained vendor bundle a consuming app copies in.

## Contents
- `alexandria-ds.css` — the entire visual foundation in one file: @font-face for all
  four faces + the full token layer (primitives → semantic → themes → material →
  typography → spacing → motion → base). Themeable via `[data-theme]` and
  `[data-material]` on a root element.
- `fonts/` — the font binaries `alexandria-ds.css` references (Geist, Geist Mono,
  Geist Pixel, Instrument Serif). Fully offline; no network.
- `pharos-ascii.js` — the `<pharos-ascii>` WebGL web component.
- `assets/` — brand renders: hero app icon (`logo-icon.svg` / `-512.png` / `favicon-32.png`)
  and the flat lighthouse glyph (`logo-glyph.svg` currentColor, `-beam` variant, + PNGs).
- `components-spec/*.css` — per-component visual specs (read to port; do not import).
- `CLAUDE.md` / `PORTING.md` — agent + human integration guides.
- `VERSION` — semver of this build.

## Consume (Vite / React 19)
```ts
import ".../alexandria-ds.css";   // tokens + fonts (keep fonts/ beside this file)
import ".../pharos-ascii.js";     // registers <pharos-ascii>
```
Build components as CSS Modules using the semantic tokens; drive behavior with
React Aria. See the DS's `PORTING.md` for the component port recipe.

Because this bundles all four faces, it is the single source of type — you can drop
a separate `geist` npm dependency and rely on this, or keep bun's geist and ignore
the Sans/Mono @font-face here (same font, harmless duplicate).
