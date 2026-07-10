// Presentation formatting at the render edge (§5 of the UI doc). Data stays as
// ISO strings / numbers in the cache; these convert at display time via Intl.
// No date library — Intl covers everything a DAM shows.

import i18n from "@/i18n";

// Memoize formatter instances per locale (they're expensive to construct).
const cache = new Map<string, Intl.DateTimeFormat | Intl.NumberFormat>();

function dateFmt(opts: Intl.DateTimeFormatOptions, key: string): Intl.DateTimeFormat {
    const k = `${i18n.language}:${key}`;
    let f = cache.get(k) as Intl.DateTimeFormat | undefined;
    if (!f) {
        f = new Intl.DateTimeFormat(i18n.language, opts);
        cache.set(k, f);
    }
    return f;
}

export function formatDate(iso: string): string {
    return dateFmt({ dateStyle: "medium" }, "d").format(new Date(iso));
}

export function formatDateTime(iso: string): string {
    return dateFmt({ dateStyle: "medium", timeStyle: "short" }, "dt").format(new Date(iso));
}

const BYTE_UNITS = ["byte", "kilobyte", "megabyte", "gigabyte", "terabyte"] as const;

export function formatBytes(bytes: number): string {
    let value = bytes;
    let i = 0;
    while (value >= 1000 && i < BYTE_UNITS.length - 1) {
        value /= 1000;
        i++;
    }
    const k = `${i18n.language}:b${i}`;
    let f = cache.get(k) as Intl.NumberFormat | undefined;
    if (!f) {
        f = new Intl.NumberFormat(i18n.language, { style: "unit", unit: BYTE_UNITS[i], maximumFractionDigits: value < 10 ? 1 : 0 });
        cache.set(k, f);
    }
    return f.format(value);
}

/** Locale-aware integer/decimal grouping — counts, pixel dimensions, EXIF numbers. */
export function formatNumber(value: number): string {
    const k = `${i18n.language}:n`;
    let f = cache.get(k) as Intl.NumberFormat | undefined;
    if (!f) {
        f = new Intl.NumberFormat(i18n.language);
        cache.set(k, f);
    }
    return f.format(value);
}

/** 65 → "1:05"; 3725 → "1:02:05". Media duration, not a date. */
export function formatDuration(totalSecs: number): string {
    const s = Math.round(totalSecs);
    const h = Math.floor(s / 3600);
    const m = Math.floor((s % 3600) / 60);
    const sec = s % 60;
    const mm = h > 0 ? String(m).padStart(2, "0") : String(m);
    return `${h > 0 ? `${h}:` : ""}${mm}:${String(sec).padStart(2, "0")}`;
}
