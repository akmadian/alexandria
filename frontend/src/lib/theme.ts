// Theme = data-theme attribute + localStorage. Machine-local pref, not a backend Setting.
// index.html applies the stored theme before first paint; this module handles changes after.

export const THEMES = ["graphite", "dark", "light"] as const;
export type Theme = (typeof THEMES)[number];

const KEY = "alexandria.theme";

export function getTheme(): Theme {
    const t = document.documentElement.dataset.theme;
    return (THEMES as readonly string[]).includes(t ?? "") ? (t as Theme) : "graphite";
}

export function setTheme(theme: Theme): void {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem(KEY, theme);
}

export function cycleTheme(): Theme {
    const next = THEMES[(THEMES.indexOf(getTheme()) + 1) % THEMES.length];
    setTheme(next);
    return next;
}
