// Theme = data-theme attribute + localStorage. Machine-local pref, not a backend Setting.
// index.html applies the stored theme before first paint; this module handles changes after.
// The theme vocabulary is GENERATED from the token resolver's contexts (C15) — this
// module never declares its own list.

import { defaultTheme, themes, type Theme } from "@/styles/tokens";

export { defaultTheme, themes, type Theme };

const KEY = "alexandria.theme";

export function getTheme(): Theme {
    const current = document.documentElement.dataset.theme;
    return (themes as readonly string[]).includes(current ?? "") ? (current as Theme) : defaultTheme;
}

export function setTheme(theme: Theme): void {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem(KEY, theme);
}

export function cycleTheme(): Theme {
    const next = themes[(themes.indexOf(getTheme()) + 1) % themes.length];
    setTheme(next);
    return next;
}
