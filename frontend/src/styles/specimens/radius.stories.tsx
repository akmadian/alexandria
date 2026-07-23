import type { Meta, StoryObj } from "@storybook/react-vite";
import { byType, cssVar, Mono, Page, rawValue, Section } from "./specimens";

// radius encodes detachment: docked 0 → control → transient → round.
const radii = byType("dimension")
    .filter((token) => token.path.startsWith("radius."))
    .sort((a, b) => (parseFloat(rawValue(a)) || 0) - (parseFloat(rawValue(b)) || 0));

const meta: Meta = {
    title: "Design System/Radius",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Radius: Story = {
    render: () => (
        <Page title="Radius" intro="Corner radius encodes how detached a surface is — docked chrome is square, transient overlays are softest, pills are fully round.">
            <Section title="Radius scale">
                <div style={{ display: "flex", gap: "var(--alx-space-8)", flexWrap: "wrap" }}>
                    {radii.map((token) => (
                        <div key={token.path} style={{ textAlign: "center" }}>
                            <span
                                title={cssVar(token.path)}
                                style={{ display: "block", width: 72, height: 72, background: "var(--alx-surface-raised)", boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline)", borderTopLeftRadius: `var(${cssVar(token.path)})`, borderTopRightRadius: `var(${cssVar(token.path)})` }}
                            />
                            <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)", marginTop: "var(--alx-space-1)" }}>
                                {token.path.split(".").pop()} · {rawValue(token)}
                            </div>
                            <Mono>{cssVar(token.path)}</Mono>
                        </div>
                    ))}
                </div>
            </Section>
        </Page>
    ),
};
