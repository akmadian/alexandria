import type { Meta, StoryObj } from "@storybook/react-vite";
import { cssVar, Mono, Page, rawValue, Section, TOKENS, type Token } from "./specimens";

const px = (token: Token) => parseFloat(rawValue(token)) || 0;
const pick = (prefix: string) => TOKENS.filter((token) => token.path.startsWith(prefix)).sort((a, b) => px(a) - px(b));

const spaceScale = pick("space.");
const controlSizes = TOKENS.filter((token) => token.path.startsWith("size.control-")).sort((a, b) => px(a) - px(b));
const iconSizes = TOKENS.filter((token) => token.path.startsWith("size.icon") && token.path !== "size.icon-stroke").sort((a, b) => px(a) - px(b));
const rowHeights = TOKENS.filter((token) => token.path.startsWith("size.row-")).sort((a, b) => px(a) - px(b));

function ScaleRow({ token }: { token: Token }) {
    return (
        <div style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
            <Mono>{cssVar(token.path)}</Mono>
            <span className="alx-type-caption" style={{ width: 48, color: "var(--alx-ink-3)" }}>
                {rawValue(token)}
            </span>
            <span style={{ height: 16, width: `var(${cssVar(token.path)})`, background: "var(--alx-accent)", borderRadius: 2 }} />
        </div>
    );
}

function SizeBox({ token }: { token: Token }) {
    return (
        <div style={{ textAlign: "center" }}>
            <div style={{ display: "grid", placeItems: "center", height: 32 }}>
                <span style={{ width: `var(${cssVar(token.path)})`, height: `var(${cssVar(token.path)})`, background: "var(--alx-ink-2)", borderRadius: 2 }} />
            </div>
            <div className="alx-type-caption" style={{ color: "var(--alx-ink-3)", marginTop: "var(--alx-space-1)" }}>
                {token.path.split(".").pop()}
            </div>
            <div className="alx-type-caption" style={{ color: "var(--alx-ink-4)" }}>
                {rawValue(token)}
            </div>
        </div>
    );
}

const meta: Meta = {
    title: "Design System/Spacing & Sizing",
    parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj;

export const SpacingAndSizing: Story = {
    render: () => (
        <Page title="Spacing & Sizing" intro="Spacing is quantum multiples via --alx-space-N. Control, icon and row dimensions are drawn at their true pixel size.">
            <Section title="Space scale" hint={`${spaceScale.length} steps`}>
                <div style={{ display: "grid", gap: "var(--alx-space-2)" }}>
                    {spaceScale.map((token) => (
                        <ScaleRow key={token.path} token={token} />
                    ))}
                </div>
            </Section>
            <Section title="Control sizes" hint="the §8 size ladder">
                <div style={{ display: "flex", gap: "var(--alx-space-8)", alignItems: "flex-end" }}>
                    {controlSizes.map((token) => (
                        <SizeBox key={token.path} token={token} />
                    ))}
                </div>
            </Section>
            <Section title="Icon sizes">
                <div style={{ display: "flex", gap: "var(--alx-space-8)", alignItems: "flex-end" }}>
                    {iconSizes.map((token) => (
                        <SizeBox key={token.path} token={token} />
                    ))}
                </div>
            </Section>
            <Section title="Row heights" hint="intent → height binding">
                <div style={{ display: "grid", gap: "var(--alx-space-2)" }}>
                    {rowHeights.map((token) => (
                        <div key={token.path} style={{ display: "flex", gap: "var(--alx-space-4)", alignItems: "center" }}>
                            <Mono>{cssVar(token.path)}</Mono>
                            <span className="alx-type-caption" style={{ width: 48, color: "var(--alx-ink-3)" }}>
                                {rawValue(token)}
                            </span>
                            <span style={{ height: `var(${cssVar(token.path)})`, width: 220, background: "var(--alx-surface-raised)", boxShadow: "inset 0 0 0 1px var(--alx-ink-hairline)", borderRadius: 2 }} />
                        </div>
                    ))}
                </div>
            </Section>
        </Page>
    ),
};
