// Interim token generator — reads design/tokens.json (THE source of truth,
// docs/design-constitution.md §22) and injects the --alx-* CSS variables per theme.
// ponytail: dies when the real compiler (tokens → static CSS + validator in `make
// check`) lands; nothing outside this file may know token VALUES, only var names.

import tokensJson from "../../design/tokens.json";

type TokenNode = { $value?: string } & Record<string, unknown>;
const T = tokensJson as unknown as Record<string, Record<string, TokenNode>>;

const val = (n: unknown): string => {
    const node = n as TokenNode;
    return (node && typeof node === "object" && "$value" in node ? node.$value : n) as string;
};

export const THEMES = Object.keys(T.theme).filter((k) => !k.startsWith("$"));
export type ThemeName = (typeof THEMES)[number];

function themeBlock(name: string): string {
    const theme = T.theme[name] as unknown as Record<string, Record<string, TokenNode>>;
    const lines: string[] = [];
    for (const [group, entries] of Object.entries(theme)) {
        if (group.startsWith("$")) continue;
        for (const [key, tok] of Object.entries(entries)) {
            if (key.startsWith("$")) continue;
            lines.push(`--alx-${group}-${key}: ${val(tok)};`);
        }
    }
    return `[data-theme="${name}"]{${lines.join("")}}`;
}

function rootBlock(): string {
    const r: string[] = [];
    const hue = T.hue as Record<string, TokenNode>;
    r.push(`--alx-stage: ${val((T.stage as Record<string, TokenNode>).default)};`);
    r.push(`--alx-accent: ${val(hue.accent)};`);
    r.push(`--alx-attention: ${val(hue.attention)};`);
    r.push(`--alx-error: ${val(hue.error)};`);
    for (const [k, v] of Object.entries(hue.label as Record<string, TokenNode>)) {
        if (!k.startsWith("$")) r.push(`--alx-label-${k}: ${val(v)};`);
    }
    const light = hue["light-register"] as Record<string, TokenNode>;
    r.push(`--alx-light-gradient: ${val(light.gradient)};`);
    r.push(`--alx-light-flow-duration: ${val(light["flow-duration"])};`);
    r.push(`--alx-light-glass-blur: ${val(light["glass-blur"])};`);
    const type = T.type as Record<string, Record<string, TokenNode>>;
    for (const [k, f] of Object.entries(type.family)) if (!k.startsWith("$")) r.push(`--alx-font-${k}: ${val(f)};`);
    for (const [k, sc] of Object.entries(type.scale)) {
        if (k.startsWith("$")) continue;
        const scale = sc as unknown as { size: string; lineHeight: string; tracking: string };
        r.push(`--alx-type-${k}-size: ${scale.size}; --alx-type-${k}-lh: ${scale.lineHeight}; --alx-type-${k}-tracking: ${scale.tracking};`);
    }
    for (const [k, w] of Object.entries(type.weight)) if (!k.startsWith("$")) r.push(`--alx-weight-${k}: ${w as unknown as number};`);
    for (const [k, d] of Object.entries(T.layout)) {
        if (k.startsWith("$")) continue;
        const v = val(d);
        if (typeof v === "string" || typeof v === "number") r.push(`--alx-${k}: ${v};`);
    }
    for (const [k, d] of Object.entries(T.radius)) if (!k.startsWith("$")) r.push(`--alx-r-${k}: ${val(d)};`);
    for (const [k, d] of Object.entries(T.z)) if (!k.startsWith("$")) r.push(`--alx-z-${k}: ${val(d)};`);
    for (const [k, d] of Object.entries(T.shadow)) if (!k.startsWith("$")) r.push(`--alx-shadow-${k}: ${val(d)};`);
    const motion = T.motion as Record<string, Record<string, TokenNode>>;
    for (const [k, d] of Object.entries(motion.duration)) r.push(`--alx-dur-${k}: ${val(d)};`);
    for (const [k, d] of Object.entries(motion.easing)) r.push(`--alx-ease-${k}: ${val(d)};`);
    const focus = T.focus as Record<string, TokenNode>;
    r.push(`--alx-focus-ring-width: ${val(focus["ring-width"])}; --alx-focus-ring-offset: ${val(focus["ring-offset"])};`);
    return `:root{${r.join("")}}`;
}

// Inject once at module load (imported for side effect from main.tsx, before render).
const style = document.createElement("style");
style.id = "alx-tokens";
style.textContent = [rootBlock(), ...THEMES.map(themeBlock)].join("\n");
document.head.appendChild(style);
// Unknown/legacy theme names (or none) fall back to the default (§21).
if (!THEMES.includes(document.documentElement.dataset.theme ?? "")) {
    document.documentElement.dataset.theme = "paper";
}

export function setTheme(name: ThemeName): void {
    document.documentElement.dataset.theme = name;
    try {
        localStorage.setItem("alexandria.theme", name);
    } catch {
        /* pre-paint pref only; losing it is harmless */
    }
}
export function currentTheme(): ThemeName {
    return document.documentElement.dataset.theme ?? "paper";
}
