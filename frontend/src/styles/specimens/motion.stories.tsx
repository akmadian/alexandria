import type { Meta, StoryObj } from "@storybook/react-vite";
import { byType, cssVar, Mono, Page, rawValue, Section, type Token } from "./specimens";

const durations = byType("duration");
const easings = byType("cubicBezier");

// A dot that slides the track; the token drives duration (or timing-function).
function Track({ style }: { style: React.CSSProperties }) {
    return (
        <div style={{ position: "relative", height: 16, width: 220, background: "var(--alx-surface-sunken)", borderRadius: 999 }}>
            <span style={{ position: "absolute", top: 0, left: 0, width: 16, height: 16, borderRadius: 999, background: "var(--alx-accent)", animation: "specimen-slide 1.4s infinite alternate", ...style }} />
        </div>
    );
}

function BezierCurve({ token }: { token: Token }) {
    const nums = (rawValue(token).match(/-?\d*\.?\d+/g) ?? []).map(Number);
    const [x1 = 0.4, y1 = 0, x2 = 0.2, y2 = 1] = nums;
    const size = 56;
    // SVG y grows downward, so invert the control-point y values.
    const path = `M0,${size} C${x1 * size},${(1 - y1) * size} ${x2 * size},${(1 - y2) * size} ${size},0`;
    return (
        <svg width={size} height={size} style={{ overflow: "visible", flex: "none" }} aria-hidden>
            <path d={`M0,${size} L${size},0`} stroke="var(--alx-ink-hairline)" strokeDasharray="3 3" fill="none" />
            <path d={path} stroke="var(--alx-accent)" strokeWidth={2} fill="none" />
        </svg>
    );
}

const meta: Meta = {
    title: "Design System/Motion",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const Motion: Story = {
    render: () => (
        <Page title="Motion" intro="Durations and easing curves, animated live. Motion is transform/opacity only and yields to prefers-reduced-motion by construction.">
            <style>{`@keyframes specimen-slide { from { left: 0 } to { left: calc(100% - 16px) } }`}</style>
            <Section title="Durations">
                <div style={{ display: "grid", gap: "var(--alx-space-4)" }}>
                    {durations.map((token) => (
                        <div key={token.path} style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
                            <div style={{ width: 200 }}>
                                <Mono>{cssVar(token.path)}</Mono>
                                <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                                    {rawValue(token)}
                                </div>
                            </div>
                            <Track style={{ animationDuration: `var(${cssVar(token.path)})` }} />
                        </div>
                    ))}
                </div>
            </Section>
            <Section title="Easing">
                <div style={{ display: "grid", gap: "var(--alx-space-6)" }}>
                    {easings.map((token) => (
                        <div key={token.path} style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
                            <BezierCurve token={token} />
                            <div style={{ width: 200 }}>
                                <Mono>{cssVar(token.path)}</Mono>
                                <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)" }}>
                                    {rawValue(token)}
                                </div>
                            </div>
                            <Track style={{ animationDuration: "1.4s", animationTimingFunction: `var(${cssVar(token.path)})` }} />
                        </div>
                    ))}
                </div>
            </Section>
        </Page>
    ),
};
