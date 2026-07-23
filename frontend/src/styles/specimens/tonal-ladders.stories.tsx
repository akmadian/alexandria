import type { Meta, StoryObj } from "@storybook/react-vite";
import { cssVar, DEFAULT_THEME, Mono, Page, Section, TOKENS, type Token } from "./specimens";

// The neutral tonal families (ink / surface / cell / stage) FOR THE CURRENTLY
// SELECTED THEME, in two views: the Ladders (each family as a vertical ramp around
// its anchor) and the Spectrum (every family placed on one shared black→white
// lightness axis). ink/surface/cell vary per theme; stage is fixed. Values are read
// from the theme's own oklch, so switching the toolbar theme re-derives everything.

interface Family {
    key: string;
    label: string;
    hint: string;
    anchor: string; // the token path that anchors the ramp
}

const FAMILIES: Family[] = [
    { key: "ink", label: "Ink", hint: "text ramp — primary → disabled, plus the hairline seam", anchor: "ink.1" },
    { key: "surface", label: "Surface", hint: "elevation planes around the panel anchor (§20)", anchor: "surface.panel" },
    { key: "cell", label: "Cell", hint: "asset-grid mat states (§19)", anchor: "cell.rest" },
    { key: "stage", label: "Stage", hint: "the backdrop — material-round anchors (fixed across themes)", anchor: "stage.default" },
];

