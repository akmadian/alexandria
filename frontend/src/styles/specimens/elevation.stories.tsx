import type { Meta, StoryObj } from "@storybook/react-vite";
import { byType, cssVar, Mono, Page, Section, shortRole, type Token } from "./specimens";

// Shadow is a layer property: only transient overlays cast one (docked chrome is flat).
const shadows = byType("shadow");
const isText = (token: Token) => token.path.includes("text-shadow");

const meta: Meta = {
    title: "Design System/Elevation",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Elevation: Story = {
    render: () => (
        <Page title="Elevation" intro="Elevation is a layer property — only transient overlays cast a shadow; docked chrome stays flat. The two 'fun' shadows are the injected material layer (§17).">
            <Section title="Shadows">
                <div style={{ display: "flex", gap: "var(--alx-space-8)", flexWrap: "wrap", background: "var(--alx-surface-sunken)", padding: "var(--alx-space-8)", borderRadius: "var(--alx-radius-transient, 12px)" }}>
                    {shadows.map((token) => (
                        <div key={token.path} style={{ textAlign: "center" }}>
                            {isText(token) ? (
                                <div style={{ display: "grid", placeItems: "center", width: 120, height: 88, background: "var(--alx-surface-raised)", borderRadius: "var(--alx-radius-control, 6px)" }}>
                                    <span className="alx-type-head" style={{ textShadow: `var(${cssVar(token.path)})` }}>
                                        Aa
                                    </span>
                                </div>
                            ) : (
                                <span title={cssVar(token.path)} style={{ display: "block", width: 120, height: 88, background: "var(--alx-surface-raised)", borderRadius: "var(--alx-radius-control, 6px)", boxShadow: `var(${cssVar(token.path)})` }} />
                            )}
                            <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)", marginTop: "var(--alx-space-2)" }}>
                                {token.path.split(".").pop()}
                            </div>
                            <Mono>{cssVar(token.path)}</Mono>
                            {token.role && (
                                <div className="alx-type-caption" style={{ color: "var(--alx-ink-4)", maxWidth: 120 }}>
                                    {shortRole(token.role)}
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            </Section>
        </Page>
    ),
};
