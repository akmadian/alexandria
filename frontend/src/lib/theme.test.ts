// theme.ts drives the data-theme attribute + the stored preference. The vocabulary
// is generated (styles/tokens.ts) — these tests pin the fallback and cycle behavior,
// including the v3 flip of the absent-attribute default to the generated defaultTheme.

import { beforeEach, expect, test } from "vitest";
import { cycleTheme, defaultTheme, getTheme, setTheme, themes } from "./theme";

beforeEach(() => {
    delete document.documentElement.dataset.theme;
    localStorage.clear();
});

test("getTheme falls back to the generated default when the attribute is absent or junk", () => {
    expect(getTheme()).toBe(defaultTheme);
    document.documentElement.dataset.theme = "not-a-theme";
    expect(getTheme()).toBe(defaultTheme);
});

test("getTheme returns any generated theme verbatim", () => {
    for (const theme of themes) {
        document.documentElement.dataset.theme = theme;
        expect(getTheme()).toBe(theme);
    }
});

test("setTheme stamps the attribute and persists the preference", () => {
    setTheme("carbon");
    expect(document.documentElement.dataset.theme).toBe("carbon");
    expect(localStorage.getItem("alexandria.theme")).toBe("carbon");
});

test("cycleTheme walks the generated vocabulary and wraps", () => {
    setTheme(themes[themes.length - 1]);
    expect(cycleTheme()).toBe(themes[0]);
    expect(document.documentElement.dataset.theme).toBe(themes[0]);
});