// oklch(L C H) → L. These families are achromatic, so L is the first number.
const parseL = (value: string): number => {
    const match = value.match(/oklch\(\s*([\d.]+)/);
    return match ? parseFloat(match[1]) : NaN;
};

const valueForTheme = (token: Token, theme: string): string => {
    if (typeof token.css === "string") return token.css; // fixed (e.g. stage)
    if (token.css) return token.css[theme] ?? token.css[DEFAULT_THEME] ?? "";
    return "";
};

const rungsFor = (family: Family, theme: string) =>
    TOKENS.filter((token) => token.type === "color" && token.path.startsWith(`${family.key}.`))
        .map((token) => ({ token, lightness: parseL(valueForTheme(token, theme)) }))
        .filter((rung) => !Number.isNaN(rung.lightness));

// ── Ladders view ────────────────────────────────────────────────────────────

function Ladder({ family, theme }: { family: Family; theme: string }) {
    const rungs = rungsFor(family, theme).sort((a, b) => b.lightness - a.lightness); // lightest at top
    const anchorL = rungs.find((rung) => rung.token.path === family.anchor)?.lightness ?? NaN;
    return (
        <Section title={family.label} hint={family.hint}>
            <div style={{ display: "grid", gap: "var(--alx-space-1)" }}>
                {rungs.map(({ token, lightness }) => {
                    const isAnchor = token.path === family.anchor;
                    const delta = lightness - anchorL;
                    return (
                        <div
                            key={token.path}
                            style={{
                                display: "grid",
                                gridTemplateColumns: "64px minmax(180px, 260px) 68px 1fr",
                                gap: "var(--alx-space-4)",
                                alignItems: "center",
                                borderLeft: `2px solid ${isAnchor ? "var(--alx-accent)" : "transparent"}`,
                                paddingLeft: "var(--alx-space-3)",
                            }}
                        >
                            <span
                                title={cssVar(token.path)}
                                style={{ height: 30, borderRadius: "var(--alx-radius-control, 6px)", background: `var(${cssVar(token.path)})`, boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline)" }}
                            />
                            <Mono>{cssVar(token.path)}</Mono>
                            <span className="alx-type-data-sm" style={{ color: "var(--alx-ink-2)" }}>
                                L {lightness.toFixed(3)}
                            </span>
                            <span className="alx-type-caption" style={{ color: isAnchor ? "var(--alx-accent)" : "var(--alx-ink-3)" }}>
                                {isAnchor ? "◆ anchor" : delta > 0 ? `↑ +${delta.toFixed(3)}  lighter` : `↓ ${delta.toFixed(3)}  darker`}
                            </span>
                        </div>
                    );
                })}
            </div>
        </Section>
    );
}

// ── Spectrum view ───────────────────────────────────────────────────────────

// One shared axis: pure black (L 0) at left, pure white (L 1) at right. The track
// gradient interpolates in oklab so on-screen position matches oklch lightness.
function SpectrumRow({ family, theme }: { family: Family; theme: string }) {
    const rungs = rungsFor(family, theme).sort((a, b) => a.lightness - b.lightness);
    return (
        <div style={{ display: "grid", gridTemplateColumns: "72px 1fr", gap: "var(--alx-space-4)", alignItems: "center" }}>
            <span className="alx-type-label-sm">{family.label}</span>
            <div style={{ position: "relative", height: 24, borderRadius: 999, background: "linear-gradient(in oklab to right, oklch(0 0 0), oklch(1 0 0))", boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline)" }}>
                {rungs.map(({ token, lightness }) => {
                    const isAnchor = token.path === family.anchor;
                    const size = isAnchor ? 20 : 14;
                    return (
                        <span
                            key={token.path}
                            title={`${cssVar(token.path)}  ·  L ${lightness.toFixed(3)}`}
                            style={{
                                position: "absolute",
                                top: "50%",
                                left: `${lightness * 100}%`,
                                transform: "translate(-50%, -50%)",
                                width: size,
                                height: size,
                                borderRadius: "50%",
                                background: `var(${cssVar(token.path)})`,
                                // Dark inner + light outer ring stays visible at any point on the gradient.
                                boxShadow: isAnchor ? "0 0 0 1px rgba(0,0,0,.45), 0 0 0 3px var(--alx-accent)" : "0 0 0 1px rgba(0,0,0,.45), 0 0 0 2.5px rgba(255,255,255,.65)",
                                zIndex: isAnchor ? 2 : 1,
                            }}
                        />
                    );
                })}
            </div>
        </div>
    );
}

function ScaleRuler() {
    const marks = [0, 0.25, 0.5, 0.75, 1];
    return (
        <div style={{ display: "grid", gridTemplateColumns: "72px 1fr", gap: "var(--alx-space-4)" }}>
            <span className="alx-type-caption" style={{ color: "var(--alx-ink-4)" }}>
                L
            </span>
            <div style={{ position: "relative", height: 16 }}>
                {marks.map((mark) => (
                    <span key={mark} className="alx-type-caption" style={{ position: "absolute", left: `${mark * 100}%`, transform: "translateX(-50%)", color: "var(--alx-ink-4)" }}>
                        {mark.toFixed(2)}
                    </span>
                ))}
            </div>
        </div>
    );
}

// ── Stories ─────────────────────────────────────────────────────────────────

const meta: Meta = {
    title: "Design System/Tonal Ladders",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

const themeOf = (globals: Record<string, unknown>) => (globals.theme as string) ?? DEFAULT_THEME;

export const Ladders: Story = {
    render: (_args, { globals }) => {
        const theme = themeOf(globals);
        return (
            <Page
                title="Tonal Ladders"
                intro={
                    <>
                        The neutral tone families for the <strong style={{ color: "var(--alx-ink-1)" }}>{theme}</strong> theme — each a ramp around its anchor (◆), with the oklch lightness and the step up (lighter) or down (darker) from the anchor. Switch themes from the toolbar; every value re-derives.
                    </>
                }
            >
                {FAMILIES.map((family) => (
                    <Ladder key={family.key} family={family} theme={theme} />
                ))}
            </Page>
        );
    },
};

export const Spectrum: Story = {
    render: (_args, { globals }) => {
        const theme = themeOf(globals);
        return (
            <Page
                title="Tonal Spectrum"
                intro={
                    <>
                        Every neutral family on one shared lightness axis for the <strong style={{ color: "var(--alx-ink-1)" }}>{theme}</strong> theme — pure black (L 0) at left, pure white (L 1) at right. Each swatch sits at its oklch lightness; the anchor is ringed in accent. Ink rides the dark end, surfaces the light end. Switch themes from the toolbar.
                    </>
                }
            >
                <div style={{ display: "grid", gap: "var(--alx-space-6)" }}>
                    {FAMILIES.map((family) => (
                        <SpectrumRow key={family.key} family={family} theme={theme} />
                    ))}
                    <ScaleRuler />
                </div>
            </Page>
        );
    },
};
