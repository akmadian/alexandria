# UI scale preference

Wanted (Ari, 2026-07-17 — prompted by AionUi's Appearance → Scale slider): a persistent
whole-chrome scale setting. Explicitly NOT pinch-magnification (§8's out-of-scope line, intent
clarified same date) and NOT density modes (§21, deferred separately).

Design shape, so nothing re-derives it later:

- **Mechanism:** uniform zoom on the root — CSS `zoom` works in both webviews (WKWebView /
  WebView2); native webview zoom factors exist as the alternative. Because the whole system is
  px-locked with no rem (§8), one knob scales the entire fixed geometry uniformly — no
  mixed-unit reflow. Applied pre-paint from localStorage (state plane 3, exactly like the
  theme: `index.html` stamp + a `lib/` module).
- **Control surfaces:** the Appearance/settings row (a slider or stepper with reset), plus the
  desktop-native ⌘+ / ⌘− / ⌘0 bindings — the keyboard half routes through the action registry
  once the keyboard round lands.
- **The one real design cost (§8 1× floor):** fractional scale renders 1px hairlines and
  10–12px Geist at non-integer device sizes — soft seams on exactly the construction the
  aesthetic leans on. The scale ladder (free values vs. integer-friendly steps like
  90/100/110/125/150) is decided at a probe with Ari's eye before shipping.

**Trigger:** the settings screen existing. Build it in then — it is an afternoon-sized feature
once there is a surface to put it on.
