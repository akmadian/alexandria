# grid-cache-spike

Validates the two open frontend risks from `docs/frontend-architecture.md` §6:

1. **Custom-scheme caching:** does the webview HTTP cache honor `Cache-Control: immutable`
   when assets come through the Wails asset handler (custom URL scheme, not real HTTP)?
   Measured as asset-handler hits during scroll-back-up vs scroll-down.
2. **Webview scroll performance:** virtualized grid frame times in the real webview
   (WKWebView / WebKitGTK), not Chromium.

The app is self-running: generates thumbnails, auto-scrolls to the bottom and back while
recording rAF frame deltas, prints `=== SPIKE RESULT ===` JSON to stdout, writes
`results.json`, and quits.

## Run

```sh
# macOS (UniformTypeIdentifiers must be linked explicitly under plain `go build`)
CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production -o grid-spike .
# Linux
go build -tags desktop,production -o grid-spike .

./grid-spike
```

**Must be a production build.** `wails dev` serves assets over real HTTP, which sidesteps
the custom-scheme cache question — the thing being tested.

Options (env): `THUMBS=10000` tile count (default 3000) · `MANUAL=1` skip the auto-test
and scroll by hand with a live hit-counter HUD.

## Reading results

- `handlerHits.requestsDuringUpPass ≈ 0` → webview cache works on the custom scheme;
  delete the §6 caveat and the LRU contingency.
- `requestsDuringUpPass ≈ unique` → no HTTP caching on scrollback. Check `scrollUp`
  frame stats: if still smooth, the fallback is free and the caveat closes the other way;
  if janky, the in-memory LRU in the thumbnail loader (§7) gets built.
- `droppedPct` under ~5 with `p95Ms` ≤ ~17 → 60fps target met.

Run on macOS **and** Linux (WebKitGTK is the PRD's go/no-go gate; needs
`libgtk-3-dev libwebkit2gtk-4.0-dev` to build).

## Results so far

**macOS / WKWebView, 2026-07 (M-series, Wails v2.12):**

| Run | Scrollback refetches | Scroll-up frames |
|---|---|---|
| 3k tiles × 7.2KB (~21MB) | 31 / 3000 (1%) | p95 17ms, 2.0% dropped |
| 10k tiles × 21.3KB (~213MB) | 8704 / 10000 (87%) | p95 18ms, 1.35% dropped |

Interpretation: there is **no durable HTTP cache** on the `wails://` scheme — only
WebKit's size-bounded in-memory image cache, which held the 21MB working set and
evicted most of the 213MB one. **And it doesn't matter:** the 87%-refetch scroll-up
was frame-for-frame identical to the fully-cached one. Handler re-serves of ~20KB
files are frame-rate-neutral. Conclusions: keep the immutable header (free win when
the memory cache holds), do **not** build a frontend LRU, macOS perf gate passed.
Remaining: the WebKitGTK run.
