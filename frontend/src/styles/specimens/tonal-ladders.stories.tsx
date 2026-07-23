import type { Meta, StoryObj } from "@storybook/react-vite";
import { cssVar, DEFAULT_THEME, Mono, Page, Section, TOKENS, type Token } from "./specimens";

// The neutral tonal families, each as a ramp around its anchor, FOR THE CURRENTLY
// SELECTED THEME. ink/surface/cell vary per theme; stage is fixed. Values (and the
// ΔL step from the anchor) are read from the theme's own oklch, so switching the
// toolbar theme re-derives the whole page. This is the material-round tonal
// doctrine made legible: pole anchor + the steps up (lighter) and down (darker).

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

function Ladder({ family, theme }: { family: Family; theme: string }) {
    const rungs = TOKENS.filter((token) => token.type === "color" && token.path.startsWith(`${family.key}.`))
        .map((token) => ({ token, lightness: parseL(valueForTheme(token, theme)) }))
        .filter((rung) => !Number.isNaN(rung.lightness))
        .sort((a, b) => b.lightness - a.lightness); // lightest at top

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
                                {isAnchor
                                    ? "◆ anchor"
                                    : delta > 0
                                      ? `↑ +${delta.toFixed(3)}  lighter`
                                      : `↓ ${delta.toFixed(3)}  darker`}
                            </span>
                        </div>
                    );
                })}
            </div>
        </Section>
    );
}

const meta: Meta = {
    title: "Design System/Tonal Ladders",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const TonalLadders: Story = {
    render: (_args, { globals }) => {
        const theme = (globals.theme as string) ?? DEFAULT_THEME;
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
