import { Fragment } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { CardGrid, cssVar, has, Page, Section, SwatchCard, TOKENS, type Token } from "./specimens";

// Spectrum order (gray last) — reads as a palette, not an alphabetical dump.
const HUE_ORDER = ["red", "orange", "peach", "amber", "lime", "green", "teal", "cyan", "blue", "indigo", "purple", "magenta", "gray"];
// The six roles each hue scale carries, ordered background → ink → line → solid.
const ROLE_ORDER = ["tint", "tint-ink", "line", "solid", "on-solid", "ring"];

const hues = HUE_ORDER.filter((hue) => has(`color.${hue}.solid`));

function HueScaleGrid() {
    return (
        <div style={{ display: "grid", gridTemplateColumns: `84px repeat(${ROLE_ORDER.length}, 1fr)`, gap: "var(--alx-space-1)", alignItems: "center" }}>
            <span />
            {ROLE_ORDER.map((role) => (
                <span key={role} className="alx-type-caption" style={{ textAlign: "center", color: "var(--alx-ink-3)" }}>
                    {role}
                </span>
            ))}
            {hues.map((hue) => (
                <Fragment key={hue}>
                    <span className="alx-type-label-sm" style={{ textTransform: "capitalize" }}>
                        {hue}
                    </span>
                    {ROLE_ORDER.map((role) => {
                        const path = `color.${hue}.${role}`;
                        return has(path) ? (
                            <span
                                key={role}
                                title={cssVar(path)}
                                style={{ height: 40, borderRadius: "var(--alx-radius-control, 6px)", background: `var(${cssVar(path)})`, boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline)" }}
                            />
                        ) : (
                            <span key={role} />
                        );
                    })}
                </Fragment>
            ))}
        </div>
    );
}

// Semantic colors grouped by purpose (Polaris-style), not listed flat.
const SEMANTIC_GROUPS: { title: string; hint?: string; match: (token: Token) => boolean }[] = [
    { title: "Signals", hint: "state hues — never the sole cue (§10)", match: (t) => ["accent", "attention", "error"].includes(t.path) },
    { title: "Ink", hint: "text ramp, dark → faint", match: (t) => t.path.startsWith("ink.") || t.path === "fun.ink" },
    { title: "Surface", hint: "elevation planes (§20)", match: (t) => t.path.startsWith("surface.") },
    { title: "Cell", hint: "asset-grid mat states (§11)", match: (t) => t.path.startsWith("cell.") },
    { title: "Label", hint: "user color labels", match: (t) => t.path.startsWith("label.") },
    { title: "Stage", hint: "material round anchors", match: (t) => t.path.startsWith("stage.") },
];

const meta: Meta = {
    title: "Design System/Colors",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Colors: Story = {
    render: () => (
        <Page title="Colors" intro="Every color resolves per theme — switch paper / linen / graphite / carbon from the toolbar and the swatches reflow live. Hue scales carry six roles; semantic colors bind those to a purpose.">
            <Section title="Hue scales" hint={`${hues.length} hues × ${ROLE_ORDER.length} roles`}>
                <HueScaleGrid />
            </Section>
            {SEMANTIC_GROUPS.map((group) => {
                const tokens = TOKENS.filter((token) => token.type === "color" && group.match(token));
                if (tokens.length === 0) return null;
                return (
                    <Section key={group.title} title={group.title} hint={group.hint}>
                        <CardGrid>
                            {tokens.map((token) => (
                                <SwatchCard key={token.path} token={token} />
                            ))}
                        </CardGrid>
                    </Section>
                );
            })}
        </Page>
    ),
};
